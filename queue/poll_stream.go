package queue

import (
	"context"
	"errors"
	"time"
)

// pollAsStream adapts a plain Queue into a PopStream by running a
// single background goroutine per PopChan call that repeatedly calls
// Pop and forwards results onto a channel, backing off pollInterval
// between empty polls.
type pollAsStream struct {
	q            Queue
	pollInterval time.Duration
}

// PollAsStream wraps q so it satisfies PopStream, by polling q.Pop at
// pollInterval whenever the queue is empty. Use this for any Queue
// driver that only implements the base interface (a plain SQL-table
// poller, an SQS client without long-poll wired up, etc).
//
// If q already implements PopStream natively (like MemoryQueue), prefer
// using it directly — wrapping it here adds an unnecessary extra
// goroutine and layer of indirection. PollAsStream does not check for
// or short-circuit that case itself; callers that want "use native
// PopStream if present, else wrap" behavior (see App.Worker) do that
// check themselves before calling this.
func PollAsStream(q Queue, pollInterval time.Duration) PopStream {
	return &pollAsStream{q: q, pollInterval: pollInterval}
}

func (p *pollAsStream) PopChan(ctx context.Context, queueName string) (<-chan *Result, error) {
	out := make(chan *Result)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msg, err := p.q.Pop(ctx, queueName)
			switch {
			case errors.Is(err, ErrEmpty):
				// Expected, not an error worth reporting — just back off.
				if !sleepOrDone(ctx, p.pollInterval) {
					return
				}
				continue
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return // ctx died mid-Pop on a driver that respects it — ordinary shutdown
			case err != nil:
				if !sendOrDone(ctx, out, &Result{Err: err}) {
					return
				}
				if !sleepOrDone(ctx, p.pollInterval) {
					return
				}
				continue
			}

			if !sendOrDone(ctx, out, &Result{Msg: msg}) {
				return
			}
		}
	}()

	return out, nil
}

// sendOrDone sends r on out, or gives up and reports false if ctx is
// cancelled first.
func sendOrDone(ctx context.Context, out chan<- *Result, r *Result) bool {
	select {
	case out <- r:
		return true
	case <-ctx.Done():
		return false
	}
}

// sleepOrDone waits for d or ctx cancellation, whichever comes first,
// reporting whether it completed the full sleep (false means ctx was
// cancelled and the caller should stop).
func sleepOrDone(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
