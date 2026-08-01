// Package container implements a generic, type-keyed service container
// for storing and retrieving arbitrary values by type, with optional
// named scoping.
package container

import (
	"fmt"
	"reflect"
	"sync"
)

// containerKey uniquely identifies a bound service by its concrete type
// and an optional name, allowing multiple bindings of the same type to
// coexist (for example two implementations of the same interface).
type containerKey struct {
	kind reflect.Type
	name string
}

// ServiceContainer holds runtime bindings that can be registered under a
// type and an optional name, then looked up later by the same type and
// name.
//
// ServiceContainer is safe for concurrent use.
type ServiceContainer struct {
	mutex sync.RWMutex
	items map[containerKey]any
}

// NewServiceContainer returns an empty, ready-to-use ServiceContainer.
func NewServiceContainer() *ServiceContainer {
	return &ServiceContainer{items: make(map[containerKey]any)}
}

// bind stores instance under kind and name, overwriting any existing
// binding for the same pair. It is the untyped primitive behind the
// generic Bind function.
func (c *ServiceContainer) bind(kind reflect.Type, name string, instance any) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.items[containerKey{kind, name}] = instance
}

// resolve looks up a value by kind and name. It is the untyped primitive
// behind the generic Resolve and TryResolve functions.
func (c *ServiceContainer) resolve(kind reflect.Type, name string) (any, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, found := c.items[containerKey{kind, name}]
	return value, found
}

// typeOf returns the reflect.Type describing the type parameter T,
// including interface types.
func typeOf[T any]() reflect.Type { return reflect.TypeFor[T]() }

// optionalName returns the first element of name if provided, or the
// empty string otherwise. It backs the variadic "optional name" parameter
// used by Bind, Resolve, and TryResolve, since Go has no native optional
// arguments.
func optionalName(name []string) string {
	if len(name) > 0 {
		return name[0]
	}
	return ""
}

// Bind registers instance in c under type T, optionally scoped by name.
// A later Bind with the same type and name overwrites the previous
// binding.
func Bind[T any](c *ServiceContainer, instance T, name ...string) {
	c.bind(typeOf[T](), optionalName(name), instance)
}

// Resolve looks up a value of type T previously registered with Bind,
// optionally scoped by name. Resolve panics if no matching binding
// exists — use TryResolve instead if the binding may legitimately be
// absent.
func Resolve[T any](c *ServiceContainer, name ...string) T {
	value, found := c.resolve(typeOf[T](), optionalName(name))
	if !found {
		panic(fmt.Sprintf("container: %s (name=%q) not bound", typeOf[T](), optionalName(name)))
	}
	return value.(T)
}

// TryResolve looks up a value of type T, optionally scoped by name, and
// reports whether it was found instead of panicking.
func TryResolve[T any](c *ServiceContainer, name ...string) (T, bool) {
	value, found := c.resolve(typeOf[T](), optionalName(name))
	if !found {
		var zero T
		return zero, false
	}
	return value.(T), true
}
