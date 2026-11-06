package app

import (
	"context"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/go-hypercube/go-hypercube/job"
	"github.com/go-hypercube/go-hypercube/queue"
	"github.com/go-hypercube/go-hypercube/scheduler"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestJobSchedulingNamespaceIsolation(t *testing.T) {
	app, _ := newMockApp(t, "")
	sc, err := scheduler.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sc.Shutdown()) })
	app.scheduler = sc
	settlements := &workerQueue{}
	q := &scheduledQueue{Queue: settlements, deliveries: make(chan scheduledDelivery, 1)}
	app.queue = q
	namespaces := []string{hostAppNamespace, "first", "second"}
	handlers := make(map[string]*job.App)
	directIDs := make(map[string]uuid.UUID)
	dispatchIDs := make(map[string]uuid.UUID)
	directRuns := make(chan string, len(namespaces))
	dispatchConfig := job.DispatchConfig{Delay: time.Second, VisibilityTimeout: 2 * time.Minute}
	for _, namespace := range namespaces {
		directIDs[namespace] = uuid.New()
		dispatchIDs[namespace] = uuid.New()
		j := &workerJob{name: "shared", handle: func(handler *job.App, _ []byte) error {
			handlers[namespace] = handler
			if err := handler.Schedule("0 0 1 1 *", func(context.Context) error {
				directRuns <- namespace
				return nil
			}, scheduler.JobConfig{ID: directIDs[namespace], Name: "callback", Namespace: "foreign", Tags: []string{"maintenance"}}); err != nil {
				return err
			}
			return handler.ScheduleDispatch("0 0 1 1 *", "shared", []byte(namespace), dispatchConfig,
				scheduler.JobConfig{ID: dispatchIDs[namespace], Name: "dispatch", Namespace: "foreign"})
		}}
		if namespace == hostAppNamespace {
			require.NoError(t, app.RegisterJob(j))
		} else {
			require.NoError(t, app.UsePlugin(&workerPlugin{fakePlugin: fakePlugin{name: namespace}, jobs: []job.Job{j}}))
		}
	}
	require.NoError(t, app.Setup())
	require.NoError(t, app.Boot())
	for _, namespace := range namespaces {
		app.processMessage(t.Context(), &queue.Message{Namespace: namespace, JobName: "shared", Attempt: 1})
	}
	require.Len(t, settlements.acked, 3, "all handlers must successfully create their schedules")
	require.Len(t, sc.Jobs(), 6)
	require.Empty(t, sc.JobsInNamespace("foreign"))
	sc.Start()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, namespace := range namespaces {
		handler := handlers[namespace]
		schedules := handler.ScheduledJobs()
		require.Len(t, schedules, 2)
		for _, other := range namespaces {
			if other != namespace {
				require.ErrorIs(t, handler.RemoveScheduledJob(directIDs[other]), gocron.ErrJobNotFound)
			}
		}
		require.ErrorIs(t, handler.Schedule("0 0 1 1 *", func(context.Context) error { return nil },
			scheduler.JobConfig{ID: directIDs[namespace], Name: "duplicate"}), scheduler.ErrDuplicateID)
		require.ErrorIs(t, handler.ScheduleDispatch("invalid", "shared", nil, job.DispatchConfig{},
			scheduler.JobConfig{Name: "invalid"}), gocron.ErrCronJobParse)
		for _, scheduled := range schedules {
			require.Contains(t, scheduled.Tags(), scheduler.NamespaceTag(namespace))
			require.NoError(t, scheduled.RunNow())
			if scheduled.ID() == directIDs[namespace] {
				require.Equal(t, "callback", scheduled.Name())
				require.Contains(t, scheduled.Tags(), "maintenance")
				select {
				case ran := <-directRuns:
					require.Equal(t, namespace, ran)
				case <-ctx.Done():
					t.Fatal("scheduled callback did not run")
				}
			} else {
				require.Equal(t, dispatchIDs[namespace], scheduled.ID())
				select {
				case delivery := <-q.deliveries:
					require.Equal(t, namespace, delivery.msg.Namespace)
					require.Equal(t, "shared", delivery.msg.JobName)
					require.Equal(t, []byte(namespace), delivery.msg.Payload)
					require.Equal(t, dispatchConfig.Delay, delivery.msg.Delay)
					require.Equal(t, dispatchConfig.VisibilityTimeout, delivery.msg.VisibilityTimeout)
				case <-ctx.Done():
					t.Fatal("scheduled dispatch did not reach the queue")
				}
			}
		}
	}
	for _, namespace := range namespaces {
		handler := handlers[namespace]
		require.NoError(t, handler.RemoveScheduledJob(directIDs[namespace]))
		require.NoError(t, handler.RemoveScheduledJob(dispatchIDs[namespace]))
		require.Empty(t, handler.ScheduledJobs())
		require.ErrorIs(t, handler.RemoveScheduledJob(directIDs[namespace]), gocron.ErrJobNotFound)
	}
	require.Empty(t, sc.Jobs())
}

func TestJobSchedulingWithoutHostScheduler(t *testing.T) {
	app, _ := newMockApp(t, "")
	app.queue = &workerQueue{}
	var handler *job.App
	require.NoError(t, app.RegisterJob(&workerJob{name: "capture", handle: func(app *job.App, _ []byte) error {
		handler = app
		return nil
	}}))
	app.processMessage(t.Context(), &queue.Message{Namespace: hostAppNamespace, JobName: "capture", Attempt: 1})
	require.NotNil(t, handler)
	require.ErrorIs(t, handler.Schedule("0 0 1 1 *", func(context.Context) error { return nil }, scheduler.JobConfig{Name: "callback"}), ErrSchedulerUnavailable)
	require.ErrorIs(t, handler.ScheduleDispatch("0 0 1 1 *", "capture", nil, job.DispatchConfig{}, scheduler.JobConfig{Name: "dispatch"}), ErrSchedulerUnavailable)
	require.Empty(t, handler.ScheduledJobs())
	require.ErrorIs(t, handler.RemoveScheduledJob(uuid.New()), ErrSchedulerUnavailable)
}
