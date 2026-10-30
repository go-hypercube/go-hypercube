package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSchedule(t *testing.T) {
	sc, err := New()
	require.NoError(t, err)
	shutdown := false
	t.Cleanup(func() {
		if !shutdown {
			require.NoError(t, sc.Shutdown())
		}
	})
	_, err = sc.Schedule("0 0 1 1 *", nil, JobConfig{Name: "task", Namespace: "owner"})
	require.ErrorIs(t, err, ErrNilTask)
	_, err = sc.Schedule("invalid", func(context.Context) error { return nil }, JobConfig{Name: "task", Namespace: "owner"})
	require.ErrorIs(t, err, gocron.ErrCronJobParse)
	require.Empty(t, sc.Jobs())
	started := make(chan context.Context, 1)
	_, err = sc.Schedule("0 0 1 1 *", func(ctx context.Context) error {
		started <- ctx
		<-ctx.Done()
		return ctx.Err()
	}, JobConfig{Name: "task", Namespace: "owner"})
	require.NoError(t, err)
	sc.Start()
	require.NoError(t, sc.Jobs()[0].RunNow())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case taskCtx := <-started:
		shutdown = true
		require.NoError(t, sc.Shutdown())
		require.ErrorIs(t, taskCtx.Err(), context.Canceled)
	case <-ctx.Done():
		t.Fatal("direct scheduled task did not run")
	}
}

func TestNewWrapsErrors(t *testing.T) {
	sc, err := New(gocron.WithStopTimeout(-time.Second))
	require.Nil(t, sc)
	require.ErrorIs(t, err, gocron.ErrWithStopTimeoutZeroOrNegative)
}

func TestScheduleOptionsAndRemoval(t *testing.T) {
	sc, err := New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sc.Shutdown()) })
	task := func(context.Context) error { return nil }
	id := uuid.MustParse("87b95dfc-3e71-11ef-9454-0242ac120002")
	_, err = sc.Schedule("0 0 1 1 *", task, JobConfig{
		ID: id, Name: "send-email", Namespace: "mail", Tags: []string{"notifications"},
		Options: []gocron.JobOption{gocron.WithContext(context.Background())},
	})
	require.NoError(t, err)
	jobs := sc.Jobs()
	require.Len(t, jobs, 1)
	require.Equal(t, id, jobs[0].ID())
	require.Equal(t, "send-email", jobs[0].Name())
	require.ElementsMatch(t, []string{NamespaceTag("mail"), "notifications"}, jobs[0].Tags())
	require.NoError(t, sc.RemoveJob(id))
	require.Empty(t, sc.Jobs())
	require.ErrorIs(t, sc.RemoveJob(id), gocron.ErrJobNotFound)

	_, err = sc.Schedule("0 0 1 1 *", task, JobConfig{Name: "task", Namespace: "mail"})
	require.NoError(t, err)
	_, err = sc.Schedule("0 0 1 1 *", task, JobConfig{Name: "task", Namespace: "other"})
	require.NoError(t, err)
	sc.RemoveByTags(NamespaceTag("mail"))
	jobs = sc.Jobs()
	require.Len(t, jobs, 1)
	require.Equal(t, []string{NamespaceTag("other")}, jobs[0].Tags())

	_, err = sc.Schedule("0 0 1 1 *", task, JobConfig{Name: "task", Namespace: "owner", Options: []gocron.JobOption{gocron.WithName("")}})
	require.ErrorIs(t, err, gocron.ErrWithNameEmpty)
	_, err = sc.Schedule("0 0 1 1 *", task, JobConfig{Name: "task", Namespace: "owner", Options: []gocron.JobOption{gocron.WithIdentifier(uuid.Nil)}})
	require.ErrorIs(t, err, gocron.ErrWithIdentifierNil)
	require.Len(t, sc.Jobs(), 1)
}

func TestScheduleMetadata(t *testing.T) {
	sc, err := New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sc.Shutdown()) })
	task := func(context.Context) error { return nil }
	first, err := sc.Schedule("0 0 1 1 *", task, JobConfig{Name: "task", Namespace: "owner"})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, first.ID())
	require.Equal(t, "task", first.Name())
	require.Equal(t, []string{NamespaceTag("owner")}, first.Tags())
	second, err := sc.Schedule("0 0 1 1 *", task, JobConfig{Name: "task", Namespace: "owner"})
	require.NoError(t, err)
	require.NotEqual(t, first.ID(), second.ID())

	id := uuid.New()
	config := JobConfig{
		ID: id, Name: "cleanup", Namespace: "mail", Tags: []string{"maintenance"},
		Options: []gocron.JobOption{gocron.WithName("override"), gocron.WithTags(NamespaceTag("other"))},
	}
	scheduled, err := sc.Schedule("0 0 1 1 *", task, config)
	require.NoError(t, err)
	require.Equal(t, id, scheduled.ID())
	require.Equal(t, "cleanup", scheduled.Name())
	require.Equal(t, []string{NamespaceTag("mail"), "maintenance"}, scheduled.Tags())
	require.Equal(t, []string{"maintenance"}, config.Tags)
	require.Len(t, sc.JobsInNamespace("mail"), 1)
	require.Empty(t, sc.JobsInNamespace("other"))
	_, err = sc.Schedule("0 0 1 1 *", task, config)
	require.ErrorIs(t, err, ErrDuplicateID)
	require.Len(t, sc.Jobs(), 3)
	require.ErrorIs(t, sc.RemoveJobInNamespace("other", id), gocron.ErrJobNotFound)
	require.NoError(t, sc.RemoveJobInNamespace("mail", id))
	require.Empty(t, sc.JobsInNamespace("mail"))

	_, err = sc.Schedule("0 0 1 1 *", nil, JobConfig{Name: "task", Namespace: "owner"})
	require.ErrorIs(t, err, ErrNilTask)
	_, err = sc.Schedule("0 0 1 1 *", task, JobConfig{})
	require.ErrorIs(t, err, ErrEmptyNamespace)
	_, err = sc.Schedule("0 0 1 1 *", task, JobConfig{Name: "task", Namespace: "owner", Tags: []string{NamespaceTag("other")}})
	require.ErrorIs(t, err, ErrReservedTag)
	_, err = sc.Schedule("invalid", task, JobConfig{Name: "task", Namespace: "owner"})
	require.ErrorIs(t, err, gocron.ErrCronJobParse)
	require.Len(t, sc.Jobs(), 2)
}

func TestConcurrentScheduleDuplicateID(t *testing.T) {
	sc, err := New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sc.Shutdown()) })
	id := uuid.New()
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := sc.Schedule("0 0 1 1 *", func(context.Context) error { return nil }, JobConfig{ID: id, Name: "task", Namespace: "owner"})
			results <- err
		}()
	}
	close(start)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	succeeded, duplicate := 0, 0
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				succeeded++
			} else if errors.Is(err, ErrDuplicateID) {
				duplicate++
			} else {
				t.Fatalf("unexpected registration error: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent registration did not finish")
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, duplicate)
	require.Len(t, sc.Jobs(), 1)
}

func TestScheduleRemovalCancelsTask(t *testing.T) {
	sc, err := New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sc.Shutdown()) })
	type contextKey struct{}
	parent := context.WithValue(context.Background(), contextKey{}, "value")
	started := make(chan context.Context, 1)
	stopped := make(chan struct{})
	scheduled, err := sc.Schedule("0 0 1 1 *", func(ctx context.Context) error {
		started <- ctx
		<-ctx.Done()
		close(stopped)
		return nil
	}, JobConfig{Name: "task", Namespace: "mail", Options: []gocron.JobOption{gocron.WithContext(parent)}})
	require.NoError(t, err)
	sc.Start()
	require.NoError(t, scheduled.RunNow())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case taskCtx := <-started:
		require.Equal(t, "value", taskCtx.Value(contextKey{}))
		require.NoError(t, sc.RemoveJobInNamespace("mail", scheduled.ID()))
	case <-ctx.Done():
		t.Fatal("scheduled task did not start")
	}
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("removed task was not canceled")
	}
	require.Empty(t, sc.Jobs())
}

func TestScheduleRequiresName(t *testing.T) {
	sc, err := New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sc.Shutdown()) })
	task := func(context.Context) error { return nil }
	config := JobConfig{ID: uuid.New(), Namespace: "owner", Options: []gocron.JobOption{gocron.WithName("option-name")}}
	scheduled, err := sc.Schedule("0 0 1 1 *", task, config)
	require.Nil(t, scheduled)
	require.ErrorIs(t, err, ErrEmptyName)
	require.Empty(t, sc.Jobs())
}

func TestScheduleOptionalID(t *testing.T) {
	sc, err := New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sc.Shutdown()) })
	task := func(context.Context) error { return nil }
	config := JobConfig{Name: "cleanup", Namespace: "owner"}
	scheduled, err := sc.Schedule("0 0 1 1 *", task, config)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, scheduled.ID())
	require.Equal(t, uuid.Nil, config.ID)
	_, err = sc.Schedule("0 0 1 1 *", task, config)
	require.NoError(t, err)
	jobs := sc.Jobs()
	require.Len(t, jobs, 2)
	require.NotEqual(t, jobs[0].ID(), jobs[1].ID())
}
