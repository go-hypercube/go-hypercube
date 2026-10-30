package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-co-op/gocron/v2"
	"github.com/go-hypercube/go-hypercube/scheduler"
	"github.com/google/uuid"
)

// ErrSchedulerUnavailable indicates that Options.Scheduler was not supplied.
var ErrSchedulerUnavailable = errors.New("scheduler is unavailable: supply Options.Scheduler")

// Schedule schedules direct callback execution. The host starts the supplied
// scheduler after Boot succeeds and shuts it down during teardown.
// The configuration sets its ID, name, tags, and execution options.
// The app supplies the host namespace.
func (app *App) Schedule(crontab string, task func(context.Context) error, config scheduler.JobConfig) error {
	return app.schedule(crontab, hostAppNamespace, task, config)
}

func (app *App) schedule(crontab, namespace string, task func(context.Context) error, config scheduler.JobConfig) error {
	if app.scheduler == nil {
		return ErrSchedulerUnavailable
	}
	config.Namespace = namespace
	_, err := app.scheduler.Schedule(crontab, task, config)
	return err
}

// ScheduleDispatch schedules recurring enqueueing in the framework namespace.
// Queue workers execute the job, including retries and dead-letter handling.
func (app *App) ScheduleDispatch(crontab, jobName string, payload []byte, config DispatchConfig, schedule scheduler.JobConfig) error {
	return app.scheduleDispatch(crontab, hostAppNamespace, jobName, payload, config, schedule)
}

func (app *App) scheduleDispatch(crontab, namespace, jobName string, payload []byte, config DispatchConfig, schedule scheduler.JobConfig) error {
	if app.scheduler == nil {
		return ErrSchedulerUnavailable
	}
	err := app.schedule(crontab, namespace, func(ctx context.Context) error {
		if err := app.dispatch(ctx, namespace, jobName, payload, config); err != nil {
			app.logger.Error("scheduled dispatch failed", "namespace", namespace, "job", jobName, "err", err)
		}
		return nil
	}, schedule)
	if err != nil {
		return fmt.Errorf("schedule job %q in namespace %q: %w", jobName, namespace, err)
	}
	return nil
}

// ScheduledJobs lists all cron schedules, including those owned by plugins.
// Job handles expose ID, Name, Tags, Schedule, NextRun, and RunNow.
func (app *App) ScheduledJobs() []gocron.Job {
	if app.scheduler == nil {
		return nil
	}
	return app.scheduler.Jobs()
}

func (app *App) scheduledJobs(namespace string) []gocron.Job {
	if app.scheduler == nil {
		return nil
	}
	return app.scheduler.JobsInNamespace(namespace)
}

// RemoveScheduledJob removes a cron by ID and cancels its running task context.
func (app *App) RemoveScheduledJob(id uuid.UUID) error {
	if app.scheduler == nil {
		return ErrSchedulerUnavailable
	}
	return app.scheduler.RemoveJob(id)
}

func (app *App) removeScheduledJob(namespace string, id uuid.UUID) error {
	if app.scheduler == nil {
		return ErrSchedulerUnavailable
	}
	return app.scheduler.RemoveJobInNamespace(namespace, id)
}
