package app

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/cmd"
	"github.com/go-hypercube/go-hypercube/config"
	"github.com/go-hypercube/go-hypercube/internal/container"
	"github.com/go-hypercube/go-hypercube/migration"
	"github.com/go-hypercube/go-hypercube/plugin"
	"github.com/go-hypercube/go-hypercube/seeder"
)

type App struct {
	config     config.Config
	database   *sql.DB
	cache      cache.Cache
	plugins    []plugin.Plugin
	migrations migration.NamespacedSlice
	seeders    seeder.NamespacedSlice
	cmds       cmd.NamespacedSlice
	services   *container.ServiceContainer
	logger     *slog.Logger
	didSetup   bool
	didBoot    bool
}

type Options struct {
	Config   config.Config
	Database *sql.DB
	Cache    cache.Cache
	Logger   *slog.Logger
}

func New(op *Options) *App {
	return &App{
		config:   op.Config,
		database: op.Database,
		cache:    op.Cache,
		logger:   op.Logger,
		services: container.NewServiceContainer(),
	}
}

func (app *App) Config() config.Config { return app.config }
func (app *App) DB() *sql.DB           { return app.database }
func (app *App) Cache() cache.Cache    { return app.cache }
func (app *App) Logger() *slog.Logger  { return app.logger }

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
				&plugin.Options{
					Plugin:    p,
					Database:  app.database,
					Cache:     app.cache,
					Logger:    app.logger.With("plugin", p.Name()),
					Container: app.services,
				},
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
		err = app.registerSeederForNamespace(p.Name(), registration.Seeders...)
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
				&plugin.Options{
					Plugin:    p,
					Database:  app.database,
					Cache:     app.cache,
					Logger:    app.logger.With("plugin", p.Name()),
					Container: app.services,
				},
			),
		)
		if err != nil {
			return err
		}
	}

	app.didBoot = true
	return nil
}
