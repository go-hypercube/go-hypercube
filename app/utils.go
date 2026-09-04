package app

import (
	"fmt"
)

type dbDriver int

const (
	// DialectUnknown defaults to "?" placeholders (MySQL, SQLite,
	// and most other database/sql drivers rewrite or accept this).
	dialectUnknown dbDriver = iota
	dialectPostgres
	dialectMySQL
	dialectSQLite
)

// placeholder returns the driver-appropriate placeholder for the nth
// (1-indexed) bound parameter in a query.
func (d dbDriver) placeholder(n int) string {
	switch d {
	case dialectPostgres:
		return fmt.Sprintf("$%d", n)
	default:
		return "?"
	}
}

func (app *App) readDbDriver() dbDriver {
	driverName := app.config.ReadString("DB_DRIVER")
	switch driverName {
	case "postgres":
		return dialectPostgres
	case "mysql":
		return dialectMySQL
	case "sqlite":
		return dialectSQLite
	default:
		return dialectUnknown
	}
}
