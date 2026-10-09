package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-hypercube/go-hypercube/job"
	"github.com/go-hypercube/go-hypercube/namespaced"
	"github.com/go-hypercube/go-hypercube/queue"
)

const (
	defaultWorkerConcurrency  = 1
	defaultWorkerErrorBackoff = 200 * time.Millisecond
)

// Jobs returns all jobs registered with the app across every namespace.
func (app *App) Jobs() namespaced.NamespacedSlice[job.Job] { return app.jobs }

// RegisterJob registers jobs under the framework's own reserved namespace, as
// opposed to a plugin's namespace.
func (app *App) RegisterJob(jobs ...job.Job) error {
	return app.registerJobForNamespace(hostAppNamespace, jobs...)
}

// registerJobForNamespace wraps each job in a job.Namespaced under namespace
// and adds it to app.jobs. A job is live behavior, so re-registering the same
// (namespace, name) replaces the existing entry in place.
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

// DispatchConfig configures a single job dispatch.
type DispatchConfig = job.DispatchConfig

// Dispatch enqueues jobName for processing in the framework namespace.
func (app *App) Dispatch(ctx context.Context, jobName string, payload []byte, config DispatchConfig) error {
	return app.dispatch(ctx, hostAppNamespace, jobName, payload, config)
}

func (app *App) dispatch(ctx context.Context, namespace, jobName string, payload []byte, config DispatchConfig) error {
	j, ok := app.jobs.Get(namespace, jobName)
	if !ok {
		return fmt.Errorf("job %q not found in namespace %q", jobName, namespace)
	}

	visibilityTimeout := config.VisibilityTimeout
	if visibilityTimeout == 0 {
		visibilityTimeout = job.VisibilityTimeoutFor(j)
	}

	msg := &queue.Message{
		QueueName:         job.QueueNameFor(j),
		Namespace:         namespace,
		JobName:           jobName,
		Delay:             config.Delay,
		Payload:           payload,
		VisibilityTimeout: visibilityTimeout,
	}
	if err := app.queue.Push(ctx, msg); err != nil {
		return fmt.Errorf("dispatch job %q in namespace %q: %w", jobName, namespace, err)
	}
	return nil
}

// Worker starts op.Concurrency consumers for op.QueueName and blocks until ctx
// ends and every consumer exits. Queue.Pop owns waiting and long-poll behavior;
// the app does not add a second polling loop around an empty queue.
func (app *App) Worker(ctx context.Context, op WorkerOptions) error {
	if err := op.validate(); err != nil {
		return err
	}
	op.applyDefaults(app)

	var wg sync.WaitGroup
	for range op.Concurrency {
		wg.Go(func() {
			app.consumeQueue(ctx, op)
		})
	}
	wg.Wait()
	return nil
}

func (app *App) consumeQueue(ctx context.Context, op WorkerOptions) {
	for {
		msgs, err := app.queue.Pop(ctx, op.QueueName, 1)
		if err != nil {
			if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
				return
			}
			if errors.Is(err, queue.ErrEmpty) {
				continue
			}

			op.ErrorHandler(fmt.Errorf("pop queue %q: %w", op.QueueName, err))
			if !waitForQueueRetry(ctx, op.ErrorBackoff) {
				return
			}
			continue
		}

		for _, msg := range msgs {
			if msg == nil {
				op.ErrorHandler(fmt.Errorf(
					"pop queue %q returned a nil message: %w",
					op.QueueName,
					queue.ErrInvalidArgument,
				))
				continue
			}

			// Cancellation stops new Pop calls but does not abandon a delivery that
			// has already been handed to a handler. Let that delivery settle during
			// graceful shutdown; driver-level timeouts still bound backend calls.
			app.processMessage(context.WithoutCancel(ctx), msg)
		}
	}
}

func waitForQueueRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// processMessage runs the registered job and explicitly commands the queue to
// acknowledge, retry, or dead-letter the current delivery.
func (app *App) processMessage(ctx context.Context, msg *queue.Message) {
	j, ok := app.jobs.Get(msg.Namespace, msg.JobName)
	if !ok {
		cause := fmt.Errorf("job %q not registered in namespace %q", msg.JobName, msg.Namespace)
		app.logger.Error(
			"unregistered job, dead-lettering",
			"namespace", msg.Namespace,
			"job", msg.JobName,
		)
		if err := app.deadLetter(ctx, msg, cause); err != nil {
			app.logSettlementError("dead-letter", msg, err)
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
			app.logSettlementError("ack", msg, err)
		}
		return
	}

	policy := job.RetryPolicyFor(j)
	if msg.Attempt >= policy.MaxAttempts() {
		app.logger.Error(
			"job exhausted retries, dead-lettering",
			"namespace", msg.Namespace,
			"job", msg.JobName,
			"attempt", msg.Attempt,
			"err", runErr,
		)
		if err := app.deadLetter(ctx, msg, runErr); err != nil {
			app.logSettlementError("dead-letter", msg, err)
		}
		return
	}

	msg.Delay = policy.Backoff(msg.Attempt)
	if err := app.queue.Retry(ctx, msg); err != nil {
		app.logSettlementError("retry", msg, err)
	}
}

type deadLetterMetadata struct {
	Error         string `json:"error"`
	PreviousExtra []byte `json:"previous_extra,omitempty"`
}

// deadLetter records the handler failure in the queue message and delegates the
// failed-state transition to the queue driver. The original Extra value is
// retained inside the failure metadata rather than discarded.
func (app *App) deadLetter(ctx context.Context, msg *queue.Message, cause error) error {
	extra, err := json.Marshal(deadLetterMetadata{
		Error:         cause.Error(),
		PreviousExtra: msg.Extra,
	})
	if err != nil {
		return fmt.Errorf("encode dead-letter metadata for message %q: %w", msg.ID, err)
	}
	msg.Extra = extra

	if err := app.queue.DeadLetter(ctx, msg); err != nil {
		return fmt.Errorf("dead-letter message %q: %w", msg.ID, err)
	}
	return nil
}

func (app *App) logSettlementError(operation string, msg *queue.Message, err error) {
	itemErr := batchItemError(err, msg)
	if errors.Is(itemErr, queue.ErrNotFound) {
		app.logger.Warn(
			"queue delivery is no longer current",
			"operation", operation,
			"namespace", msg.Namespace,
			"job", msg.JobName,
			"message_id", msg.ID,
			"err", itemErr,
		)
		return
	}

	app.logger.Error(
		"queue settlement failed",
		"operation", operation,
		"namespace", msg.Namespace,
		"job", msg.JobName,
		"message_id", msg.ID,
		"err", itemErr,
	)
}

// batchItemError extracts the item-level cause from the one-message batch calls
// used by the worker while preserving operation-level errors unchanged.
func batchItemError(err error, msg *queue.Message) error {
	var batchErr *queue.BatchError
	if !errors.As(err, &batchErr) {
		return err
	}
	for _, failure := range batchErr.Failed {
		if failure.Msg == msg {
			return failure.Err
		}
	}
	return err
}

// WorkerOptions configures a call to Worker.
type WorkerOptions struct {
	// QueueName is the queue to consume from. Required.
	QueueName string

	// Concurrency is the maximum number of jobs processed concurrently. Each
	// consumer owns at most one in-flight delivery at a time. Defaults to 1.
	Concurrency int

	// ErrorBackoff is the delay before retrying Pop after an infrastructure
	// error. ErrEmpty does not use this delay because Pop already waited for the
	// driver-configured polling timeout. Defaults to 200ms.
	ErrorBackoff time.Duration

	// ErrorHandler receives Pop infrastructure and contract errors. It is called
	// by consumer goroutines and therefore must be safe for concurrent use.
	// ErrEmpty and context cancellation are not reported. A nil handler defaults
	// to structured logging through app.Logger().
	ErrorHandler func(err error)
}

func (o *WorkerOptions) applyDefaults(app *App) {
	if o.Concurrency <= 0 {
		o.Concurrency = defaultWorkerConcurrency
	}
	if o.ErrorBackoff <= 0 {
		o.ErrorBackoff = defaultWorkerErrorBackoff
	}
	if o.ErrorHandler == nil {
		o.ErrorHandler = func(err error) {
			app.logger.Error("queue pop failed", "queue", o.QueueName, "err", err)
		}
	}
}

func (o WorkerOptions) validate() error {
	if o.QueueName == "" {
		return errors.New("queue name is required")
	}
	return nil
}
