package plugin

import (
	"github.com/go-hypercube/go-hypercube/cmd"
	"github.com/go-hypercube/go-hypercube/migration"
)

type Plugin interface {
	// Name returns the unique identifier for this plugin.
	Name() string

	// Dependencies returns the names (IDs) of other plugins that must be
	// registered and resolved before this one.
	//
	// Note: These dependencies refer to plugins already installed and added
	// to the framework by the user. The framework itself will not fetch or
	// install any new Go modules.
	Dependencies() []string

	// Register performs the plugin's registration logic and returns a
	// Registration handle.
	Register(app *App) (*Registration, error)

	// Boot executes the plugin's startup logic after all dependencies
	// are resolved and registered.
	Boot(app *App) error
}

type Registration struct {
	Migrations []*migration.Migration
	Cmds       []cmd.Command
}
