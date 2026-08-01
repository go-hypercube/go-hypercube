// Package cache defines the portable cache contract used by the framework
// and implemented by driver packages (Redis, Memcached, in-memory, etc).
// It intentionally has no dependency on the rest of the framework, so a
// driver repository only needs to import this package — not the whole
// framework module — to implement Cache. This mirrors the split between
// database/sql and database/sql/driver.
package cache

import (
	"context"
	"time"
)

// Cache is the minimal, backend-agnostic set of operations a caching
// layer must support. It covers plain key/value caching only — plugins
// that need Redis-specific data structures (hashes, lists, sets, sorted
// sets) should Resolve a narrower store interface, or the raw client,
// instead of depending on Cache.
//
// Implementations must be safe for concurrent use.
type Cache interface {
	// Get returns the value stored at key. It returns ErrNotFound
	// (checkable via errors.Is) if key does not exist or has expired.
	Get(ctx context.Context, key string) (string, error)

	// Set stores value at key. If ttl is zero, the key never expires.
	// If ttl is negative, Set returns ErrInvalidTTL. Set overwrites any
	// existing value and TTL at key.
	Set(ctx context.Context, key string, value string, ttl time.Duration) error

	// Delete removes key. It is not an error if key does not exist.
	Delete(ctx context.Context, key string) error

	// Has reports whether key currently exists and has not expired.
	// It never returns ErrNotFound — a missing key is (false, nil).
	Has(ctx context.Context, key string) (bool, error)

	// Increment adds delta to the integer value stored at key and
	// returns the result. If key does not exist, it is created as if
	// it were 0 before applying delta (matching Redis' INCRBY
	// semantics) — implementations must not return ErrNotFound here.
	// If the existing value at key is not a base-10 integer, Increment
	// returns an error.
	Increment(ctx context.Context, key string, delta int64) (int64, error)

	// Expire sets a new TTL on an existing key. It returns ErrNotFound
	// if key does not exist, and ErrInvalidTTL if ttl is negative. A
	// ttl of zero clears any existing expiration.
	Expire(ctx context.Context, key string, ttl time.Duration) error
}
