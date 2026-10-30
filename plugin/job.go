package plugin

import (
	"context"
	"errors"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"

	"github.com/go-hypercube/go-hypercube/job"
	"github.com/go-hypercube/go-hypercube/scheduler"
)

// ErrSchedulerUnavailable indicates that no scheduling callback was supplied to App.
var ErrSchedulerUnavailable = errors.New("plugin job scheduling is unavailable")

// Schedule schedules a callback for direct execution, without job registration or
// queue delivery. The host owns the scheduler's Start and Shutdown calls.
func (app *App) Schedule(crontab string, task func(context.Context) error, config scheduler.JobConfig) error {
	if app.schedule == nil {
		return ErrSchedulerUnavailable
	}
	return app.schedule(crontab, task, config)
}

// ScheduleDispatch schedules enqueueing a job in this plugin's namespace.
// Queue workers execute the job through the normal retry and dead-letter path.
func (app *App) ScheduleDispatch(crontab, jobName string, payload []byte, config job.DispatchConfig, schedule scheduler.JobConfig) error {
	if app.scheduleDispatch == nil {
		return ErrSchedulerUnavailable
	}
	return app.scheduleDispatch(crontab, jobName, payload, config, schedule)
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

// ScheduledJobs lists only the cron schedules owned by this plugin.
func (app *App) ScheduledJobs() []gocron.Job {
	if app.scheduledJobs == nil {
		return nil
	}
	return app.scheduledJobs()
}

// RemoveScheduledJob removes a cron owned by this plugin and cancels its task.
// IDs belonging to another namespace return gocron.ErrJobNotFound.
func (app *App) RemoveScheduledJob(id uuid.UUID) error {
	if app.removeScheduledJob == nil {
		return ErrSchedulerUnavailable
	}
	return app.removeScheduledJob(id)
}
