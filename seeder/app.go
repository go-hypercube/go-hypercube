package seeder

import (
	"database/sql"
	"log/slog"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
)

type App struct {
	seeder Seeder

	DB    *sql.DB
	Cache cache.Cache

	Logger *slog.Logger

	container *container.ServiceContainer
}

type Options struct {
	Seeder    Seeder
	Database  *sql.DB
	Cache     cache.Cache
	Logger    *slog.Logger
	Container *container.ServiceContainer
}

func NewAppForSeeder(op *Options) *App {
	return &App{
		seeder:    op.Seeder,
		DB:        op.Database,
		Cache:     op.Cache,
		container: op.Container,
		Logger:    op.Logger,
	}
}
