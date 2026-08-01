package plugin

import (
	"database/sql"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
	"github.com/go-hypercube/go-hypercube/migration"
)

// App is the subset of framework capabilities exposed to plugins. It is
// implemented by the framework's concrete App type. This package does not
// import the framework package — the framework imports this one — which
// is what breaks the natural cycle between "App holds a list of Plugins"
// and "Plugin methods receive an App."
type App interface {
	DB() *sql.DB
	Cache() cache.Cache

	RegisterMigration(migrations ...migration.Migration)

	Container() *container.ServiceContainer
}

type Plugin interface {
	ID() string
	Name() string
	Register(app App)
	Boot(app App)
}

// Bind registers instance under type T in app's service container,
// optionally scoped by name. A later Bind with the same type and name
// overwrites the previous binding.
func Bind[T any](app App, instance T, name ...string) {
	container.Bind[T](app.Container(), instance, name...)
}

// Resolve looks up a value of type T previously registered with Bind,
// optionally scoped by name. Resolve panics if no matching binding
// exists — use TryResolve instead if the binding may legitimately be
// absent.
func Resolve[T any](app App, name ...string) T {
	return container.Resolve[T](app.Container(), name...)
}

// TryResolve looks up a value of type T, optionally scoped by name, and
// reports whether it was found instead of panicking.
func TryResolve[T any](app App, name ...string) (T, bool) {
	return container.TryResolve[T](app.Container(), name...)
}
