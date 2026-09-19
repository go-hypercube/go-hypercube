package app

import "github.com/go-co-op/gocron/v2"

type Scheduler struct {
	s   gocron.Scheduler
	app *App
}

func (app *App) NewScheduler() (*Scheduler, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}
	return &Scheduler{s: s, app: app}, nil
}

// Every registers a recurring dispatch — gocron only decides when;
// the actual job runs through the normal queue/worker/retry path, not
// inside the scheduler's own goroutine.
func (sc *Scheduler) Every(crontab, jobName string, payload []byte, config DispatchConfig) error {
	return sc.every(crontab, hostAppNamespace, jobName, payload, config)
}

func (sc *Scheduler) every(crontab, namespace, jobName string, payload []byte, config DispatchConfig) error {
	_, err := sc.s.NewJob(
		gocron.CronJob(crontab, false),
		gocron.NewTask(func() {
			if err := sc.app.dispatch(namespace, jobName, payload, config); err != nil {
				sc.app.logger.Error("scheduled dispatch failed", "namespace", namespace, "job", jobName, "err", err)
			}
		}),
	)
	return err
}

func (sc *Scheduler) Start()          { sc.s.Start() }
func (sc *Scheduler) Shutdown() error { return sc.s.Shutdown() }
