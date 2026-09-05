package seeder

import (
	"database/sql"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
)

// App is the scoped context passed to a Seeder's Run method, mirroring
// cmd.App and plugin.App.
type App struct {
	seeder Seeder

	DB    *sql.DB
	Cache cache.Cache

	container *container.ServiceContainer
}

func NewAppForSeeder(
	seeder Seeder,
	db *sql.DB,
	cache cache.Cache,
	container *container.ServiceContainer,
) *App {
	return &App{
		seeder:    seeder,
		DB:        db,
		Cache:     cache,
		container: container,
	}
}
