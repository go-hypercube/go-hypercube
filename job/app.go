package job

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/go-co-op/gocron/v2"
	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
	"github.com/go-hypercube/go-hypercube/scheduler"
	"github.com/google/uuid"
)

type App struct {
	job Job

	DB    *sql.DB
	Cache cache.Cache

	Logger *slog.Logger

	container          *container.ServiceContainer
	dispatch           func(context.Context, string, []byte, DispatchConfig) error
	schedule           func(string, func(context.Context) error, scheduler.JobConfig) error
	scheduleDispatch   func(string, string, []byte, DispatchConfig, scheduler.JobConfig) error
	scheduledJobs      func() []gocron.Job
	removeScheduledJob func(uuid.UUID) error
}

type Options struct {
	Job       Job
	Database  *sql.DB
	Cache     cache.Cache
	Logger    *slog.Logger
	Container *container.ServiceContainer
	// Dispatch enqueues a job in the running job's namespace. Supplied by the framework.
	Dispatch func(context.Context, string, []byte, DispatchConfig) error
	// Schedule schedules a callback in the running job's namespace. Supplied by the framework.
	Schedule func(string, func(context.Context) error, scheduler.JobConfig) error
	// ScheduleDispatch schedules enqueueing in the running job's namespace.
	ScheduleDispatch func(string, string, []byte, DispatchConfig, scheduler.JobConfig) error
	// ScheduledJobs lists schedules in the running job's namespace.
	ScheduledJobs func() []gocron.Job
	// RemoveScheduledJob removes only schedules in the running job's namespace.
	RemoveScheduledJob func(uuid.UUID) error
}

func NewAppForJob(op *Options) *App {
	return &App{
		job:                op.Job,
		DB:                 op.Database,
		Cache:              op.Cache,
		container:          op.Container,
		Logger:             op.Logger,
		dispatch:           op.Dispatch,
		schedule:           op.Schedule,
		scheduleDispatch:   op.ScheduleDispatch,
		scheduledJobs:      op.ScheduledJobs,
		removeScheduledJob: op.RemoveScheduledJob,
	}
}
