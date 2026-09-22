// Package memoryqueue is a process-local, non-persistent queue.Queue
// implementation intended for tests and single-instance apps that don't
// need durability across restarts.
//
// VisibilityTimeout semantics: a popped message is invisible for
// max(msg.VisibilityTimeout, Config.DefaultVisibilityTimeout) — zero
// on both falls back to the package default below. After the timeout
// expires the message becomes poppable again (at-least-once), even
// before the original worker Ack/Retry/DeadLetters it. Because
// expiry is checked lazily on Pop (no background sweeper), an
// expired message may still be reported as InFlight by Stats until
// the next Pop reclaims it — the double-count window documented on
// queue.QueueStats.
package memoryqueue

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/go-hypercube/go-hypercube/queue"
)

// DefaultVisibilityTimeout is used when neither the message nor the
// driver Config declares a visibility timeout.
const DefaultVisibilityTimeout = 30 * time.Second

// Config holds driver-level defaults for the memory queue. The zero
// value is valid: unset fields fall back to package defaults.
type Config struct {
	// DefaultVisibilityTimeout is the fallback visibility window
	// applied when a popped message's own
	// queue.Message.VisibilityTimeout is zero. Zero selects
	// DefaultVisibilityTimeout.
	DefaultVisibilityTimeout time.Duration
}

// NewMapBacked returns a queue with zero-value Config.
func NewMapBacked() queue.Queue {
	return New(Config{})
}

// New returns a process-local queue with the given driver config.
func New(cfg Config) queue.Queue {
	def := cfg.DefaultVisibilityTimeout
	if def == 0 {
		def = DefaultVisibilityTimeout
	}
	return &memoryQueue{
		cfg:      def,
		pending:  make(map[string][]*entry),
		inFlight: make(map[string]*entry),
	}
}

type entry struct {
	msg *queue.Message

	// delayedUntil: earliest time Pop may hand this message out
	// (set at Push/Retry from the delay parameter).
	delayedUntil time.Time

	// invisibilityDeadline: until this time, an in-flight message
	// may not be re-popped. Zero while pending.
	invisibilityDeadline time.Time
}

type memoryQueue struct {
	mu       sync.Mutex
	cfg      time.Duration       // resolved default visibility timeout
	pending  map[string][]*entry // queueName -> pending entries
	inFlight map[string]*entry   // msg.ID -> entry popped but not yet Ack/Retry/DeadLetter
}

var (
	_ queue.Queue         = (*memoryQueue)(nil)
	_ queue.StatsProvider = (*memoryQueue)(nil)
)

func (q *memoryQueue) Push(_ context.Context, m *queue.Message, delay time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending[m.QueueName] = append(q.pending[m.QueueName], &entry{
		msg:          m,
		delayedUntil: time.Now().Add(delay),
	})
	return nil
}

// Pop returns immediately with queue.ErrEmpty if nothing is visible —
// it does not block. Callers (e.g. App.Worker) are expected to
// poll-and-backoff on ErrEmpty themselves.
//
// Pop scans pending entries first, then reclaims in-flight messages
// whose visibility timeout has expired (at-least-once redelivery).
func (q *memoryQueue) Pop(_ context.Context, queueName string) (*queue.Message, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()

	// 1. Pending: oldest eligible entry wins (FIFO).
	list := q.pending[queueName]
	for i, e := range list {
		if e.delayedUntil.After(now) {
			continue // not yet eligible (delayed); entries are ordered, but delays vary — keep scanning
		}
		q.pending[queueName] = append(list[:i:i], list[i+1:]...)
		return q.claim(e, now), nil
	}

	// 2. Expired in-flight: visibility timeout passed, the original
	// worker never finished — make it deliverable again. This is
	// the lazy promotion; until this runs, Stats may count the
	// message as InFlight (see package doc).
	for id, e := range q.inFlight {
		if e.msg.QueueName != queueName {
			continue
		}
		if e.invisibilityDeadline.After(now) {
			continue
		}
		delete(q.inFlight, id)
		return q.claim(e, now), nil
	}

	return nil, queue.ErrEmpty
}

// claim marks e as in-flight: increments Attempt and starts the
// visibility window. Caller must hold q.mu.
func (q *memoryQueue) claim(e *entry, now time.Time) *queue.Message {
	e.msg.Attempt++
	vis := e.msg.VisibilityTimeout
	if vis == 0 {
		vis = q.cfg
	}
	e.invisibilityDeadline = now.Add(vis)
	q.inFlight[e.msg.ID] = e
	return e.msg
}

func (q *memoryQueue) Ack(_ context.Context, msg *queue.Message) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.inFlight[msg.ID] == nil {
		return queue.ErrNotFound
	}
	delete(q.inFlight, msg.ID)
	return nil
}

func (q *memoryQueue) Retry(_ context.Context, msg *queue.Message, delay time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	e := q.inFlight[msg.ID]
	if e == nil {
		return queue.ErrNotFound
	}
	delete(q.inFlight, msg.ID)
	e.invisibilityDeadline = time.Time{} // back to pending state
	e.delayedUntil = time.Now().Add(delay)
	q.pending[e.msg.QueueName] = append(q.pending[e.msg.QueueName], e)
	return nil
}

func (q *memoryQueue) DeadLetter(_ context.Context, msg *queue.Message) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.inFlight[msg.ID] == nil {
		return queue.ErrNotFound
	}
	// This driver discards dead-lettered messages; per the
	// StatsProvider docs, Dead is therefore always reported as 0.
	delete(q.inFlight, msg.ID)
	return nil
}

// Queues returns the names of queues that currently hold at least
// one message, pending OR in flight. Empty queues are omitted —
// absence does not mean the queue "doesn't exist" (see
// StatsProvider docs). A queue whose only messages are in flight
// still appears, so callers don't mistake active work for a
// missing queue.
func (q *memoryQueue) Queues(_ context.Context) ([]string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	names := make(map[string]struct{})
	for name, list := range q.pending {
		if len(list) > 0 {
			names[name] = struct{}{}
		}
	}
	for _, e := range q.inFlight {
		names[e.msg.QueueName] = struct{}{}
	}
	return slices.Sorted(maps.Keys(names)), nil
}

// Stats returns a point-in-time, APPROXIMATE snapshot for queueName.
// Unknown or empty queues yield zero-value counts, not an error.
//
// InFlight counts messages popped and not yet Acked/Retried/
// DeadLettered, including the window after their visibility timeout
// expired but before the next Pop reclaims them — so Pending and
// InFlight may briefly double-count a message during that handoff
// (documented on queue.QueueStats).
func (q *memoryQueue) Stats(_ context.Context, queueName string) (*queue.QueueStats, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	return &queue.QueueStats{
		Name:     queueName,
		Pending:  int64(len(q.pending[queueName])),
		InFlight: q.countInFlightLocked(queueName),
		Dead:     0, // driver discards dead-lettered messages
	}, nil
}

// countInFlightLocked counts in-flight entries belonging to
// queueName. Caller must hold q.mu.
func (q *memoryQueue) countInFlightLocked(queueName string) int64 {
	var n int64
	for _, e := range q.inFlight {
		if e.msg.QueueName == queueName {
			n++
		}
	}
	return n
}

var _ queue.StatsProvider = (*memoryQueue)(nil)
