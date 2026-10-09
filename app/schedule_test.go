package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/go-hypercube/go-hypercube/job"
	"github.com/go-hypercube/go-hypercube/plugin"
	"github.com/go-hypercube/go-hypercube/queue"
	"github.com/go-hypercube/go-hypercube/scheduler"
	"github.com/stretchr/testify/require"
)

type scheduledDelivery struct {
	ctx context.Context
	msg *queue.Message
}

type scheduledQueue struct {
	queue.Queue
	deliveries chan scheduledDelivery
	block      bool
}

func (q *scheduledQueue) Push(ctx context.Context, msgs ...*queue.Message) error {
	for _, msg := range msgs {
		select {
		case q.deliveries <- scheduledDelivery{ctx: ctx, msg: msg}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if q.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

type schedulingPlugin struct {
	fakePlugin
	registerApp *plugin.App
	bootApp     *plugin.App
	bootErr     error
}

func (p *schedulingPlugin) Register(app *plugin.App) (*plugin.Registration, error) {
	p.registerApp = app
	if err := app.EveryDispatch("0 0 1 1 *", "send-email", []byte(p.name+":register"), job.DispatchConfig{}); err != nil {
		return nil, err
	}
	return &plugin.Registration{Jobs: []job.Job{dispatchJob{}}}, nil
}

func (p *schedulingPlugin) Boot(app *plugin.App) error {
	p.bootApp = app
	if err := app.EveryDispatch("0 0 1 1 *", "send-email", []byte(p.name+":boot"), job.DispatchConfig{}); err != nil {
		return err
	}
	return p.bootErr
}

func TestPluginScheduler(t *testing.T) {
	app, _ := newMockApp(t, "")
	q := &scheduledQueue{Queue: app.queue, deliveries: make(chan scheduledDelivery, 1)}
	app.queue = q
	sc, err := scheduler.New()
	app.scheduler = sc
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sc.Shutdown()) })
	plugins := []*schedulingPlugin{
		{fakePlugin: fakePlugin{name: "first"}},
		{fakePlugin: fakePlugin{name: "second"}},
	}
	require.NoError(t, app.UsePlugin(plugins[0], plugins[1]))
	require.NoError(t, app.Setup())
	require.Len(t, app.scheduler.Jobs(), 2)
	require.NoError(t, app.Boot())
	require.Len(t, app.scheduler.Jobs(), 4)
	require.NoError(t, app.Boot())
	require.Len(t, app.scheduler.Jobs(), 4, "repeated Boot must not duplicate schedules")
	sc.Start()
	config := job.DispatchConfig{Delay: time.Second, VisibilityTimeout: 2 * time.Minute}
	for _, p := range plugins {
		for phase, pluginApp := range map[string]*plugin.App{"register": p.registerApp, "boot": p.bootApp} {
			require.NoError(t, pluginApp.EveryDispatch("0 0 1 1 *", "send-email", []byte(p.name+":"+phase+":later"), config))
		}
	}
	require.Len(t, app.scheduler.Jobs(), 8)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seen := make(map[string]bool)
	for _, scheduled := range app.scheduler.Jobs() {
		require.NoError(t, scheduled.RunNow())
		select {
		case delivery := <-q.deliveries:
			msg := delivery.msg
			key := string(msg.Payload)
			require.False(t, seen[key], "duplicate scheduled payload")
			seen[key] = true
			require.Contains(t, []string{"first", "second"}, msg.Namespace)
			require.Contains(t, []string{msg.Namespace + ":register", msg.Namespace + ":boot", msg.Namespace + ":register:later", msg.Namespace + ":boot:later"}, key)
			require.Equal(t, "send-email", msg.JobName)
			require.Equal(t, "mail", msg.QueueName)
			if key == msg.Namespace+":register" || key == msg.Namespace+":boot" {
				require.Zero(t, msg.Delay)
				require.Equal(t, time.Minute, msg.VisibilityTimeout)
			} else {
				require.Equal(t, config.Delay, msg.Delay)
				require.Equal(t, config.VisibilityTimeout, msg.VisibilityTimeout)
			}
		case <-ctx.Done():
			t.Fatal("scheduled dispatch did not reach the queue")
		}
	}
}

func TestSchedulerBootFailure(t *testing.T) {
	app, _ := newMockApp(t, "")
	sc, err := scheduler.New()
	app.scheduler = sc
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sc.Shutdown()) })
	bootErr := errors.New("plugin boot failed")
	p := &schedulingPlugin{fakePlugin: fakePlugin{name: "broken"}, bootErr: bootErr}
	require.NoError(t, app.UsePlugin(p))
	require.NoError(t, app.Setup())
	require.ErrorIs(t, app.Boot(), bootErr)
	require.False(t, app.didBoot)
}

func TestSchedulerShutdownCancelsDispatch(t *testing.T) {
	app, _ := newMockApp(t, "")
	q := &scheduledQueue{Queue: app.queue, deliveries: make(chan scheduledDelivery, 1), block: true}
	app.queue = q
	require.NoError(t, app.RegisterJob(dispatchJob{}))
	sc, err := scheduler.New()
	app.scheduler = sc
	require.NoError(t, err)
	// Cleanup also covers failures before the explicit shutdown below.
	shutdown := false
	defer func() {
		if !shutdown {
			require.NoError(t, sc.Shutdown())
		}
	}()
	err = app.EveryDispatch("invalid", "send-email", nil, DispatchConfig{})
	require.ErrorIs(t, err, gocron.ErrCronJobParse)
	require.Empty(t, sc.Jobs())
	require.NoError(t, app.EveryDispatch("0 0 1 1 *", "send-email", nil, DispatchConfig{}))
	sc.Start()
	require.NoError(t, sc.Jobs()[0].RunNow())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case delivery := <-q.deliveries:
		require.Equal(t, hostAppNamespace, delivery.msg.Namespace)
		require.Equal(t, time.Minute, delivery.msg.VisibilityTimeout)
		shutdown = true
		require.NoError(t, sc.Shutdown())
		require.ErrorIs(t, delivery.ctx.Err(), context.Canceled)
	case <-ctx.Done():
		t.Fatal("scheduled dispatch did not reach the queue")
	}
}

func TestPluginSchedulerUnavailable(t *testing.T) {
	app := plugin.NewAppForPlugin(&plugin.Options{})
	err := app.EveryDispatch("0 0 1 1 *", "send-email", nil, job.DispatchConfig{})
	require.ErrorIs(t, err, plugin.ErrSchedulerUnavailable)
	err = app.Every("0 0 1 1 *", func(context.Context) error { return nil })
	require.ErrorIs(t, err, plugin.ErrSchedulerUnavailable)
}

func TestPluginSchedulingRequiresHostScheduler(t *testing.T) {
	app, _ := newMockApp(t, "")
	p := &schedulingPlugin{fakePlugin: fakePlugin{name: "missing-scheduler"}}
	require.NoError(t, app.UsePlugin(p))
	require.ErrorIs(t, app.Setup(), ErrSchedulerUnavailable)
	require.Nil(t, app.scheduler)
}

type directSchedulingPlugin struct {
	fakePlugin
	runs chan string
}

func (p *directSchedulingPlugin) Register(app *plugin.App) (*plugin.Registration, error) {
	err := app.Every("0 0 1 1 *", func(ctx context.Context) error {
		select {
		case p.runs <- "register":
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	return &plugin.Registration{}, err
}

func (p *directSchedulingPlugin) Boot(app *plugin.App) error {
	return app.Every("0 0 1 1 *", func(ctx context.Context) error {
		select {
		case p.runs <- "boot":
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

func TestPluginDirectScheduling(t *testing.T) {
	base, _ := newMockApp(t, "")
	sc, err := scheduler.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sc.Shutdown()) })
	q := &dispatchQueue{Queue: base.queue, err: errors.New("queue must not be used")}
	app, err := New(&Options{
		Config: base.config, Database: base.database, Cache: base.cache,
		Logger: base.logger, Queue: q, Scheduler: sc,
	})
	require.NoError(t, err)
	p := &directSchedulingPlugin{fakePlugin: fakePlugin{name: "direct"}, runs: make(chan string, 2)}
	require.NoError(t, app.UsePlugin(p))
	require.NoError(t, app.Setup())
	require.NoError(t, app.Boot())
	require.Empty(t, app.Jobs(), "direct tasks need no job registration")
	sc.Start()
	for _, scheduled := range sc.Jobs() {
		require.NoError(t, scheduled.RunNow())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seen := make(map[string]bool)
	for range 2 {
		select {
		case phase := <-p.runs:
			seen[phase] = true
		case <-ctx.Done():
			t.Fatal("plugin direct callback did not run")
		}
	}
	require.Equal(t, map[string]bool{"register": true, "boot": true}, seen)
	require.Empty(t, q.contexts, "direct tasks must not use the queue")
}
