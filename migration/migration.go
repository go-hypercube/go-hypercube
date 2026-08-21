package migration

import (
	"slices"
	"strings"
)

type Migration struct {
	Name string
	Up   []string
	Down []string
}

type Namespaced struct {
	Namespace string
	*Migration
}

func NewNamespaced(namespace string, migration *Migration) *Namespaced {
	return &Namespaced{
		Namespace: namespace,
		Migration: migration,
	}
}

// NamespacedSlice is a slice of namespaced migrations.
// It provides convenient methods for grouping, sorting, and querying
// migrations by their associated namespace.
type NamespacedSlice []*Namespaced

// GetNamespace returns all migrations belonging to the given namespace,
// sorted alphabetically by their Name.
func (s NamespacedSlice) GetNamespace(namespace string) []*Migration {
	var result []*Migration
	for _, ns := range s {
		if ns.Namespace == namespace {
			result = append(result, ns.Migration)
		}
	}
	slices.SortFunc(result, func(a, b *Migration) int {
		return strings.Compare(a.Name, b.Name)
	})
	return result
}

// GroupByNamespace returns a map from namespace to a slice of its migrations.
// Each slice is sorted by Name for deterministic output.
func (s NamespacedSlice) GroupByNamespace() map[string][]*Migration {
	groups := make(map[string][]*Migration)
	for _, ns := range s {
		groups[ns.Namespace] = append(groups[ns.Namespace], ns.Migration)
	}
	// Sort each group's migrations by name for deterministic output
	for _, migrations := range groups {
		slices.SortFunc(migrations, func(a, b *Migration) int {
			return strings.Compare(a.Name, b.Name)
		})
	}
	return groups
}

// Sort sorts the NamespacedSlice in place by (Namespace, Migration.Name)
// in ascending order.
func (s NamespacedSlice) Sort() {
	slices.SortFunc(s, func(a, b *Namespaced) int {
		if nsCmp := strings.Compare(a.Namespace, b.Namespace); nsCmp != 0 {
			return nsCmp
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// Namespaces returns a sorted list of all unique namespaces present in the slice.
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

// Contains reports whether a migration with the given name exists
// under the specified namespace.
func (s NamespacedSlice) Contains(namespace, name string) bool {
	for _, ns := range s {
		if ns.Namespace == namespace && ns.Name == name {
			return true
		}
	}
	return false
}

// GetNamespaces returns all Namespaced entries whose namespace is in the
// provided list. The result is sorted by (Namespace, Migration.Name).
// If no namespaces are provided, it returns nil.
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
	// Sort by namespace then name
	slices.SortFunc(result, func(a, b *Namespaced) int {
		if nsCmp := strings.Compare(a.Namespace, b.Namespace); nsCmp != 0 {
			return nsCmp
		}
		return strings.Compare(a.Name, b.Name)
	})
	return result
}
