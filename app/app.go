package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/cmd"
	"github.com/go-hypercube/go-hypercube/config"
	"github.com/go-hypercube/go-hypercube/internal/container"
	"github.com/go-hypercube/go-hypercube/job"
	"github.com/go-hypercube/go-hypercube/migration"
	"github.com/go-hypercube/go-hypercube/namespaced"
	"github.com/go-hypercube/go-hypercube/plugin"
	"github.com/go-hypercube/go-hypercube/queue"
	"github.com/go-hypercube/go-hypercube/scheduler"
	"github.com/go-hypercube/go-hypercube/seeder"
)

type App struct {
	config config.Config

	database *sql.DB
	cache    cache.Cache

	plugins []plugin.Plugin

	migrations namespaced.NamespacedSlice[*migration.Migration]
	seeders    namespaced.NamespacedSlice[seeder.Seeder]
	cmds       namespaced.NamespacedSlice[cmd.Command]
	jobs       namespaced.NamespacedSlice[job.Job]

	queue    queue.Queue
	services *container.ServiceContainer
	logger   *slog.Logger

	didSetup bool
	didBoot  bool

	scheduler *scheduler.Scheduler
}

func New(op *Options) (*App, error) {
	if err := op.validate(); err != nil {
		return nil, err
	}
	return &App{
		config:    op.Config,
		database:  op.Database,
		cache:     op.Cache,
		logger:    op.Logger,
		queue:     op.Queue,
		services:  container.NewServiceContainer(),
		scheduler: op.Scheduler,
	}, nil
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
		namespace := p.Name()
		registration, err := p.Register(
			plugin.NewAppForPlugin(
				&plugin.Options{
					Plugin:    p,
					Database:  app.database,
					Cache:     app.cache,
					Logger:    app.logger.With("plugin", namespace),
					Container: app.services,
					Dispatch: func(ctx context.Context, name string, payload []byte, config job.DispatchConfig) error {
						return app.dispatch(ctx, namespace, name, payload, config)
					},
					Every: app.Every,
					EveryDispatch: func(crontab, name string, payload []byte, config job.DispatchConfig) error {
						return app.everyDispatch(crontab, namespace, name, payload, config)
					},
				},
			),
		)
		if err != nil {
			return err
		}
		err = app.registerMigrationForNamespace(namespace, registration.Migrations...)
		if err != nil {
			return err
		}
		err = app.registerCommandForNamespace(namespace, registration.Cmds...)
		if err != nil {
			return err
		}
		err = app.registerSeederForNamespace(namespace, registration.Seeders...)
		if err != nil {
			return err
		}
		err = app.registerJobForNamespace(namespace, registration.Jobs...)
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
		namespace := p.Name()
		err := p.Boot(
			plugin.NewAppForPlugin(
				&plugin.Options{
					Plugin:    p,
					Database:  app.database,
					Cache:     app.cache,
					Logger:    app.logger.With("plugin", p.Name()),
					Container: app.services,
					Dispatch: func(ctx context.Context, name string, payload []byte, config job.DispatchConfig) error {
						return app.dispatch(ctx, namespace, name, payload, config)
					},
					Every: app.Every,
					EveryDispatch: func(crontab, name string, payload []byte, config job.DispatchConfig) error {
						return app.everyDispatch(crontab, namespace, name, payload, config)
					},
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

type Options struct {
	// Scheduler is optional. Its owner controls Start and Shutdown.
	Scheduler *scheduler.Scheduler
	Queue     queue.Queue
	Config    config.Config
	Database  *sql.DB
	Cache     cache.Cache
	Logger    *slog.Logger
}

func (o Options) validate() error {
	if o.Queue == nil {
		return errors.New("queue is required")
	}
	if o.Config == nil {
		return errors.New("config is required")
	}
	if o.Database == nil {
		return errors.New("database is required")
	}
	if o.Cache == nil {
		return errors.New("cache is required")
	}
	if o.Logger == nil {
		return errors.New("logger is required")
	}
	return nil
}
