package hypercube

import (
	"database/sql"
	"embed"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/cmd"
	"github.com/go-hypercube/go-hypercube/config"
	"github.com/go-hypercube/go-hypercube/internal/container"
	"github.com/go-hypercube/go-hypercube/migration"
	"github.com/go-hypercube/go-hypercube/plugin"
)

// Compile-time check that *App satisfies plugin.App, cmd.App. If this fails
// to compile, App is missing (or has mismatched signatures for) a method
// required by plugin.App or cmd.App.
var (
	_ plugin.App = (*App)(nil)
	_ cmd.App    = (*App)(nil)
)

type App struct {
	config     config.Config
	database   *sql.DB
	cache      cache.Cache
	plugins    []plugin.Plugin
	migrations []migration.Migration
	cmds       []cmd.Command
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

func (app *App) UsePlugin(plugins ...plugin.Plugin) {
	app.plugins = append(app.plugins, plugins...)
}
func (app *App) Plugins() []plugin.Plugin { return app.plugins }

func (app *App) RegisterMigration(migrations ...migration.Migration) error {
	app.migrations = append(app.migrations, migrations...)
	return nil
}

func (app *App) RegisterMigrationFromFs(files embed.FS) error {
	migrationFiles, err := migration.ExtractFromEmbedFs(files)
	if err != nil {
		return err
	}
	return app.RegisterMigration(migrationFiles...)
}

func (app *App) Migrations() []migration.Migration { return app.migrations }

func (app *App) Container() *container.ServiceContainer { return app.services }

func (app *App) RegisterCommand(cmds ...cmd.Command) {
	app.cmds = append(app.cmds, cmds...)
}

func (app *App) Bootstrap() error {
	for _, p := range app.plugins {
		if err := p.Register(app); err != nil {
			return err
		}
	}
	return nil
}

// Bind registers instance under type T in app's service container,
// optionally scoped by name. A later Bind with the same type and name
// overwrites the previous binding.
func Bind[T any](app *App, instance T, name ...string) {
	container.Bind(app.Container(), instance, name...)
}

// Resolve looks up a value of type T previously registered with Bind,
// optionally scoped by name. Resolve panics if no matching binding
// exists — use TryResolve instead if the binding may legitimately be
// absent.
func Resolve[T any](app *App, name ...string) T {
	return container.Resolve[T](app.Container(), name...)
}

// TryResolve looks up a value of type T, optionally scoped by name, and
// reports whether it was found instead of panicking.
func TryResolve[T any](app *App, name ...string) (T, bool) {
	return container.TryResolve[T](app.Container(), name...)
}
