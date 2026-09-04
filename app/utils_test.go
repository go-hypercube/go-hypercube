package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeConfig is a minimal config.Config implementation for tests that
// only need to control DB_DRIVER (or another single key).
type fakeConfig struct {
	values map[string]string
}

func newFakeConfig(kv map[string]string) *fakeConfig {
	return &fakeConfig{values: kv}
}

func (c *fakeConfig) ReadString(key string) string { return c.values[key] }
func (c *fakeConfig) ReadInt(key string) int64     { return 0 }
func (c *fakeConfig) ReadFloat(key string) float64 { return 0 }

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
