package job

import (
	"context"
	"errors"
	"time"
)

// ErrDispatchUnavailable indicates that no dispatcher was supplied to App.
var ErrDispatchUnavailable = errors.New("job dispatch is unavailable")

// Dispatch enqueues a registered job in the running job's namespace.
// The caller's context controls the enqueue operation.
func (app *App) Dispatch(ctx context.Context, jobName string, payload []byte, config DispatchConfig) error {
	if app.dispatch == nil {
		return ErrDispatchUnavailable
	}
	return app.dispatch(ctx, jobName, payload, config)
}

// DispatchConfig configures a single job dispatch.
type DispatchConfig struct {
	// Delay is the initial delay before the message becomes visible.
	Delay time.Duration

	// VisibilityTimeout overrides the visibility timeout for this single message.
	// Non-zero wins over the job's declaration; zero falls back to the job
	// declaration and finally to the driver default.
	VisibilityTimeout time.Duration
}
