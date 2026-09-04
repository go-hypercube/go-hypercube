package cmd

import (
	"slices"
	"strings"
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

type Namespaced struct {
	Namespace string
	Command
}

func NewNamespaced(namespace string, cmd Command) *Namespaced {
	return &Namespaced{
		Namespace: namespace,
		Command:   cmd,
	}
}

// NamespacedSlice is a slice of namespaced commands.
// It provides convenient methods for grouping, sorting, and querying
// commands by their associated namespace.
type NamespacedSlice []*Namespaced

// GetNamespace returns all commands belonging to the given namespace,
// sorted alphabetically by their Name.
func (s NamespacedSlice) GetNamespace(namespace string) []Command {
	var result []Command
	for _, ns := range s {
		if ns.Namespace == namespace {
			result = append(result, ns.Command)
		}
	}
	slices.SortFunc(result, func(a, b Command) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return result
}

// GroupByNamespace returns a map from namespace to a slice of its commands.
// Each slice is sorted by Name for deterministic output.
func (s NamespacedSlice) GroupByNamespace() map[string][]Command {
	groups := make(map[string][]Command)
	for _, ns := range s {
		groups[ns.Namespace] = append(groups[ns.Namespace], ns.Command)
	}
	// Sort each group's commands by name for deterministic output
	for _, commands := range groups {
		slices.SortFunc(commands, func(a, b Command) int {
			return strings.Compare(a.Name(), b.Name())
		})
	}
	return groups
}

// Sort sorts the NamespacedSlice in place by (Namespace, Command.Name)
// in ascending order.
func (s NamespacedSlice) Sort() {
	slices.SortFunc(s, func(a, b *Namespaced) int {
		if nsCmp := strings.Compare(a.Namespace, b.Namespace); nsCmp != 0 {
			return nsCmp
		}
		return strings.Compare(a.Name(), b.Name())
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
	return s.GetCommand(namespace, name) != nil
}

// GetCommand returns the command registered under namespace with the
// given name, or nil if no such command exists.
//
// The returned value is the *Namespaced wrapper rather than the bare
// Command — since Namespaced embeds Command, it still satisfies the
// Command interface, but callers that need the raw underlying command
// (e.g. for type assertions to a concrete command type) can access it
// via the embedded field.
func (s NamespacedSlice) GetCommand(namespace, name string) Command {
	for _, ns := range s {
		if ns.Namespace == namespace && ns.Name() == name {
			return ns
		}
	}
	return nil
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
		return strings.Compare(a.Name(), b.Name())
	})
	return result
}
