// Package scheduler runs scheduled callbacks in the current process, independently
// of the framework app, queue drivers, and job registry.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-co-op/gocron/v2"
)

// Scheduler uses gocron's scheduling and lifecycle controls.
// Its owner must call Shutdown, even if Start is never called.
type Scheduler struct {
	gocron.Scheduler
}

// New creates a stopped scheduler using the supplied gocron options.
func New(options ...gocron.SchedulerOption) (*Scheduler, error) {
	s, err := gocron.NewScheduler(options...)
	if err != nil {
		return nil, fmt.Errorf("create scheduler: %w", err)
	}
	return &Scheduler{Scheduler: s}, nil
}

// ErrNilTask indicates that a scheduled callback is nil.
var ErrNilTask = errors.New("scheduled task must not be nil")

// Every schedules task using a five-field cron expression. Tasks run directly in
// this process; errors are logged and do not trigger queue retries. The task's
// context is canceled on Shutdown, and tasks must observe it to stop promptly.
func (sc *Scheduler) Every(crontab string, task func(context.Context) error) error {
	if task == nil {
		return ErrNilTask
	}
	_, err := sc.NewJob(gocron.CronJob(crontab, false), gocron.NewTask(func(ctx context.Context) {
		if err := task(ctx); err != nil {
			slog.Error("scheduled task failed", "cron", crontab, "err", err)
		}
	}))
	if err != nil {
		return fmt.Errorf("schedule task with cron %q: %w", crontab, err)
	}
	return nil
}
