package cmd

import (
	"github.com/go-hypercube/go-hypercube/namespaced"
)

// Command is a named, runnable unit of work registered with the
// framework or a plugin. Run receives an *App scoped to this command
// (with access to the database, cache, and service container) and may
// return an arbitrary result value alongside an error.
type Command interface {
	// Name returns the unique identifier for this command within its
	// namespace.
	Name() string

	// Run executes the command's logic and returns a result value (which
	// may be nil) and an error if execution failed.
	Run(*App) (any, error)
}

func NewNamespaced(namespace string, cmd Command) *namespaced.Namespaced[Command] {
	return &namespaced.Namespaced[Command]{
		Namespace: namespace,
		Item:      cmd,
	}
}
