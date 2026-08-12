package cmd

import (
	"database/sql"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
)

type App struct {
	cmd Command

	DB    *sql.DB
	Cache cache.Cache

	container *container.ServiceContainer
}

func NewAppForCmd(
	cmd Command,
	db *sql.DB,
	cache cache.Cache,
	container *container.ServiceContainer,
) *App {
	return &App{
		cmd:       cmd,
		DB:        db,
		Cache:     cache,
		container: container,
	}
}
