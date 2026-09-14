// Package namespaced provides a generic wrapper for grouping,
// sorting, and querying registry-like items (commands, migrations,
// seeders, etc.) by an associated namespace (typically a plugin name
// or the framework's own reserved namespace).
package namespaced

import (
	"slices"
	"strings"
)

// Identifiable is anything that can be looked up by a unique name
// within its namespace.
type Identifiable interface {
	Name() string
}

type Namespaced[T Identifiable] struct {
	Namespace string
	Item      T
}

func NewNamespaced[T Identifiable](namespace string, item T) *Namespaced[T] {
	return &Namespaced[T]{Namespace: namespace, Item: item}
}

// NamespacedSlice is a slice of namespaced items. It provides
// convenient methods for grouping, sorting, and querying items by
// their associated namespace.
type NamespacedSlice[T Identifiable] []*Namespaced[T]

// GetNamespace returns all items belonging to the given namespace,
// sorted alphabetically by Name.
func (s NamespacedSlice[T]) GetNamespace(namespace string) []T {
	var result []T
	for _, ns := range s {
		if ns.Namespace == namespace {
			result = append(result, ns.Item)
		}
	}
	slices.SortFunc(result, func(a, b T) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return result
}

// GroupByNamespace returns a map from namespace to its items. Each
// slice is sorted by Name for deterministic output.
func (s NamespacedSlice[T]) GroupByNamespace() map[string][]T {
	groups := make(map[string][]T)
	for _, ns := range s {
		groups[ns.Namespace] = append(groups[ns.Namespace], ns.Item)
	}
	for _, items := range groups {
		slices.SortFunc(items, func(a, b T) int {
			return strings.Compare(a.Name(), b.Name())
		})
	}
	return groups
}

// Sort sorts the NamespacedSlice in place by (Namespace, Name) ascending.
func (s NamespacedSlice[T]) Sort() {
	slices.SortFunc(s, func(a, b *Namespaced[T]) int {
		if nsCmp := strings.Compare(a.Namespace, b.Namespace); nsCmp != 0 {
			return nsCmp
		}
		return strings.Compare(a.Item.Name(), b.Item.Name())
	})
}

// Namespaces returns a sorted list of all unique namespaces present.
func (s NamespacedSlice[T]) Namespaces() []string {
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

// Contains reports whether an item with the given name exists under
// the specified namespace.
func (s NamespacedSlice[T]) Contains(namespace, name string) bool {
	_, ok := s.Get(namespace, name)
	return ok
}

// Get returns the item registered under namespace with the given name,
// and whether it was found.
func (s NamespacedSlice[T]) Get(namespace, name string) (T, bool) {
	for _, ns := range s {
		if ns.Namespace == namespace && ns.Item.Name() == name {
			return ns.Item, true
		}
	}
	var zero T
	return zero, false
}

// GetNamespaces returns all Namespaced entries whose namespace is in
// the provided list, sorted by (Namespace, Name). Returns nil if no
// namespaces are given.
func (s NamespacedSlice[T]) GetNamespaces(namespaces ...string) []*Namespaced[T] {
	if len(namespaces) == 0 {
		return nil
	}
	nsSet := make(map[string]struct{}, len(namespaces))
	for _, ns := range namespaces {
		nsSet[ns] = struct{}{}
	}
	result := make([]*Namespaced[T], 0)
	for _, ns := range s {
		if _, ok := nsSet[ns.Namespace]; ok {
			result = append(result, ns)
		}
	}
	slices.SortFunc(result, func(a, b *Namespaced[T]) int {
		if nsCmp := strings.Compare(a.Namespace, b.Namespace); nsCmp != 0 {
			return nsCmp
		}
		return strings.Compare(a.Item.Name(), b.Item.Name())
	})
	return result
}
