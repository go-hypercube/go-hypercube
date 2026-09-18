// Package memoryqueue is a process-local, non-persistent queue.Queue
// implementation intended for tests and single-instance apps that don't
// need durability across restarts.
package memoryqueue

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/go-hypercube/go-hypercube/queue"
)

type entry struct {
	msg       *queue.Message
	visibleAt time.Time
}

type memoryQueue struct {
	mu       sync.Mutex
	pending  map[string][]*entry // queueName -> pending entries
	inFlight map[string]*entry   // msg.ID -> entry currently popped but not yet Ack/Fail
}

func NewMapBacked() queue.Queue {
	return &memoryQueue{
		pending:  make(map[string][]*entry),
		inFlight: make(map[string]*entry),
	}
}

func (q *memoryQueue) Push(_ context.Context, m *queue.Message, delay time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending[m.QueueName] = append(q.pending[m.QueueName], &entry{
		msg:       m,
		visibleAt: time.Now().Add(delay),
	})
	return nil
}

// Pop returns immediately with queue.ErrEmpty if nothing is visible —
// it does not block. Callers (e.g. App.Worker) are expected to
// poll-and-backoff on ErrEmpty themselves.
func (q *memoryQueue) Pop(_ context.Context, queueName string) (*queue.Message, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	list := q.pending[queueName]
	for i, e := range list {
		if e.visibleAt.After(now) {
			continue
		}
		q.pending[queueName] = append(append([]*entry{}, list[:i]...), list[i+1:]...)
		e.msg.Attempt++
		q.inFlight[e.msg.ID] = e
		return e.msg, nil
	}
	return nil, queue.ErrEmpty
}

func (q *memoryQueue) Ack(_ context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.inFlight, m.ID)
	return nil
}

func (q *memoryQueue) Fail(_ context.Context, id string, delay time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.inFlight, m.ID)
	q.pending[m.QueueName] = append(q.pending[m.QueueName], &entry{
		msg:       m,
		visibleAt: time.Now().Add(delay),
	})
	return nil
}

func (q *memoryQueue) Len(_ context.Context, queueName string) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return int64(len(q.pending[queueName])), nil
}

func (q *memoryQueue) Queues(ctx context.Context) ([]string, error) {
	return slices.Collect(maps.Keys(q.pending)), nil
}
