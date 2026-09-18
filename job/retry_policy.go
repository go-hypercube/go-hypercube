package job

import "time"

// RetryPolicy is optional. A job that doesn't implement it gets
// DefaultRetryPolicy.
type RetryPolicy interface {
	MaxAttempts() int
	Backoff(attempt int) time.Duration
}

// RetryPolicyFor returns j's own RetryPolicy if it implements one,
// otherwise DefaultRetryPolicy.
func RetryPolicyFor(j Job) RetryPolicy {
	if rp, ok := j.(RetryPolicy); ok {
		return rp
	}
	return defaultRetryPolicy
}

var defaultRetryPolicy = DefaultRetryPolicy{
	cap:         5 * time.Minute,
	maxAttempts: 5,
}

// DefaultRetryPolicy allows 5 attempts with a doubling backoff capped
// at 5 minutes: 1s, 2s, 4s, 8s, 16s...
type DefaultRetryPolicy struct {
	cap         time.Duration
	maxAttempts int
}

func (d DefaultRetryPolicy) MaxAttempts() int { return d.maxAttempts }

func (d DefaultRetryPolicy) Backoff(attempt int) time.Duration {
	duration := time.Duration(1<<uint(attempt)) * time.Second
	if duration > d.cap || duration <= 0 { // guard against overflow on large attempt
		return d.cap
	}
	return duration
}
