# go-hypercube

This is an evolution & learned lessons from the legacy [go-light-framework](https://github.com/Nidal-Bakir/go-light-framework)

A batteries-included Go backend framework
---

Jobs are registered by `(namespace, name)`. Jobs registered with
`app.RegisterJob` belong to the host namespace, `owner`; jobs returned from a
plugin's `Register` belong to that plugin's name. The host and multiple plugins
can therefore register the same job name without colliding. Plugin names must
be non-empty, unique, and different from the reserved name `owner`. Jobs must
be non-nil and have a non-empty name. Invalid job batches are rejected before
any jobs in that batch are added or replaced.

The following example uses an app constructed with its database, cache,
configuration, logger, and queue dependencies:

```go
type EmailJob struct{}

func (EmailJob) Name() string { return "send-email" }
func (EmailJob) QueueName() string { return "mail" }
func (EmailJob) Handle(app *job.App, payload []byte) error {
    app.Logger.Info("sending email", "payload", string(payload))
    return nil
}

type MailPlugin struct{}

func (MailPlugin) Name() string { return "mail-plugin" }
func (MailPlugin) Version() string { return "v1.0.0" }
func (MailPlugin) Dependencies() []plugin.DependencyDesc { return nil }
func (MailPlugin) Register(*plugin.App) (*plugin.Registration, error) {
    return &plugin.Registration{Jobs: []job.Job{EmailJob{}}}, nil
}
func (MailPlugin) Boot(app *plugin.App) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    return app.Dispatch(ctx, "send-email", []byte("plugin email"), job.DispatchConfig{})
}

func RunMailWorker(ctx context.Context, host *app.App) error {
    if err := host.RegisterJob(EmailJob{}); err != nil {
        return err
    }
    if err := host.UsePlugin(MailPlugin{}); err != nil {
        return err
    }
    if err := host.Setup(); err != nil {
        return err
    }
    if err := host.Boot(); err != nil {
        return err
    }
    if err := host.Dispatch(ctx, "send-email", []byte("host email"), job.DispatchConfig{}); err != nil {
        return err
    }
    return host.Worker(ctx, app.WorkerOptions{QueueName: "mail", Concurrency: 2})
}
```

The example uses the standard packages `context` and `time`, and this module's
`app`, `job`, and `plugin` packages. Plugin jobs become available after their
`Register` method returns; dispatch them from `Boot` or later. Complete
registration before starting workers or the scheduler, and keep the registry
unchanged while they run. Re-registering a name in the same namespace replaces
its handler.

Namespaces select handlers; queue names select which workers consume messages.
The worker above processes both `owner/send-email` and
`mail-plugin/send-email` from the shared `mail` queue. Jobs without `QueueName()`
use the `default` queue. Each worker process must register all handlers for the
queues it consumes: unknown `(namespace, name)` deliveries are dead-lettered.
Handlers returning errors use their job's retry policy and are dead-lettered
after exhausting attempts. Delivery is at least once; handlers should tolerate
duplicate execution. Canceling the worker context stops consumption and waits
for in-flight handlers to finish.

Inside a handler, use `app.Dispatch(ctx, name, payload, job.DispatchConfig{})`
on the supplied `*job.App` to enqueue follow-up work in that handler's namespace.
This uses the registered target job's queue and visibility settings, with
per-dispatch overrides. The context controls enqueueing. Use a timeout for
enqueue operations inside handlers. Job handlers enqueue follow-up work through
`Dispatch`; the framework worker owns acknowledgment, retries, and dead-lettering.

Job handlers also expose `Schedule`, `ScheduleDispatch`, `ScheduledJobs`, and
`RemoveScheduledJob` through injected callbacks, just like plugins. Creation,
listing, and removal are scoped to the running job's namespace, including host
jobs in `owner`. The host supplies and manages the scheduler. Scheduling does
not deduplicate repeated handler calls automatically; use explicit schedule IDs
when callers need duplicate detection (`scheduler.ErrDuplicateID`).

Scheduling can be used on its own through the `scheduler` package: create a
scheduler with `scheduler.New()`, register callbacks with
`Schedule(crontab, func(context.Context) error, scheduler.JobConfig{Name: "cleanup", Namespace: "owner"})`, then call `Start()` and `Shutdown()`
to manage its lifecycle. Direct tasks run in the current process and need no app,
queue, or registered job.

For app and plugin scheduling, supply the instance in `app.Options.Scheduler`.
`Schedule` runs callbacks directly; `ScheduleDispatch(crontab, jobName, payload, dispatchConfig, scheduleConfig)`
enqueues registered jobs for queue workers. Plugin job namespaces are supplied
automatically. Start the scheduler after successful setup and boot.

App and plugin crons receive a UUID and a reserved `namespace:<name>` tag.
The host namespace is `owner`; plugin namespaces use the plugin's name.
Every cron requires a non-empty `JobConfig.Name`, including dispatch schedules.
Empty names return `scheduler.ErrEmptyName`. `JobConfig.ID` is optional: omitting
it or passing `uuid.Nil` generates a UUID; supplied IDs are preserved.
Supply one `scheduler.JobConfig` to set its identity, add tags, or configure execution:

```go
id := uuid.New() // github.com/google/uuid
err := app.Schedule("0 2 * * *", cleanup, scheduler.JobConfig{
    ID:   id,
    Name: "nightly-cleanup",
    Tags: []string{"maintenance"},
    Options: []gocron.JobOption{
        gocron.WithContext(ctx),
        gocron.WithSingletonMode(gocron.LimitModeReschedule),
    },
})
```

`ScheduleDispatch` accepts the same configuration after its dispatch
config. The framework sets namespace ownership, preserving custom tags.
Namespace tags are reserved; set other labels through `JobConfig.Tags`.
`JobConfig` metadata takes precedence over metadata in its raw `Options`.
Explicit IDs must be unique across the scheduler; duplicates return
`scheduler.ErrDuplicateID`.

Use `app.ScheduledJobs()` to inspect IDs, names, tags, cron expressions,
next/last run times, and running status through gocron's job handles, or call
`RunNow()` on a handle. `app.RemoveScheduledJob(id)` removes a schedule and
cancels its running callback's context. Plugins list and remove only their own
schedules; callbacks must observe cancellation to stop promptly. Removing a
dispatch schedule stops future enqueueing; already queued jobs remain queued.

For standalone use, `sc.Schedule(crontab, task, scheduler.JobConfig{Name: "cleanup", Namespace: "owner"})`
returns `(gocron.Job, error)`. Namespace is required and must identify the host
or owning plugin; an empty namespace returns `scheduler.ErrEmptyNamespace`.
`sc.JobsInNamespace(namespace)` and `sc.RemoveJobInNamespace(namespace, id)`
provide namespace-scoped inspection and removal. Put additional gocron options
in `JobConfig.Options`.

Custom `plugin.Options.Schedule` and `ScheduleDispatch` callbacks now accept a
trailing `scheduler.JobConfig` parameter.

Enable the pre-commit checks after cloning:

```sh
git config core.hooksPath .githooks
```

The hook runs `make fmt`, then runs tests and lint in parallel, failing if either check fails.
Formatting can change working files; review and stage those changes before committing.
Skip the checks when needed with `git commit --no-verify`.
