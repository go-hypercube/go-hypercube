package plugin

import (
	"database/sql"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
)

type App struct {
	plugin Plugin

	DB    *sql.DB
	Cache cache.Cache

	container *container.ServiceContainer
}

func NewAppForPlugin(
	plugin Plugin,
	db *sql.DB,
	cache cache.Cache,
	container *container.ServiceContainer,
) *App {
	return &App{
		plugin:    plugin,
		DB:        db,
		Cache:     cache,
		container: container,
	}
}
