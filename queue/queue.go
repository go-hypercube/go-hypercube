// Package queue defines the portable job-transport contract used by the
// framework and implemented by driver packages. It has no dependency on
// the rest of the framework
package queue

import (
	"context"
	"time"
)

// Queue is the storage contract for a job queue driver.
//
// # Responsibilities
//
// A Queue implementation is a DUMB EXECUTOR. It stores messages,
// delivers them, and performs exactly the state transition it is told
// to perform. It holds NO opinions about retry policy, attempt
// limits, or when a message is "permanently failed" — all decisions
// belong to the caller (the app/manager layer), which knows each
// job's individual retry policy.
//
// # Message life-cycle / state transitions
//
//	Push              -> pending
//	Pop               -> pending -> in-flight   (Attempt incremented)
//	Ack               -> in-flight -> done      (removed; terminal)
//	Retry 			  -> in-flight -> pending   (re-added)
//	DeadLetter		  -> in-flight -> failed   	(removed; terminal)
//
// There is no conditional transition and no "last attempt" — the
// driver never decides that a message is exhausted. If the caller
// wants a message gone, it calls Ack or DeadLetter.
//
// # Delivery semantics
//
// Delivery is AT-LEAST-ONCE. A driver MAY redeliver a message that is
// still in-flight (e.g. after a visibility timeout or worker crash).
// Handlers MUST therefore be idempotent.
//
// ## Delivery guarantees: at-least-once
//
// The queue guarantees a message is delivered **at least once** — never
// zero times, but possibly more than once.
//
// ### Why duplicates happen
//
// A worker crashes (or loses connection) *after* doing the work but
// *before* sending the ack:
//
//	Worker                          Queue
//	  │ pop ✓                        │
//	  │ ... runs the job ...         │
//	  │            ✗ CRASH           │  ← ack never sent
//	                                 │ message still "in-flight"
//	                                 │ → redelivered to another worker
//
// The queue cannot tell "job finished" from "worker died", so it must
// redeliver. Result: the job may run twice.
//
// ### Rule for handler authors
//
// Handlers MUST be idempotent: running twice must have the same effect
// as running once.
//
// # Concurrency
//
// Implementations MUST be safe for concurrent use by multiple
// goroutines.
type Queue interface {
	// Push enqueues msg. The driver MUST assign msg.ID if it is not
	// already set. If delay > 0, the message becomes eligible for
	// delivery only after the delay has elapsed.
	//
	// Push does not validate queue names, namespaces, or jobs — the
	// caller is responsible for that.
	Push(ctx context.Context, msg *Message, delay time.Duration) error

	// Pop removes and returns the next available message from the
	// named queue, moving it to in-flight.
	//
	// The driver MUST increment msg.Attempt before returning it.
	//
	// If the queue is empty, Pop returns (nil, ErrEmpty) — an empty queue
	// is not an error. Callers that want streaming semantics should
	// implement PopStream instead of Pop.
	Pop(ctx context.Context, queueName string) (*Message, error)

	// Ack permanently removes an in-flight message, marking it as
	// successfully processed.
	//
	// Ack is a command, not a decision: the caller decided the
	// message succeeded.
	//
	// Errors:
	//   - ErrNotFound: no in-flight message with this id.
	Ack(ctx context.Context, id string) error

	// Retry makes an in-flight message eligible for redelivery after
	// the given delay. It never dead-letters and never enforces an
	// attempt limit — retry policy belongs entirely to the caller,
	// which reads msg.Attempt and applies its own per-job policy.
	//
	// Retry is a command, not a decision: the caller already decided
	// this attempt failed and wants another one.
	//
	// Errors:
	//   - ErrNotFound: no in-flight message with this id.
	Retry(ctx context.Context, id string, delay time.Duration) error

	// DeadLetter permanently removes an in-flight message, marking it
	// as permanently failed. The message MUST NOT be redelivered.
	//
	// Drivers with a native dead-letter facility (e.g. an SQS DLQ, a
	// Redis "dead" list, a failed-messages table) SHOULD park the
	// message there for inspection; drivers without one simply
	// discard it. Either way, the framework-level record of the
	// failure (cause, payload, attempt count) is kept by the caller.
	//
	// DeadLetter is a command, not a decision: the caller — not the
	// driver — determined that the message is permanently failed.
	// Drivers MUST NOT dead-letter messages on their own: no internal
	// attempt limits, no TTL-based expiry into a failed state, no
	// automatic discarding of repeated failures.
	//
	// Errors:
	//   - ErrNotFound: no in-flight message with this id.
	DeadLetter(ctx context.Context, id string) error
}

// Message is a unit of work traveling through a queue.
type Message struct {
	// ID is a unique identifier for this message, assigned by Push
	// if not already set.
	ID string

	// QueueName is the queue this message belongs to.
	QueueName string

	// Namespace and JobName identify which registered job this
	// message is for. The queue driver treats them as opaque.
	Namespace string
	JobName   string

	// Payload is the opaque job payload.
	Payload []byte

	// Attempt is the number of times this message has been popped
	// for delivery. The first delivery has Attempt == 1.
	//
	// Ownership: the DRIVER owns this counter — it MUST increment
	// Attempt on every Pop. The CALLER (the app/manager layer) owns
	// the DECISION of what to do with the count (retry policy,
	// max-attempts limit, dead-lettering). Drivers MUST NOT enforce
	// any attempt limit of their own.
	Attempt int

	// VisibilityTimeout is how long this message stays invisible to
	// other workers after being popped. If the worker has not Acked,
	// Retried, or DeadLettered the message within this window, the
	// driver MAY make it visible again (redelivery — see the
	// at-least-once guarantee).
	//
	// Ownership: the CALLER sets this per message or based on the
	// job's configured max runtime. If zero, the driver applies its
	// own default.
	//
	// Drivers that cannot honor a per-message timeout (e.g. SQS
	// per-message visibility is supported, but some backends only
	// have a per-queue setting) MUST document their behavior.
	VisibilityTimeout time.Duration
}

// PopStream is an OPTIONAL interface. A Queue driver that can deliver
// messages as a stream (long-poll, subscription, etc.) implements it
// to avoid polling overhead.
//
// If a driver does not implement PopStream, the framework wraps it
// with PollAsStream, which repeatedly calls Pop and forwards results
// over a channel. Workers therefore go through a single code path
// either way.
type PopStream interface {
	// PopChan returns a channel delivering messages from the named
	// queue until ctx is cancelled, at which point the channel MUST
	// be closed.
	//
	// The driver MUST increment msg.Attempt on each delivered
	// message, as with Pop. Delivery is at-least-once.
	//
	// The driver owns the channel and MUST tolerate the caller
	// abandoning it after ctx cancellation.
	PopChan(ctx context.Context, queueName string) (<-chan *Result, error)
}

// Result pairs a delivered message with a delivery error. Exactly one
// of Msg / Err is non-nil.
type Result struct {
	Msg *Message
	Err error
}
