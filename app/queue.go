package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-hypercube/go-hypercube/job"
	"github.com/go-hypercube/go-hypercube/namespaced"
	"github.com/go-hypercube/go-hypercube/queue"
	"github.com/google/uuid"
)

// defaultWorkerConcurrency and defaultWorkerPollInterval are used by
// Worker whenever the corresponding WorkerOptions field is left at its
// zero value.
const (
	defaultWorkerConcurrency  = 1
	defaultWorkerPollInterval = 200 * time.Millisecond
)

// Jobs returns all jobs registered with the app across every namespace.
func (app *App) Jobs() namespaced.NamespacedSlice[job.Job] { return app.jobs }

// RegisterJob registers jobs under the framework's own reserved
// namespace, as opposed to a plugin's namespace.
func (app *App) RegisterJob(jobs ...job.Job) error {
	return app.registerJobForNamespace(hostAppNamespace, jobs...)
}

// registerJobForNamespace wraps each job in a job.Namespaced under
// namespace and adds it to app.jobs. A job is live
// behavior, so re-registering the same (namespace, name) replaces the
// existing entry in place rather than erroring or duplicating.
func (app *App) registerJobForNamespace(namespace string, jobs ...job.Job) error {
	for _, j := range jobs {
		namespaced := job.NewNamespaced(namespace, j)

		replaced := false
		for i, existing := range app.jobs {
			if existing.Namespace == namespace && existing.Item.Name() == j.Name() {
				app.jobs[i] = namespaced
				replaced = true
				break
			}
		}
		if !replaced {
			app.jobs = append(app.jobs, namespaced)
		}
	}
	return nil
}

// Dispatch enqueues jobName for immediate processing.
func (app *App) Dispatch(jobName string, payload []byte) error {
	return app.dispatch(hostAppNamespace, jobName, payload, 0)
}

// DispatchIn enqueues jobName for processing after delay.
func (app *App) DispatchIn(jobName string, payload []byte, delay time.Duration) error {
	return app.dispatch(hostAppNamespace, jobName, payload, delay)
}

// dispatch looks up the registered job to resolve its declared queue
// name (job.QueueNameFor) before pushing — this is the piece that was
// missing before: Push now always knows exactly which queue a message
// belongs to, driven by the job's own registration rather than a value
// the caller has to separately remember and pass in.
func (app *App) dispatch(namespace, jobName string, payload []byte, delay time.Duration) error {
	j, ok := app.jobs.Get(namespace, jobName)
	if !ok {
		return fmt.Errorf("job %q not found in namespace %q", jobName, namespace)
	}
	return app.queue.Push(context.Background(), &queue.Message{
		ID:        uuid.NewString(),
		QueueName: job.QueueNameFor(j),
		Namespace: namespace,
		JobName:   jobName,
		Payload:   payload,
	}, delay)
}

// Worker starts op.Concurrency goroutines processing messages from
// op.QueueName until ctx is cancelled, blocking until all have exited.
//
// If app.queue implements queue.PopStream natively, its own
// implementation drives delivery and op.PollInterval is unused.
// Otherwise app.queue is wrapped via queue.PollAsStream(app.queue,
// op.PollInterval), so Worker always runs through a single
// shared-channel code path — concurrency goroutines ranging over one
// channel — regardless of which kind of driver is configured.
func (app *App) Worker(ctx context.Context, op WorkerOptions) error {
	if err := op.validate(); err != nil {
		return err
	}
	op.applyDefaults(app)

	streamer, ok := app.queue.(queue.PopStream)
	if !ok {
		streamer = queue.PollAsStream(app.queue, op.PollInterval)
	}

	ch, err := streamer.PopChan(ctx, op.QueueName)
	if err != nil {
		return fmt.Errorf("start pop stream for queue %q: %w", op.QueueName, err)
	}

	var wg sync.WaitGroup
	for range op.Concurrency {
		wg.Go(func() {
			for result := range ch { // closed by the driver/PollAsStream on ctx cancellation
				if result.Err != nil {
					op.ErrorHandler(result.Err)
					continue
				}
				app.processMessage(ctx, result.Msg)
			}
		})
	}
	wg.Wait()
	return nil
}

// processMessage looks up the job by (msg.Namespace, msg.JobName)
// runs it, and Acks, retries, or dead-letters based on the result 
// and the job's RetryPolicy.
func (app *App) processMessage(ctx context.Context, msg *queue.Message) {
	j, ok := app.jobs.Get(msg.Namespace, msg.JobName)
	if !ok {
		err := fmt.Errorf("job %q not registered in namespace %q", msg.JobName, msg.Namespace)
		app.logger.Error("unregistered job, dead-lettering", "namespace", msg.Namespace, "job", msg.JobName)
		if dlErr := app.deadLetter(ctx, msg, err); dlErr != nil {
			app.logger.Error("dead-letter failed", "err", dlErr)
		}
		return
	}

	runErr := j.Handle(job.NewAppForJob(&job.Options{
		Job:       j,
		Database:  app.database,
		Cache:     app.cache,
		Queue:     app.queue,
		Logger:    app.logger.With("namespace", msg.Namespace, "job", msg.JobName, "attempt", msg.Attempt),
		Container: app.services,
	}), msg.Payload)

	if runErr == nil {
		if err := app.queue.Ack(ctx, msg); err != nil {
			app.logger.Error("ack failed", "namespace", msg.Namespace, "job", msg.JobName, "err", err)
		}
		return
	}

	policy := job.RetryPolicyFor(j)
	if msg.Attempt >= policy.MaxAttempts() {
		app.logger.Error("job exhausted retries, dead-lettering",
			"namespace", msg.Namespace, "job", msg.JobName, "attempt", msg.Attempt, "err", runErr)
		if err := app.deadLetter(ctx, msg, runErr); err != nil {
			app.logger.Error("dead-letter failed", "err", err)
		}
		return
	}

	if err := app.queue.Fail(ctx, msg, policy.Backoff(msg.Attempt)); err != nil {
		app.logger.Error("fail/requeue failed", "namespace", msg.Namespace, "job", msg.JobName, "err", err)
	}
}

// deadLettersTable is the bookkeeping table for permanently failed
// jobs, following the hypercube_* naming convention used by
// hypercube_migrations and hypercube_seeders.
const deadLettersTable = "hypercube_dead_letters"

func (app *App) ensureDeadLettersTable() error {
	_, err := app.database.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id         TEXT PRIMARY KEY,
			namespace  TEXT NOT NULL,
			job        TEXT NOT NULL,
			queue_name TEXT NOT NULL,
			payload    BLOB,
			attempt    INTEGER NOT NULL,
			error      TEXT NOT NULL,
			failed_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`, deadLettersTable))
	return err
}

// deadLetter permanently records msg's failure and Acks it out of the
// live queue — dead-lettering means "stop redelivering," which from the
// driver's point of view is exactly what Ack does; the failure detail
// is preserved separately here for inspection/replay instead.
func (app *App) deadLetter(ctx context.Context, msg *queue.Message, cause error) error {
	if err := app.ensureDeadLettersTable(); err != nil {
		return fmt.Errorf("ensure dead letters table: %w", err)
	}

	dbDriver := app.readDbDriver()
	query := fmt.Sprintf(
		`INSERT INTO %s (id, namespace, job, queue_name, payload, attempt, error) VALUES (%s, %s, %s, %s, %s, %s, %s)`,
		deadLettersTable,
		dbDriver.placeholder(1), dbDriver.placeholder(2), dbDriver.placeholder(3),
		dbDriver.placeholder(4), dbDriver.placeholder(5), dbDriver.placeholder(6), dbDriver.placeholder(7),
	)
	if _, err := app.database.Exec(query,
		msg.ID, msg.Namespace, msg.JobName, msg.QueueName, msg.Payload, msg.Attempt, cause.Error(),
	); err != nil {
		return fmt.Errorf("insert dead letter %q/%q: %w", msg.Namespace, msg.JobName, err)
	}

	if err := app.queue.Ack(ctx, msg); err != nil {
		return fmt.Errorf("ack dead-lettered message %q/%q: %w", msg.Namespace, msg.JobName, err)
	}
	return nil
}

// WorkerOptions configures a call to Worker. QueueName is required;
// Concurrency and PollInterval fall back to their package defaults
// (defaultWorkerConcurrency, defaultWorkerPollInterval) when left at
// zero. ErrorHandler defaults to logging via app.logger when nil.
type WorkerOptions struct {
	// QueueName is the queue to consume from. Required.
	QueueName string

	// Concurrency is how many goroutines process messages concurrently.
	// Defaults to 1 if <= 0.
	Concurrency int

	// PollInterval is how often an empty/errored queue is re-checked,
	// when app.queue does not implement queue.PopStream natively and is
	// wrapped via queue.PollAsStream. Ignored entirely for a driver that
	// implements PopStream itself (e.g. MemoryQueue). Defaults to
	// defaultWorkerPollInterval if <= 0.
	PollInterval time.Duration

	// ErrorHandler is called, on the goroutine that received it, for
	// every queue.Result with a non-nil Err (e.g. a persistent Pop
	// failure surfaced by queue.PollAsStream) — giving the caller a hook
	// for alerting, metrics, or circuit-breaking, rather than the error
	// only ever reaching app.logger. It must be safe for concurrent use:
	// with Concurrency > 1, more than one goroutine may call it at the
	// same time.
	//
	// If nil, defaults to logging err via app.logger at Error level
	// (the previous, only, behavior) — so existing callers that don't
	// set this see no change.
	ErrorHandler func(err error)
}

func (o *WorkerOptions) applyDefaults(app *App) {
	if o.Concurrency <= 0 {
		o.Concurrency = defaultWorkerConcurrency
	}
	if o.PollInterval <= 0 {
		o.PollInterval = defaultWorkerPollInterval
	}
	if o.ErrorHandler == nil {
		o.ErrorHandler = func(err error) {
			app.logger.Error("pop stream error", "queue", o.QueueName, "err", err)
		}
	}
}

func (o WorkerOptions) validate() error {
	if o.QueueName == "" {
		return errors.New("queue name is required")
	}
	return nil
}
