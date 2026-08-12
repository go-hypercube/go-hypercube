package cmd

import "github.com/go-hypercube/go-hypercube/internal/container"

// Resolve looks up a value of type T previously registered with Bind,
// optionally scoped by name. Resolve panics if no matching binding
// exists — use TryResolve instead if the binding may legitimately be
// absent.
func Resolve[T any](app App, name ...string) T {
	return container.Resolve[T](app.container, name...)
}

// TryResolve looks up a value of type T, optionally scoped by name, and
// reports whether it was found instead of panicking.
func TryResolve[T any](app App, name ...string) (T, bool) {
	return container.TryResolve[T](app.container, name...)
}
