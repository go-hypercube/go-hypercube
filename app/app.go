package app

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/cmd"
	"github.com/go-hypercube/go-hypercube/config"
	"github.com/go-hypercube/go-hypercube/internal/container"
	"github.com/go-hypercube/go-hypercube/internal/structure"
	"github.com/go-hypercube/go-hypercube/migration"
	"github.com/go-hypercube/go-hypercube/plugin"
)

type App struct {
	config     config.Config
	database   *sql.DB
	cache      cache.Cache
	plugins    []plugin.Plugin
	migrations migration.NamespacedSlice
	cmds       cmd.NamespacedSlice
	services   *container.ServiceContainer
}

func New(config config.Config, database *sql.DB, cache cache.Cache) *App {
	return &App{
		config:   config,
		database: database,
		cache:    cache,
		services: container.NewServiceContainer(),
	}
}

func (app *App) Config() config.Config { return app.config }
func (app *App) DB() *sql.DB           { return app.database }
func (app *App) Cache() cache.Cache    { return app.cache }

func (app *App) Setup() error {
	for _, p := range app.plugins {
		registration, err := p.Register(
			plugin.NewAppForPlugin(
				p,
				app.database,
				app.cache,
				app.services,
			),
		)
		if err != nil {
			return err
		}
		err = app.registerMigrationForNamespace(p.Name(), registration.Migrations...)
		if err != nil {
			return err
		}
		err = app.registerCommandForNamespace(p.Name(), registration.Cmds...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (app *App) Boot() error {
	pluginGraph := structure.NewGraph[string]()
	for _, p := range app.plugins {
		pluginGraph.AddVertex(p.Name())
		for _, dep := range p.Dependencies() {
			pluginGraph.AddEdge(p.Name(), dep)
		}
	}
	// check for missing plugins
	if err := validate(pluginGraph); err != nil {
		return err
	}

	return nil
}

func validate(g *structure.Graph[string]) error {
	var missing []string
	for p, deps := range g.Vertices {
		for _, dep := range deps {
			if _, exists := g.Vertices[dep]; !exists {
				missing = append(missing, fmt.Sprintf(
					"plugin '%s' depends on '%s', but '%s' was not added to the app",
					p, dep, dep,
				))
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing plugin dependencies:\n%s", strings.Join(missing, "\n"))
	}
	return nil
}
