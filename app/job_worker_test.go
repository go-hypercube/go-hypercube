package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/go-hypercube/go-hypercube/job"
	"github.com/go-hypercube/go-hypercube/plugin"
	"github.com/go-hypercube/go-hypercube/queue"
	"github.com/stretchr/testify/require"
)

type workerJob struct {
	name   string
	handle func(*job.App, []byte) error
}

func (j *workerJob) Name() string { return j.name }
func (j *workerJob) Handle(app *job.App, payload []byte) error {
	return j.handle(app, payload)
}
func (*workerJob) MaxAttempts() int                 { return 2 }
func (*workerJob) Backoff(int) time.Duration        { return time.Second }
func (*workerJob) VisibilityTimeout() time.Duration { return time.Minute }

type workerPlugin struct {
	fakePlugin
	jobs []job.Job
	app  *plugin.App
}

func (p *workerPlugin) Register(app *plugin.App) (*plugin.Registration, error) {
	p.app = app
	return &plugin.Registration{Jobs: p.jobs}, nil
}

// workerQueue scripts deliveries and settlements without clocks or polling.
// Only one consumer accesses it; tests inspect it after Worker has returned.
type workerQueue struct {
	queue.Queue
	pending []*queue.Message
	acked   []*queue.Message
	retried []*queue.Message
	dead    []*queue.Message
	stop    context.CancelFunc
}

func (q *workerQueue) Push(ctx context.Context, msgs ...*queue.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, msg := range msgs {
		copy := *msg
		copy.Payload = append([]byte(nil), msg.Payload...)
		copy.Extra = append([]byte(nil), msg.Extra...)
		q.pending = append(q.pending, &copy)
	}
	return nil
}

func (q *workerQueue) Pop(ctx context.Context, _ string, _ int) ([]*queue.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(q.pending) == 0 {
		q.stop()
		return nil, ctx.Err()
	}
	msg := q.pending[0]
	q.pending = q.pending[1:]
	msg.Attempt++
	msg.Delay = 0
	return []*queue.Message{msg}, nil
}

func (q *workerQueue) Ack(_ context.Context, msgs ...*queue.Message) error {
	q.acked = append(q.acked, msgs...)
	return nil
}

func (q *workerQueue) Retry(ctx context.Context, msgs ...*queue.Message) error {
	q.retried = append(q.retried, msgs...)
	return q.Push(ctx, msgs...)
}

func (q *workerQueue) DeadLetter(_ context.Context, msgs ...*queue.Message) error {
	q.dead = append(q.dead, msgs...)
	return nil
}

func TestWorkerNamespaceRoutingAndFollowUpDispatch(t *testing.T) {
	app, _ := newMockApp(t, "")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	q := &workerQueue{stop: cancel}
	app.queue = q
	seen := make(map[string][]string)
	makeJob := func(namespace string) job.Job {
		return &workerJob{name: "shared", handle: func(app *job.App, payload []byte) error {
			seen[namespace] = append(seen[namespace], string(payload))
			if string(payload) == "parent" {
				return app.Dispatch(ctx, "shared", []byte("child"), job.DispatchConfig{})
			}
			return nil
		}}
	}
	require.NoError(t, app.RegisterJob(makeJob(hostAppNamespace)))
	first := &workerPlugin{fakePlugin: fakePlugin{name: "first"}, jobs: []job.Job{makeJob("first")}}
	second := &workerPlugin{fakePlugin: fakePlugin{name: "second"}, jobs: []job.Job{makeJob("second")}}
	require.NoError(t, app.UsePlugin(first, second))
	require.NoError(t, app.Setup())
	require.NoError(t, app.Boot())
	require.NoError(t, app.Dispatch(ctx, "shared", []byte("parent"), job.DispatchConfig{}))
	require.NoError(t, first.app.Dispatch(ctx, "shared", []byte("parent"), job.DispatchConfig{}))
	require.NoError(t, second.app.Dispatch(ctx, "shared", []byte("parent"), job.DispatchConfig{}))
	require.NoError(t, app.Worker(ctx, WorkerOptions{QueueName: "default", Concurrency: 1}))
	require.ErrorIs(t, ctx.Err(), context.Canceled, "worker should drain before the timeout")
	for _, namespace := range []string{hostAppNamespace, "first", "second"} {
		require.Equal(t, []string{"parent", "child"}, seen[namespace])
	}
	require.Len(t, q.acked, 6)
	for _, msg := range q.acked {
		require.Equal(t, time.Minute, msg.VisibilityTimeout)
	}
	require.Empty(t, q.retried)
	require.Empty(t, q.dead)
}

func TestWorkerNamespaceRetryAndDeadLetter(t *testing.T) {
	app, _ := newMockApp(t, "")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	q := &workerQueue{stop: cancel}
	app.queue = q
	seen := make(map[string]int)
	for _, namespace := range []string{hostAppNamespace, "first", "second"} {
		j := &workerJob{name: "shared", handle: func(*job.App, []byte) error {
			seen[namespace]++
			return errors.New(namespace + " failed")
		}}
		require.NoError(t, app.registerJobForNamespace(namespace, j))
		require.NoError(t, app.dispatch(ctx, namespace, "shared", nil, DispatchConfig{}))
	}
	require.NoError(t, q.Push(ctx, &queue.Message{QueueName: "default", Namespace: "unknown", JobName: "shared"}))
	require.NoError(t, app.Worker(ctx, WorkerOptions{QueueName: "default", Concurrency: 1}))
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Equal(t, map[string]int{hostAppNamespace: 2, "first": 2, "second": 2}, seen)
	require.Empty(t, q.acked)
	require.Len(t, q.retried, 3)
	for _, msg := range q.retried {
		require.Equal(t, time.Second, msg.Delay)
		require.Equal(t, 1, msg.Attempt)
	}
	require.Len(t, q.dead, 4)
	for _, msg := range q.dead {
		var metadata deadLetterMetadata
		require.NoError(t, json.Unmarshal(msg.Extra, &metadata))
		if msg.Namespace == "unknown" {
			require.Equal(t, 1, msg.Attempt)
			require.Contains(t, metadata.Error, `not registered in namespace "unknown"`)
		} else {
			require.Equal(t, 2, msg.Attempt)
			require.Equal(t, msg.Namespace+" failed", metadata.Error)
		}
	}
}

func TestJobDispatchContextErrorsAndIsolation(t *testing.T) {
	app, _ := newMockApp(t, "")
	q := &dispatchQueue{}
	app.queue = q
	var handlerApp *job.App
	j := &workerJob{name: "shared", handle: func(app *job.App, _ []byte) error {
		handlerApp = app
		return nil
	}}
	require.NoError(t, app.registerJobForNamespace("first", j))
	require.NoError(t, app.RegisterJob(&workerJob{name: "host-only"}))
	// Use a settlement-capable queue for the parent delivery.
	settlements := &workerQueue{}
	app.queue = settlements
	app.processMessage(t.Context(), &queue.Message{Namespace: "first", JobName: "shared", Attempt: 1})
	app.queue = q
	require.NotNil(t, handlerApp)
	config := job.DispatchConfig{Delay: time.Second, VisibilityTimeout: 2 * time.Minute}
	require.NoError(t, handlerApp.Dispatch(t.Context(), "shared", []byte("follow-up"), config))
	require.Equal(t, "first", q.messages[0].Namespace)
	require.Equal(t, []byte("follow-up"), q.messages[0].Payload)
	require.Equal(t, config.Delay, q.messages[0].Delay)
	require.Equal(t, config.VisibilityTimeout, q.messages[0].VisibilityTimeout)
	err := handlerApp.Dispatch(t.Context(), "host-only", nil, job.DispatchConfig{})
	require.ErrorContains(t, err, `not found in namespace "first"`)
	require.Len(t, q.contexts, 1, "a foreign job must not reach the queue")
	q.err = errors.New("push failed")
	require.ErrorIs(t, handlerApp.Dispatch(t.Context(), "shared", nil, job.DispatchConfig{}), q.err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, handlerApp.Dispatch(ctx, "shared", nil, job.DispatchConfig{}), context.Canceled)
	require.Equal(t, ctx, q.contexts[len(q.contexts)-1])
}

func TestJobRegistrationValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		job  job.Job
		err  error
	}{
		{name: "nil", err: job.ErrNilJob},
		{name: "typed nil", job: (*workerJob)(nil), err: job.ErrNilJob},
		{name: "empty name", job: &workerJob{}, err: job.ErrEmptyName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newMockApp(t, "")
			original := &workerJob{name: "shared"}
			require.NoError(t, app.RegisterJob(original))
			err := app.RegisterJob(&workerJob{name: "shared"}, tc.job)
			require.ErrorIs(t, err, tc.err)
			require.Len(t, app.Jobs(), 1)
			require.Same(t, original, app.Jobs()[0].Item, "invalid batches must not replace existing jobs")
			p := &workerPlugin{fakePlugin: fakePlugin{name: "invalid"}, jobs: []job.Job{&workerJob{name: "valid"}, tc.job}}
			require.NoError(t, app.UsePlugin(p))
			require.ErrorIs(t, app.Setup(), tc.err)
			require.Empty(t, app.Jobs().GetNamespace("invalid"))
		})
	}
}
