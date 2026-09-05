package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDbDriver_Placeholder(t *testing.T) {
	tests := []struct {
		name   string
		driver dbDriver
		n      int
		want   string
	}{
		{"postgres first param", dialectPostgres, 1, "$1"},
		{"postgres second param", dialectPostgres, 2, "$2"},
		{"postgres large index", dialectPostgres, 42, "$42"},
		{"mysql uses question mark", dialectMySQL, 1, "?"},
		{"sqlite uses question mark", dialectSQLite, 3, "?"},
		{"unknown defaults to question mark", dialectUnknown, 1, "?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.driver.placeholder(tt.n))
		})
	}
}

func TestApp_ReadDbDriver(t *testing.T) {
	tests := []struct {
		name       string
		driverName string
		want       dbDriver
	}{
		{"postgres", "postgres", dialectPostgres},
		{"mysql", "mysql", dialectMySQL},
		{"sqlite", "sqlite", dialectSQLite},
		{"empty falls back to unknown", "", dialectUnknown},
		{"unrecognized falls back to unknown", "mssql", dialectUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{config: newFakeConfig(map[string]string{"DB_DRIVER": tt.driverName})}
			assert.Equal(t, tt.want, app.readDbDriver())
		})
	}
}
