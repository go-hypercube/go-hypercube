package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-hypercube/go-hypercube/job"
	"github.com/go-hypercube/go-hypercube/plugin"
	"github.com/go-hypercube/go-hypercube/queue"
	"github.com/stretchr/testify/require"
)

type dispatchQueue struct {
	queue.Queue
	contexts []context.Context
	messages []*queue.Message
	err      error
}

func (q *dispatchQueue) Push(ctx context.Context, msgs ...*queue.Message) error {
	q.contexts = append(q.contexts, ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.err != nil {
		return q.err
	}
	q.messages = append(q.messages, msgs...)
	return nil
}

type dispatchJob struct{}

func (dispatchJob) Name() string                     { return "send-email" }
func (dispatchJob) Handle(*job.App, []byte) error    { return nil }
func (dispatchJob) QueueName() string                { return "mail" }
func (dispatchJob) VisibilityTimeout() time.Duration { return time.Minute }

type dispatchPlugin struct {
	fakePlugin
	registeredApp *plugin.App
}

func (p *dispatchPlugin) Register(app *plugin.App) (*plugin.Registration, error) {
	p.registeredApp = app
	return &plugin.Registration{Jobs: []job.Job{dispatchJob{}}}, nil
}

func (p *dispatchPlugin) Boot(app *plugin.App) error {
	return app.Dispatch(context.Background(), "send-email", []byte(p.name), job.DispatchConfig{})
}

func TestPluginDispatch(t *testing.T) {
	app, _ := newMockApp(t, "")
	q := &dispatchQueue{Queue: app.queue}
	app.queue = q
	plugins := []*dispatchPlugin{
		{fakePlugin: fakePlugin{name: "first"}},
		{fakePlugin: fakePlugin{name: "second"}},
	}
	require.NoError(t, app.UsePlugin(plugins[0], plugins[1]))
	require.NoError(t, app.Setup())
	require.NoError(t, app.Boot())
	require.Len(t, q.messages, 2)
	for _, p := range plugins {
		var found bool
		for _, msg := range q.messages {
			if msg.Namespace == p.name {
				found = true
				require.Equal(t, []byte(p.name), msg.Payload)
				require.Equal(t, "send-email", msg.JobName)
				require.Equal(t, "mail", msg.QueueName)
				require.Equal(t, time.Minute, msg.VisibilityTimeout)
			}
		}
		require.True(t, found, "missing dispatch for %s", p.name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	config := job.DispatchConfig{Delay: time.Second, VisibilityTimeout: 2 * time.Minute}
	require.NoError(t, plugins[0].registeredApp.Dispatch(ctx, "send-email", []byte("later"), config))
	require.Equal(t, ctx, q.contexts[2])
	require.Equal(t, "first", q.messages[2].Namespace)
	require.Equal(t, []byte("later"), q.messages[2].Payload)
	require.Equal(t, config.Delay, q.messages[2].Delay)
	require.Equal(t, config.VisibilityTimeout, q.messages[2].VisibilityTimeout)

	err := plugins[0].registeredApp.Dispatch(ctx, "missing", nil, job.DispatchConfig{})
	require.ErrorContains(t, err, `job "missing" not found in namespace "first"`)
	require.Len(t, q.contexts, 3, "missing jobs must not reach the queue")

	q.err = errors.New("queue unavailable")
	err = plugins[0].registeredApp.Dispatch(ctx, "send-email", nil, job.DispatchConfig{})
	require.ErrorIs(t, err, q.err)
	require.ErrorContains(t, err, `dispatch job "send-email" in namespace "first"`)
	cancel()
	err = plugins[0].registeredApp.Dispatch(ctx, "send-email", nil, job.DispatchConfig{})
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, q.messages, 3)
}

type defaultDispatchJob struct{ dispatchJob }

func (defaultDispatchJob) QueueName() string                { return "" }
func (defaultDispatchJob) VisibilityTimeout() time.Duration { return 0 }

func TestFrameworkDispatchCompatibility(t *testing.T) {
	app, _ := newMockApp(t, "")
	q := &dispatchQueue{Queue: app.queue}
	app.queue = q
	require.NoError(t, app.RegisterJob(defaultDispatchJob{}))
	require.NoError(t, app.Dispatch(context.Background(), "send-email", nil, DispatchConfig{}))
	require.Len(t, q.messages, 1)
	require.Equal(t, hostAppNamespace, q.messages[0].Namespace)
	require.Equal(t, "default", q.messages[0].QueueName)
	require.Zero(t, q.messages[0].VisibilityTimeout)
}

func TestPluginDispatchUnavailable(t *testing.T) {
	app := plugin.NewAppForPlugin(&plugin.Options{})
	err := app.Dispatch(context.Background(), "send-email", nil, job.DispatchConfig{})
	require.ErrorIs(t, err, plugin.ErrDispatchUnavailable)
}
