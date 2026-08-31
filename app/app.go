package app

import (
	"database/sql"
	"fmt"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/cmd"
	"github.com/go-hypercube/go-hypercube/config"
	"github.com/go-hypercube/go-hypercube/internal/container"
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
	didSetup   bool
	didBoot    bool
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
	if app.didSetup {
		return nil
	}

	err := app.initPlugins()
	if err != nil {
		return err
	}

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

	app.didSetup = true
	return nil
}

func (app *App) Boot() error {
	if !app.didSetup {
		return fmt.Errorf("cannot boot the framework before setting it up; did you forget to call Setup()")
	}
	if app.didBoot {
		return nil
	}

	for _, p := range app.plugins {
		err := p.Boot(
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
	}

	app.didBoot = true
	return nil
}
