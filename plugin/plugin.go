package plugin

import (
	"github.com/go-hypercube/go-hypercube/cmd"
	"github.com/go-hypercube/go-hypercube/migration"
)

type Plugin interface {
	Name() string
	Register(app *App) (Registration, error)
	Boot(app *App) error
}

type Registration struct {
	Migrations []*migration.Migration
	Cmds       []cmd.Command
}
