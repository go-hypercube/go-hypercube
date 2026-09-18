// Package queue defines the portable job-transport contract used by the
// framework and implemented by driver packages. It has no dependency on
// the rest of the framework
package queue

import (
	"context"
	"time"
)

// Message is a single unit of work moving through a Queue. Namespace
// distinguishes plugin-owned jobs from host-app-owned ones.
type Message struct {
	ID        string
	QueueName string
	Namespace string
	JobName   string
	Payload   []byte
	Attempt   int // 1-indexed; incremented by the driver on each Pop/redelivery
}

// Queue is the minimal, backend-agnostic contract a job/message queue
// must support.
//
// Implementations must be safe for concurrent use: Pop, Ack, and Fail
// may be called concurrently by multiple goroutines, including against
// the same queue name or the same message id, without data races or
// corrupting internal state.
//
// State transitions for a single message, and the error each operation
// returns per state, are as follows:
//
//	State           | Ack              | Fail (attempts left) 	   | Fail (last attempt)
//	----------------|------------------|---------------------------|---------------------
//	popped/visible  | -> Acked, ok     | -> pending, attempt++, ok | -> dead-lettered, ok
//	delayed         | ErrNotFound      | ErrNotFound               | ErrNotFound
//	acked           | ErrAlreadyAcked  | ErrNotFound               | ErrNotFound
//	dead-lettered   | ErrNotFound      | ErrAlreadyFailed          | ErrAlreadyFailed
//	never existed   | ErrNotFound      | ErrNotFound               | ErrNotFound
type Queue interface {
	// Push enqueues m for delivery on m.QueueName. If delay > 0, the
	// message only becomes visible to Pop after delay elapses.
	Push(ctx context.Context, m *Message, delay time.Duration) error

	// Pop returns the next available message on queueName, or ErrEmpty
	// if none is currently available. The message is invisible to other
	// Pop calls until Ack, Fail, or a driver-defined visibility timeout
	// elapses. Implementations must increment the returned message's
	// Attempt on every delivery.
	//
	// Pop must be safe to call concurrently: two goroutines calling Pop
	// on the same queueName at the same time must never both receive the
	// same message.
	Pop(ctx context.Context, queueName string) (*Message, error)

	// Ack acknowledges successful processing of the message identified
	// by id (as previously returned by Pop), removing it from the
	// queue permanently. Acked is a terminal state.
	//
	// Returns ErrNotFound if id is not a currently-actionable in-flight
	// message (never existed, delayed, or already dead-lettered via
	// Fail). Returns ErrAlreadyAcked if id has already been Acked.
	Ack(ctx context.Context, id string) error

	// Fail marks the message identified by id as failed. If the
	// message's attempt count is below the driver's configured max
	// attempts, it is made visible again for redelivery (attempt is
	// incremented on the next Pop). If attempts are exhausted, the
	// message is moved to a terminal dead-lettered state instead.
	//
	// Returns ErrNotFound if id is not a currently-actionable in-flight
	// message (never existed, delayed, or already terminal via Ack).
	// Returns ErrAlreadyFailed if id is already dead-lettered; a Fail
	// call that instead triggers a normal retry is not an error.
	Fail(ctx context.Context, id string, delay time.Duration) error

	// Len reports the number of pending (visible + delayed) messages on
	// queueName.
	Len(ctx context.Context, queueName string) (int64, error)

	// Queues reports the currently available queues.
	Queues(ctx context.Context) ([]string, error)
}

// Result is what a PopStream delivers on its channel: either a message
// ready to process, or an error encountered while trying to produce
// one. Exactly one of Msg/Err is non-nil — never both, never neither.
type Result struct {
	Msg *Message
	Err error
}

// PopStream is implemented by drivers that can deliver messages as a
// channel instead of discrete Pop calls. See Queue's doc for why this
// is optional rather than part of Queue itself.
type PopStream interface {
	// PopChan returns a channel of incoming results for queueName.
	// Multiple goroutines may range over the same returned channel
	// concurrently — the driver is responsible for distributing
	// messages across them, not the caller.
	//
	// A Result with Err set reports a delivery problem (e.g. a
	// persistent Pop failure on the underlying queue) without ending
	// the stream — the driver keeps trying and may still deliver
	// messages afterward. Callers should log/handle Err results but
	// keep ranging over the channel. The channel is closed only once
	// ctx is cancelled/expires; that is the sole "stream has ended"
	// signal — a closed channel is never itself an error to check for.
	//
	// The returned error is only for setup failure (e.g. a driver that
	// needs to open a subscription before it can stream). A driver with
	// no setup step should simply return nil here.
	PopChan(ctx context.Context, queueName string) (<-chan *Result, error)
}
