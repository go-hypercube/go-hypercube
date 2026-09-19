package job

import "time"

// VisibilityTimeout is an OPTIONAL interface a Job may implement to
// declare how long a single attempt of that job may run before the
// message becomes eligible for redelivery (see the at-least-once
// guarantee in the queue package).
//
// A visibility timeout can be set at two levels, resolved at
// dispatch in this order:
//
//  1. Per-dispatch override DispatchConfig.VisibilityTimeout.
//     Wins non-zero. Use for one-off calls where the caller
//     knows this particular message needs a different window.
//  2. Per-job declaration: implement this interface on the Job.
//     Use for the job's normal, every-time bound.
//
// If neither is set (zero everywhere), the driver applies its own
// default timeout.
//
// Jobs with attempt runtimes that can exceed the driver default
// (bulk exports, slow API calls) SHOULD implement this interface so
// the bound travels with the job registration rather than being
// repeated at every dispatch site.
//
// The declared value must cover the full attempt: worst-case handler
// runtime plus ack latency. If a job outlives its
// timeout it is redelivered WHILE STILL RUNNING — so the app layer
// should also bound the handler itself (e.g. a context set
// the same value) so the handler stops before redelivery starts.
//
// This is a declaration by the JOB, resolved at dispatch and stamped
// onto queue.Message.VisibilityTimeout. The queue driver never
// inspects job — it only sees the message field.
type VisibilityTimeout interface {
	VisibilityTimeout() time.Duration
}

// VisibilityTimeoutFor returns the visibility timeout declared by j,
// or zero if j does not VisibilityTimeout.
func VisibilityTimeoutFor(j Job) time.Duration {
	if vt, ok := j.(VisibilityTimeout); ok {
		return vt.VisibilityTimeout()
	}
	return 0
}
