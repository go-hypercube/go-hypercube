package cmd

import (
	"database/sql"
	"log/slog"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
)

type App struct {
	cmd Command

	DB    *sql.DB
	Cache cache.Cache

	Logger *slog.Logger

	container *container.ServiceContainer
}

type Options struct {
	Cmd       Command
	Database  *sql.DB
	Cache     cache.Cache
	Logger    *slog.Logger
	Container *container.ServiceContainer
}

func NewAppForCmd(op *Options) *App {
	return &App{
		cmd:       op.Cmd,
		DB:        op.Database,
		Cache:     op.Cache,
		container: op.Container,
		Logger:    op.Logger,
	}
}
