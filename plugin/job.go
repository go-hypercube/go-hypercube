package plugin

import (
	"context"
	"errors"

	"github.com/go-hypercube/go-hypercube/job"
)

// ErrSchedulerUnavailable indicates that no scheduling callback was supplied to App.
var ErrSchedulerUnavailable = errors.New("plugin job scheduling is unavailable")

// Every schedules a callback for direct execution, without job registration or
// queue delivery. The host owns the scheduler's Start and Shutdown calls.
func (app *App) Every(crontab string, task func(context.Context) error) error {
	if app.every == nil {
		return ErrSchedulerUnavailable
	}
	return app.every(crontab, task)
}

// EveryDispatch schedules enqueueing a job in this plugin's namespace.
// Queue workers execute the job through the normal retry and dead-letter path.
func (app *App) EveryDispatch(crontab, jobName string, payload []byte, config job.DispatchConfig) error {
	if app.everyDispatch == nil {
		return ErrSchedulerUnavailable
	}
	return app.everyDispatch(crontab, jobName, payload, config)
}

// ErrDispatchUnavailable indicates that no dispatcher was supplied to App.
var ErrDispatchUnavailable = errors.New("plugin job dispatch is unavailable")

// Dispatch enqueues a registered job in this plugin's namespace.
// The plugin's own jobs become available after Register returns; dispatch them
// from Boot or later. The caller's context controls the enqueue operation.
func (app *App) Dispatch(ctx context.Context, jobName string, payload []byte, config job.DispatchConfig) error {
	if app.dispatch == nil {
		return ErrDispatchUnavailable
	}
	return app.dispatch(ctx, jobName, payload, config)
}
