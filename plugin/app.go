package plugin

import (
	"database/sql"
	"log/slog"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
)

type App struct {
	plugin Plugin

	DB    *sql.DB
	Cache cache.Cache

	Logger *slog.Logger

	container *container.ServiceContainer
}

type Options struct {
	Plugin    Plugin
	Database  *sql.DB
	Cache     cache.Cache
	Logger    *slog.Logger
	Container *container.ServiceContainer
}

func NewAppForPlugin(op *Options) *App {
	return &App{
		plugin:    op.Plugin,
		DB:        op.Database,
		Cache:     op.Cache,
		container: op.Container,
		Logger:    op.Logger,
	}
}
