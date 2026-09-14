// Package seeder defines the Seeder contract: a named unit of work that
// populates a database with initial or sample data. Seeders are
// registered per-namespace (framework or plugin) the same way commands
// and migrations are.
package seeder

import "github.com/go-hypercube/go-hypercube/namespaced"

// Seeder is a named, runnable unit of database seeding logic. Run
// receives an *App scoped to this seeder (with access to the database,
// cache, and service container).
type Seeder interface {
	// Name returns the unique identifier for this seeder within its
	// namespace. Used for tracking which seeders have already run.
	Name() string

	// Run executes the seeder's logic.
	Run(*App) error
}

func NewNamespaced(namespace string, s Seeder) *namespaced.Namespaced[Seeder] {
	return &namespaced.Namespaced[Seeder]{
		Namespace: namespace,
		Item:      s,
	}
}
