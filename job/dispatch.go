package job

import "time"

// DispatchConfig configures a single job dispatch.
type DispatchConfig struct {
	// Delay is the initial delay before the message becomes visible.
	Delay time.Duration

	// VisibilityTimeout overrides the visibility timeout for this single message.
	// Non-zero wins over the job's declaration; zero falls back to the job
	// declaration and finally to the driver default.
	VisibilityTimeout time.Duration
}
