package cmd

import (
	"database/sql"

	"github.com/go-hypercube/go-hypercube/cache"
	"github.com/go-hypercube/go-hypercube/internal/container"
)

type Command interface {
	Name() string
	Run(App) error
}

// App is the subset of framework capabilities exposed to Cmds. It is
// implemented by the framework's concrete App type. This package does not
// import the framework package — the framework imports this one — which
// is what breaks the natural cycle between "App holds a list of Cmds"
// and "Cmd methods receive an App."
type App interface {
	DB() *sql.DB
	Cache() cache.Cache

	Container() *container.ServiceContainer
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
