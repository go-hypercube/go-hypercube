package app

import (
	"fmt"

	"github.com/go-hypercube/go-hypercube/cmd"
)

// RegisterCommand registers cmds under the framework's own reserved
// namespace (frameworkDevNamespace), as opposed to a plugin's namespace.
// It is a thin wrapper around registerCommandForNamespace for
// framework-owned commands (e.g. built-in dev/debug commands) rather
// than ones contributed by a plugin.
func (app *App) RegisterCommand(cmds ...cmd.Command) error {
	return app.registerCommandForNamespace(hostAppNamespace, cmds...)
}

// registerCommandForNamespace wraps each of cmds in a cmd.Namespaced
// under namespace and adds them to app.cmds.
//
// If a command with the same (namespace, Name()) is already registered,
// the existing entry is replaced in place rather than appended
// alongside it — the new registration wins. This mirrors
// registerSeederForNamespace: a command is a live piece of behavior, so
// re-registering the same name is treated as an intentional
// replacement (e.g. a plugin overriding a default command), not an
// error.
func (app *App) registerCommandForNamespace(namespace string, cmds ...cmd.Command) error {
	for _, c := range cmds {
		namespaced := cmd.NewNamespaced(namespace, c)

		replaced := false
		for i, existing := range app.cmds {
			if existing.Namespace == namespace && existing.Name() == c.Name() {
				app.cmds[i] = namespaced
				replaced = true
				break
			}
		}
		if !replaced {
			app.cmds = append(app.cmds, namespaced)
		}
	}
	return nil
}

// RunCommand looks up the command registered under namespace with the
// given cmdName and executes it, wiring up a cmd.App populated with the
// framework's database, cache, and service container — the same
// dependencies a plugin-registered command would receive.
//
// Returns an error if no command matches (namespace, cmdName). Otherwise
// it returns whatever value and error the command's Run method produces,
// letting commands hand back arbitrary results (e.g. a report struct, a
// count, a generated ID) to the caller of RunCommand.
func (app *App) RunCommand(namespace, cmdName string) (any, error) {
	command := app.cmds.GetCommand(namespace, cmdName)
	if command == nil {
		return nil, fmt.Errorf("command %q not found in namespace %q", cmdName, namespace)
	}
	return command.Run(cmd.NewAppForCmd(
		&cmd.Options{
			Cmd:       command,
			Database:  app.database,
			Cache:     app.cache,
			Logger:    app.logger.With("namespace", namespace, "command", command.Name()),
			Container: app.services,
		},
	))
}
