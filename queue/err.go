package queue

import "errors"

// ErrEmpty is returned by Pop when queueName currently has no visible
// message available. All drivers must return this exact error
// (checkable via errors.Is) instead of a driver-specific empty-queue
// error.
var ErrEmpty = errors.New("queue: no message available")

// ErrNotFound is returned by Ack and Fail when id does not refer to a
// currently-actionable in-flight message. This covers three distinct
// situations under one error:
//   - id never existed on this queue,
//   - id exists but is currently delayed / not yet visible,
//   - id already reached a terminal state via the *other* operation
//     (e.g. Ack is called on an id that was already Failed to
//     dead-letter, or Fail is called on an id that was already Acked).
//
// This is intentional, not a placeholder: callers needing to
// distinguish these cases must track message state themselves — the
// queue driver does not expose it.
var ErrNotFound = errors.New("queue: message not found or not actionable")

// ErrAlreadyAcked is returned by Ack when id has already been
// successfully Acked. It is only returned for a repeated Ack on an
// already-Acked id — Ack on an id that reached a terminal state via
// Fail instead returns ErrNotFound (see ErrNotFound).
var ErrAlreadyAcked = errors.New("queue: message already acked")

// ErrAlreadyFailed is returned by Fail when id has already been marked
// permanently failed (dead-lettered, i.e. its retry attempts are
// exhausted). It is not returned for a Fail that still has retry
// attempts remaining — that call succeeds normally and requeues the
// message. Fail on an id that reached a terminal state via Ack instead
// returns ErrNotFound (see ErrNotFound).
var ErrAlreadyFailed = errors.New("queue: message already failed (dead-lettered)")
