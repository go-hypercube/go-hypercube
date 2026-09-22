// Package memoryqueue provides process-local, non-persistent
// queue.Queue implementations for tests and single-instance apps.
//
// This file implements the channel-backed driver: one Go channel per
// queue name. Its defining property is BLOCKING POP — Pop waits until
// a message is available or ctx is cancelled, and never returns
// queue.ErrEmpty. Use this driver when low-latency wake-up matters;
// use the map-backed driver in the same package when you need
// broker-faithful semantics (visibility timeout, redelivery,
// meaningful InFlight stats).
//
// CONTRACT COMPLIANCE vs queue.Queue docs:
//
// Push(ctx, m, delay):
//   - delay <= 0: enqueues immediately.
//   - delay > 0: a timer holds the message and enqueues it after the
//     delay. While delayed, the message is invisible to Pop but still
//     counted by Stats (Pending includes delayed messages — fixed vs
//     naive implementations) and Queues reports the queue name.
//   - Delayed sends survive ctx cancellation of the original Push:
//     once Push returns nil, delivery is the driver's responsibility
//     (the timer uses a background context). Push returns ctx.Err()
//     only if the immediate send path can't complete in time.
//
// Pop(ctx, queueName):
//   - Blocking: returns as soon as a message is visible, or
//     ctx.Err() on cancellation. Never queue.ErrEmpty.
//   - Increments msg.Attempt on every delivery.
//
// Ack: no-op. A channel receive is destructive; the message has
// already left the queue when Pop returned, so there is nothing to
// acknowledge. Returns nil always. DEVIATION: at-least-once is NOT
// provided — if the worker crashes after Pop and before finishing
// the work, the message is gone. This is inherent to channel
// semantics, not a bug.
//
// Retry(ctx, m, delay):
//   - Re-enqueues the message after delay (immediately if delay <= 0).
//   - Attempt bookkeeping: Pop incremented Attempt on delivery, and
//     the next Pop will increment it again. Retry therefore
//     DECREMENTS Attempt before re-enqueueing, so the count observed
//     after redelivery is correct (delivery N+1, not N+2). This
//     makes Retry/Pop loops Attempt-neutral per cycle.
//   - Deadlock warning: Retry performs a channel send. If the channel
//     is full (or unbuffered) and no receiver is ready, Retry blocks.
//     Calling Retry from the single worker goroutine that would
//     otherwise receive is a self-deadlock in that case — always pass
//     a cancellable ctx and treat ctx.Err() as "retry dropped". The
//     message is NOT lost-and-redelivered on failure: if the send
//     doesn't happen, the message is dropped entirely (channel
//     semantics — there is no copy to fall back on).
//
// DeadLetter: no-op discard. No dead-letter store; per QueueStats
// docs, Dead is always 0.
//
// VisibilityTimeout: ignored entirely. There is no in-flight
// registry and no invisibility window — both msg.VisibilityTimeout
// and Config.DefaultVisibilityTimeout are accepted and discarded.
//
// StatsProvider (Stats / Queues) and PopStream (PopChan) are
// implemented; see their method docs for exact guarantees.
package memoryqueue

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/go-hypercube/go-hypercube/queue"
)

// ChannelConfig configures the channel-backed driver. The zero value
// is valid (DefaultChannelBuffer applies).
type ChannelConfig struct {
	// BufferSize is the channel buffer size created per queue name.
	// Must be >= 0; 0 selects DefaultChannelBuffer. A buffer of 0
	// would make every Push/Retry a rendezvous with
	// receiver — allowed, but discouraged; see Retry docs.
	BufferSize int
}

// DefaultChannelBuffer is used when ChannelConfig.BufferSize is 0.
const DefaultChannelBuffer = 64

// NewChannelBacked returns a channel-backed queue with zero-value
// ChannelConfig.
func NewChannelBacked() queue.Queue { return NewChannel(ChannelConfig{}) }

// NewChannel returns a channel-backed queue with the given config.
func NewChannel(cfg ChannelConfig) queue.Queue {
	size := cfg.BufferSize
	if size == 0 {
		size = DefaultChannelBuffer
	}
	if size < 0 {
		panic("memoryqueue: ChannelConfig.BufferSize must be >= 0")
	}
	return &channelQueue{
		bufSize: size,
		queues:  make(map[string]*queueChannel),
	}
}

// queueChannel bundles a queue name's channel with its delayed-message
// bookkeeping. Channels are created lazily on first use and never
// closed (closing would race concurrent senders; garbage collection
// handles the driver's lifetime).
type queueChannel struct {
	ch chan *queue.Message

	// delayed is the number of messages currently held by delivery
	// timers for this queue. Maintained under channelQueue.mu.
	// Included in Stats.Pending so delayed messages are visible.
	delayed int
}

type channelQueue struct {
	mu      sync.Mutex
	bufSize int
	queues  map[string]*queueChannel
}

var (
	_ queue.Queue         = (*channelQueue)(nil)
	_ queue.StatsProvider = (*channelQueue)(nil)
)

// qFor returns (creating if needed) the queueChannel for queueName.
// Caller must hold q.mu.
func (q *channelQueue) qFor(queueName string) *queueChannel {
	qc, ok := q.queues[queueName]
	if !ok {
		qc = &queueChannel{ch: make(chan *queue.Message, q.bufSize)}
		q.queues[queueName] = qc
	}
	return qc
}

// Push enqueues m on its queue's channel, honoring delay.
//
// Immediate path (delay <= 0): blocking send with ctx. Returns
// ctx.Err() if ctx is cancelled before a send slot is available.
//
// Delayed path (delay > 0): registers the message as delayed, then a
// timer performs the send after the delay on a background context —
// Push returns nil immediately and delivery is guaranteed regardless
// of the caller's ctx (once accepted, delivery is the driver's job).
// Delayed sends use a non-blocking send with a drop-on-full policy:
// if the channel is full when the timer fires, the message is DROPPED
// rather than blocking a goroutine forever. This is documented
// best-effort behavior; callers needing guaranteed delayed delivery
// should ensure buffer headroom or use the map-backed driver.
func (q *channelQueue) Push(ctx context.Context, m *queue.Message, delay time.Duration) error {
	if delay <= 0 {
		return q.send(ctx, m)
	}

	qc := q.qFor(m.QueueName)
	q.mu.Lock()
	qc.delayed++
	q.mu.Unlock()

	time.AfterFunc(delay, func() {
		q.mu.Lock()
		qc.delayed--
		q.mu.Unlock()

		// Best-effort: drop if full. Never block a timer goroutine.
		select {
		case qc.ch <- m:
		default:
			// Channel full at fire time; message dropped. See Push doc.
		}
	})
	return nil
}

// send performs a ctx-aware blocking send.
func (q *channelQueue) send(ctx context.Context, m *queue.Message) error {
	qc := q.qFor(m.QueueName)
	select {
	case qc.ch <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Pop blocks until a message is available or ctx is done. It never
// returns queue.ErrEmpty — callers that need poll semantics should
// use the map-backed driver, or wrap this Pop with their own
// select/timeout. Increments Attempt on delivery.
func (q *channelQueue) Pop(ctx context.Context, queueName string) (*queue.Message, error) {
	qc := q.qFor(queueName)
	select {
	case m := <-qc.ch:
		m.Attempt++
		return m, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Ack is a no-op: channel receives are destructive, so there is
// nothing to acknowledge and nothing to fail. Always returns nil.
// See the package doc for the at-least-once deviation this implies.
func (q *channelQueue) Ack(_ context.Context, _ *queue.Message) error {
	return nil
}

// Retry re-enqueues m for redelivery after delay (immediately if
// delay <= 0), preserving correct Attempt numbering: it decrements
// Attempt first, because the next Pop will increment it again.
//
// Failure semantics: Retry sends onto the channel and can block if
// the channel is full with no ready receiver. On ctx cancellation
// before the send completes, the retry is DROPPED and ctx.Err() is
// returned — the message is not silently re-queued somewhere else,
// it is gone (see package doc). Prefer a buffered channel sized for
// your retry bursts.
//
// Delayed retries (delay > 0) follow Push's delayed path, including
// its drop-on-full policy.
func (q *channelQueue) Retry(ctx context.Context, m *queue.Message, delay time.Duration) error {
	if m.Attempt > 0 {
		m.Attempt-- // next Pop re-increments; keeps Attempt accurate
	}
	return q.Push(ctx, m, delay)
}

// DeadLetter discards the message permanently. No dead-letter store;
// per the QueueStats docs, Dead is always 0 for this driver.
func (q *channelQueue) DeadLetter(_ context.Context, _ *queue.Message) error {
	return nil
}

// Stats returns a point-in-time APPROXIMATE snapshot.
//
// Pending = messages resident in the channel + messages currently
// held by delivery timers (delayed). Unlike naive channel drivers,
// delayed messages are NOT invisible to Stats — the delayed counter
// closes that gap.
//
// InFlight is always 0 and must be interpreted as UNKNOWN, not
// zero-work: a popped message has left the channel and the driver
// tracks nothing about it afterwards. This is the documented
// deviation inherent to destructive receives.
//
// Dead is always 0 (dead letters are discarded).
//
// Unknown queue names yield a zero-value snapshot, not an error.
func (q *channelQueue) Stats(_ context.Context, queueName string) (*queue.QueueStats, error) {
	q.mu.Lock()
	qc, ok := q.queues[queueName]
	delayed := int64(0)
	if ok {
		delayed = int64(qc.delayed)
	}
	q.mu.Unlock()

	pending := int64(0)
	if ok {
		pending = int64(len(qc.ch)) + delayed
	}

	return &queue.QueueStats{
		Name:     queueName,
		Pending:  pending,
		InFlight: 0, // unknown by design — see method doc
		Dead:     0, // discarded per QueueStats contract
	}, nil
}

// Queues returns the sorted names of all queues this driver knows
// about — i.e., every queue name that has ever been Pushed, Popped,
// or Stats'd. Channels are created lazily and never destroyed, so a
// drained queue still appears here; per the StatsProvider docs,
// presence does NOT mean messages exist, and emptiness should be
// judged via Stats. (This is a deliberate, documented choice: for a
// channel driver, tracking "queues with live messages only" would
// require a second registry that races with channel state for
// marginal benefit.)
func (q *channelQueue) Queues(_ context.Context) ([]string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Sorted(maps.Keys(q.queues)), nil
}

// PopChan implements queue.PopStream. It returns a channel that
// yields messages as they arrive until ctx is cancelled, then closes.
//
// Competing consumers: if multiple goroutines consume from the
// returned channel — or mix PopChan and Pop on the same queue name —
// each message is delivered to exactly ONE receiver (Go channel
// semantics). That is correct competing-consumer behavior, not
// fan-out; use multiple drivers or fan-out at the application layer
// if broadcast is needed.
//
// Attempt is incremented once per message as it is placed on the
// returned channel (delivery = placement on the out channel).
//
// Backpressure: the pump goroutine forwards one message at a time,
// so if, the pump stalls — which in turn stops
// draining the underlying channel and applies natural backpressure
// to Push senders. No unbounded buffering is introduced between the
// driver and
// Ordering guarantee per consumer: one message is in flight from
// src to out at a time, so relative arrival order seen by a single
// consumer matches Push order (modulo delayed pushes, which arrive
// in timer-fire order).
func (q *channelQueue) PopChan(ctx context.Context, queueName string) (<-chan *queue.Result, error) {
	src := q.qFor(queueName).ch
	out := make(chan *queue.Result)

	go func() {
		defer close(out)
		for {
			select {
			case m := <-src:
				m.Attempt++
				select {
				case out <- &queue.Result{Msg: m}:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

var (
	_ queue.StatsProvider = (*channelQueue)(nil)
	_ queue.PopStream     = (*channelQueue)(nil)
)
