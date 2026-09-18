package job

import (
	"database/sql"
	"log/slog"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
	"github.com/go-hypercube/go-hypercube/queue"
)

type App struct {
	job Job

	DB    *sql.DB
	Cache cache.Cache
	Queue queue.Queue // lets a job dispatch follow-up jobs

	Logger *slog.Logger

	container *container.ServiceContainer
}

type Options struct {
	Job       Job
	Database  *sql.DB
	Cache     cache.Cache
	Queue     queue.Queue
	Logger    *slog.Logger
	Container *container.ServiceContainer
}

func NewAppForJob(op *Options) *App {
	return &App{
		job:       op.Job,
		DB:        op.Database,
		Cache:     op.Cache,
		Queue:     op.Queue,
		container: op.Container,
		Logger:    op.Logger,
	}
}
