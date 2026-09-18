package memoryqueue

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/go-hypercube/go-hypercube/queue"
)

// channelQueue is a process-local queue.Queue backed by one Go channel
// per queue name. Unlike MemoryQueue (which polls a slice and never
// blocks), Pop here is a real blocking receive: it returns as soon as a
// message is available, or as soon as ctx is cancelled/expires — it
// never returns queue.ErrEmpty. Use this when a test or caller cares
// about wake-up latency; use MemoryQueue when you just want the
// simplest possible in-process queue.
//
// Ack is a no-op: a channel receive is destructive, so by the time Pop
// has returned, the message has already left the channel — there is no
// separate in-flight set to clear (unlike MemoryQueue). Fail re-queues
// by sending the message back onto the channel, keeping the Attempt
// that Pop already incremented.
type channelQueue struct {
	mu      sync.Mutex
	queues  map[string]chan *queue.Message
	bufSize int
}

// New returns a ChannelQueue whose per-queue channels are buffered to
// bufSize. bufSize == 0 makes Push block until a worker is ready to
// receive (useful for testing backpressure); a small positive size
// (e.g. 64) is more typical.
func NewChannelBacked(bufSize int) queue.Queue {
	return &channelQueue{
		queues:  make(map[string]chan *queue.Message),
		bufSize: bufSize,
	}
}

func (q *channelQueue) channelFor(queueName string) chan *queue.Message {
	q.mu.Lock()
	defer q.mu.Unlock()
	ch, ok := q.queues[queueName]
	if !ok {
		ch = make(chan *queue.Message, q.bufSize)
		q.queues[queueName] = ch
	}
	return ch
}

func (q *channelQueue) Push(ctx context.Context, m *queue.Message, delay time.Duration) error {
	if delay <= 0 {
		return q.send(ctx, m)
	}
	// Best-effort delayed delivery: the timer goroutine outlives this
	// call, so a ctx cancelled after Push returns does not cancel the
	// eventual send — only immediate (delay <= 0) sends respect ctx.
	time.AfterFunc(delay, func() {
		_ = q.send(context.Background(), m)
	})
	return nil
}

func (q *channelQueue) send(ctx context.Context, m *queue.Message) error {
	select {
	case q.channelFor(m.QueueName) <- m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *channelQueue) Pop(ctx context.Context, queueName string) (*queue.Message, error) {
	select {
	case m := <-q.channelFor(queueName):
		m.Attempt++
		return m, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (q *channelQueue) Ack(_ context.Context, _ string) error {
	return nil
}

func (q *channelQueue) Fail(ctx context.Context, id string, delay time.Duration) error {
	return q.Push(ctx, m, delay) // Attempt already incremented by Pop
}

// Len reports only messages currently resident in the channel — it does
// not count messages still waiting on a delayed Push's time.AfterFunc
// timer, unlike MemoryQueue.Len's "pending (visible + delayed)"
// contract. Treat this as an approximation for blocking-drain tests,
// not a precise backlog count.
func (q *channelQueue) Len(_ context.Context, queueName string) (int64, error) {
	return int64(len(q.channelFor(queueName))), nil
}

func (q *channelQueue) Queues(ctx context.Context) ([]string, error) {
	return slices.Collect(maps.Keys(q.queues)), nil
}

// PopChan implements queue.PopStream. The returned channel is safe for
// multiple goroutines to range over concurrently — Go's channel
// semantics already distribute one message to exactly one receiver, so
// no extra fan-out logic is needed here.
func (q *channelQueue) PopChan(ctx context.Context, queueName string) (<-chan *queue.Result, error) {
	src := q.channelFor(queueName)
	out := make(chan *queue.Result)

	// NOTE: When multiple goroutines are listening to the same channel (src/queueName)
	// (calling PopChan more than once, for example, if the user needs multiple processes per queueName)
	// and an event is sent, the event will go to only one of the listening goroutines.

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

var _ queue.PopStream = (*channelQueue)(nil)
