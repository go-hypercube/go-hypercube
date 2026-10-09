package app

import (
	"context"
	"errors"
	"fmt"
)

// ErrSchedulerUnavailable indicates that Options.Scheduler was not supplied.
var ErrSchedulerUnavailable = errors.New("scheduler is unavailable: supply Options.Scheduler")

// Every schedules direct callback execution. The host starts the supplied
// scheduler after Boot succeeds and shuts it down during teardown.
func (app *App) Every(crontab string, task func(context.Context) error) error {
	if app.scheduler == nil {
		return ErrSchedulerUnavailable
	}
	return app.scheduler.Every(crontab, task)
}

// EveryDispatch schedules recurring enqueueing in the framework namespace.
// Queue workers execute the job, including retries and dead-letter handling.
func (app *App) EveryDispatch(crontab, jobName string, payload []byte, config DispatchConfig) error {
	return app.everyDispatch(crontab, hostAppNamespace, jobName, payload, config)
}

func (app *App) everyDispatch(crontab, namespace, jobName string, payload []byte, config DispatchConfig) error {
	if app.scheduler == nil {
		return ErrSchedulerUnavailable
	}
	err := app.scheduler.Every(crontab, func(ctx context.Context) error {
		if err := app.dispatch(ctx, namespace, jobName, payload, config); err != nil {
			app.logger.Error("scheduled dispatch failed", "namespace", namespace, "job", jobName, "err", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("schedule job %q in namespace %q: %w", jobName, namespace, err)
	}
	return nil
}
