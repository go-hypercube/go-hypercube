package job

import (
	"context"
	"errors"

	"github.com/go-co-op/gocron/v2"
	"github.com/go-hypercube/go-hypercube/scheduler"
	"github.com/google/uuid"
)

// ErrSchedulerUnavailable indicates that no scheduling callback was supplied to App.
var ErrSchedulerUnavailable = errors.New("job scheduling is unavailable")

// Schedule schedules a callback for direct execution in the running job's
// namespace. The host owns the scheduler's Start and Shutdown calls.
func (app *App) Schedule(crontab string, task func(context.Context) error, config scheduler.JobConfig) error {
	if app.schedule == nil {
		return ErrSchedulerUnavailable
	}
	return app.schedule(crontab, task, config)
}

// ScheduleDispatch schedules enqueueing a job in the running job's namespace.
// Queue workers execute the job through the normal retry and dead-letter path.
func (app *App) ScheduleDispatch(crontab, jobName string, payload []byte, config DispatchConfig, schedule scheduler.JobConfig) error {
	if app.scheduleDispatch == nil {
		return ErrSchedulerUnavailable
	}
	return app.scheduleDispatch(crontab, jobName, payload, config, schedule)
}

// ScheduledJobs lists only schedules owned by the running job's namespace.
func (app *App) ScheduledJobs() []gocron.Job {
	if app.scheduledJobs == nil {
		return nil
	}
	return app.scheduledJobs()
}

// RemoveScheduledJob removes a schedule in the running job's namespace and
// cancels its task. IDs in other namespaces return gocron.ErrJobNotFound.
func (app *App) RemoveScheduledJob(id uuid.UUID) error {
	if app.removeScheduledJob == nil {
		return ErrSchedulerUnavailable
	}
	return app.removeScheduledJob(id)
}
