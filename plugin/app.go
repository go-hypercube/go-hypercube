package plugin

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
	"github.com/go-hypercube/go-hypercube/job"
)

type App struct {
	plugin Plugin

	DB    *sql.DB
	Cache cache.Cache

	Logger *slog.Logger

	container     *container.ServiceContainer
	dispatch      func(context.Context, string, []byte, job.DispatchConfig) error
	every         func(string, func(context.Context) error) error
	everyDispatch func(string, string, []byte, job.DispatchConfig) error
}

type Options struct {
	Plugin    Plugin
	Database  *sql.DB
	Cache     cache.Cache
	Logger    *slog.Logger
	Container *container.ServiceContainer
	// Dispatch enqueues a job in this plugin's namespace. Supplied by the framework.
	Dispatch func(context.Context, string, []byte, job.DispatchConfig) error
	// Every schedules a callback for direct execution. Supplied by the framework.
	Every func(string, func(context.Context) error) error
	// EveryDispatch schedules enqueueing a job in this plugin's namespace.
	EveryDispatch func(string, string, []byte, job.DispatchConfig) error
}

func NewAppForPlugin(op *Options) *App {
	return &App{
		plugin:        op.Plugin,
		DB:            op.Database,
		Cache:         op.Cache,
		container:     op.Container,
		Logger:        op.Logger,
		dispatch:      op.Dispatch,
		every:         op.Every,
		everyDispatch: op.EveryDispatch,
	}
}
