package cache

import "errors"

// ErrNotFound is returned by Get when key does not exist, or has expired.
// All drivers must return this exact error (or wrap it, checkable via
// errors.Is) instead of a driver-specific miss error, so code written
// against Cache can check for a miss portably regardless of which driver
// is wired in.
var ErrNotFound = errors.New("cache: key not found")

// ErrInvalidTTL is returned by Set or Expire when a negative duration is
// given. A ttl of zero is valid and means "no expiration."
var ErrInvalidTTL = errors.New("cache: ttl must be zero or positive")
