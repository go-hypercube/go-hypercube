# go-hypercube

This is an evolution & learned lessons from the legacy [go-light-framework](https://github.com/Nidal-Bakir/go-light-framework)

A batteries-included Go backend framework
---

Scheduling can be used on its own through the `scheduler` package: create a
scheduler with `scheduler.New()`, register callbacks with
`Every(crontab, func(context.Context) error)`, then call `Start()` and `Shutdown()`
to manage its lifecycle. Direct tasks run in the current process and need no app,
queue, or registered job.

For app and plugin scheduling, supply the instance in `app.Options.Scheduler`.
`Every` runs callbacks directly; `EveryDispatch(crontab, jobName, payload, config)`
enqueues registered jobs for queue workers. Plugin job namespaces are supplied
automatically. Start the scheduler after successful setup and boot.

Enable the pre-commit checks after cloning:

```sh
git config core.hooksPath .githooks
```

The hook runs `make lint`, `make fmt`, and `make test`, stopping on failure.
Formatting can change working files; review and stage those changes before committing.
Skip the checks when needed with `git commit --no-verify`.
