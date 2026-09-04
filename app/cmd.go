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
	return app.registerCommandForNamespace(frameworkDevNamespace, cmds...)
}

// registerCommandForNamespace wraps each of cmds in a cmd.Namespaced
// under namespace and appends them to app.cmds. It never returns a
// non-nil error today, but keeps an error return so registration can
// gain validation (e.g. duplicate-name checks) later without changing
// the call sites in app/plugin.go and RegisterCommand.
func (app *App) registerCommandForNamespace(namespace string, cmds ...cmd.Command) error {
	namespacedCmds := make([]*cmd.Namespaced, len(cmds))
	for i, m := range cmds {
		namespacedCmds[i] = cmd.NewNamespaced(namespace, m)
	}
	app.cmds = append(app.cmds, namespacedCmds...)
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
		command,
		app.database,
		app.cache,
		app.services,
	))
}
