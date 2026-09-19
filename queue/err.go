package queue

import "errors"

// ErrEmpty is returned by Pop when queueName currently has no visible
// message available. All drivers must return this exact error
// (checkable via errors.Is) instead of a driver-specific empty-queue
// error.
var ErrEmpty = errors.New("queue: no message available")

// ErrNotFound is returned by Ack, Retry, and DeadLetter when no
// in-flight message matches the given id. This can happen when the
// message was already acked, retried, or dead-lettered, or after a
// worker crash/restart that left stale state behind.
//
// This is the ONLY sentinel error in the package. Drivers MUST return
// it (directly or via errors.Is) for missing messages, so callers can
// distinguish "message already handled" from infrastructure failure.
var ErrNotFound = errors.New("queue: message not found")
