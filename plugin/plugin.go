package plugin

import (
	"github.com/go-hypercube/go-hypercube/cmd"
	"github.com/go-hypercube/go-hypercube/migration"
	"github.com/go-hypercube/go-hypercube/seeder"
)

type Plugin interface {
	// Name returns the unique identifier for this plugin.
	Name() string

	// Version returns this plugin's own semantic version, e.g.
	// "v1.4.2". Must be a valid version string accepted by
	// golang.org/x/mod/semver so other plugins can depend on a minimum
	// version of it.
	Version() string

	// Dependencies returns the other plugins that must be registered,
	// resolved, and at least at the given minimum version before this
	// one.
	//
	// Note: These dependencies refer to plugins already installed and
	// added to the framework by the developer. The framework itself
	// will not fetch or install any new Go modules.
	Dependencies() []DependencyDesc

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
	Seeders    []seeder.Seeder
}

// DependencyDesc names a plugin dependency and the minimum version of
// it required. Version must be a valid semantic version accepted by
// golang.org/x/mod/semver (e.g. "v1.2.0" — note the leading "v" that
// package requires), or empty to accept any version of the dependency.
type DependencyDesc struct {
	ID      string
	Version string
}
