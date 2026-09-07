// Package seeder defines the Seeder contract: a named unit of work that
// populates a database with initial or sample data. Seeders are
// registered per-namespace (framework or plugin) the same way commands
// and migrations are.
package seeder

import (
	"slices"
	"strings"
)

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

// Namespaced pairs a Seeder with the namespace it belongs to (a
// plugin's Name(), or the framework's own reserved namespace).
type Namespaced struct {
	Namespace string
	Seeder
}

func NewNamespaced(namespace string, s Seeder) *Namespaced {
	return &Namespaced{Namespace: namespace, Seeder: s}
}

// NamespacedSlice is a slice of namespaced seeders. It provides
// convenient methods for grouping, sorting, and querying seeders by
// their associated namespace, mirroring cmd.NamespacedSlice and
// migration.NamespacedSlice.
type NamespacedSlice []*Namespaced

// GetNamespace returns all seeders belonging to the given namespace,
// sorted alphabetically by their Name.
func (s NamespacedSlice) GetNamespace(namespace string) []Seeder {
	var result []Seeder
	for _, ns := range s {
		if ns.Namespace == namespace {
			result = append(result, ns.Seeder)
		}
	}
	slices.SortFunc(result, func(a, b Seeder) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return result
}

// GroupByNamespace returns a map from namespace to a slice of its
// seeders. Each slice is sorted by Name for deterministic output.
func (s NamespacedSlice) GroupByNamespace() map[string][]Seeder {
	groups := make(map[string][]Seeder)
	for _, ns := range s {
		groups[ns.Namespace] = append(groups[ns.Namespace], ns.Seeder)
	}
	for _, seeders := range groups {
		slices.SortFunc(seeders, func(a, b Seeder) int {
			return strings.Compare(a.Name(), b.Name())
		})
	}
	return groups
}

// Sort sorts the NamespacedSlice in place by (Namespace, Seeder.Name)
// in ascending order.
func (s NamespacedSlice) Sort() {
	slices.SortFunc(s, func(a, b *Namespaced) int {
		if nsCmp := strings.Compare(a.Namespace, b.Namespace); nsCmp != 0 {
			return nsCmp
		}
		return strings.Compare(a.Name(), b.Name())
	})
}

// Namespaces returns a sorted list of all unique namespaces present in
// the slice.
func (s NamespacedSlice) Namespaces() []string {
	seen := make(map[string]struct{})
	result := []string{}
	for _, ns := range s {
		if _, ok := seen[ns.Namespace]; !ok {
			seen[ns.Namespace] = struct{}{}
			result = append(result, ns.Namespace)
		}
	}
	slices.Sort(result)
	return result
}

// Contains reports whether a seeder with the given name exists under
// the specified namespace.
func (s NamespacedSlice) Contains(namespace, name string) bool {
	for _, ns := range s {
		if ns.Namespace == namespace && ns.Name() == name {
			return true
		}
	}
	return false
}

// GetSeeder returns the seeder registered under namespace with the
// given name, or nil if no such seeder exists.
func (s NamespacedSlice) GetSeeder(namespace, name string) Seeder {
	for _, ns := range s {
		if ns.Namespace == namespace && ns.Name() == name {
			return ns.Seeder
		}
	}
	return nil
}

// GetNamespaces returns all Namespaced entries whose namespace is in
// the provided list, sorted by (Namespace, Seeder.Name). If no
// namespaces are provided, it returns nil.
func (s NamespacedSlice) GetNamespaces(namespaces ...string) []*Namespaced {
	if len(namespaces) == 0 {
		return nil
	}
	nsSet := make(map[string]struct{}, len(namespaces))
	for _, ns := range namespaces {
		nsSet[ns] = struct{}{}
	}
	result := make([]*Namespaced, 0)
	for _, ns := range s {
		if _, ok := nsSet[ns.Namespace]; ok {
			result = append(result, ns)
		}
	}
	slices.SortFunc(result, func(a, b *Namespaced) int {
		if nsCmp := strings.Compare(a.Namespace, b.Namespace); nsCmp != 0 {
			return nsCmp
		}
		return strings.Compare(a.Name(), b.Name())
	})
	return result
}
