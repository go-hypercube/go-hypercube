package plugin

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
	"github.com/go-hypercube/go-hypercube/job"
	"github.com/go-hypercube/go-hypercube/scheduler"
)

type App struct {
	plugin Plugin

	DB    *sql.DB
	Cache cache.Cache

	Logger *slog.Logger

	container          *container.ServiceContainer
	dispatch           func(context.Context, string, []byte, job.DispatchConfig) error
	schedule           func(string, func(context.Context) error, scheduler.JobConfig) error
	scheduleDispatch   func(string, string, []byte, job.DispatchConfig, scheduler.JobConfig) error
	scheduledJobs      func() []gocron.Job
	removeScheduledJob func(uuid.UUID) error
}

type Options struct {
	Plugin    Plugin
	Database  *sql.DB
	Cache     cache.Cache
	Logger    *slog.Logger
	Container *container.ServiceContainer
	// Dispatch enqueues a job in this plugin's namespace. Supplied by the framework.
	Dispatch func(context.Context, string, []byte, job.DispatchConfig) error
	// Schedule schedules a callback for direct execution. Supplied by the framework.
	Schedule func(string, func(context.Context) error, scheduler.JobConfig) error
	// ScheduleDispatch schedules enqueueing a job in this plugin's namespace.
	ScheduleDispatch func(string, string, []byte, job.DispatchConfig, scheduler.JobConfig) error
	// ScheduledJobs lists only this plugin's schedules. Supplied by the framework.
	ScheduledJobs func() []gocron.Job
	// RemoveScheduledJob removes only this plugin's schedules.
	RemoveScheduledJob func(uuid.UUID) error
}

func NewAppForPlugin(op *Options) *App {
	return &App{
		plugin:             op.Plugin,
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
