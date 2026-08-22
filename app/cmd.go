package app

import "github.com/go-hypercube/go-hypercube/cmd"

func (app *App) RegisterCommand(cmds ...cmd.Command) error {
	return app.registerCommandForNamespace(frameworkDevNamespace, cmds...)
}

func (app *App) registerCommandForNamespace(namespace string, cmds ...cmd.Command) error {
	namespacedCmds := make([]*cmd.Namespaced, len(cmds))
	for i, m := range cmds {
		namespacedCmds[i] = cmd.NewNamespaced(namespace, m)
	}
	app.cmds = append(app.cmds, namespacedCmds...)
	return nil
}
