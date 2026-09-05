package app

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-hypercube/go-hypercube/plugin"
	"github.com/stretchr/testify/require"
)

// fakePlugin is a minimal plugin.Plugin used across tests. It supports
// a version and a list of dependencies so it can exercise UsePlugin,
// initPlugins, checkPluginDependencies, and SortPluginsByIdOrder.
type fakePlugin struct {
	name    string
	version string
	deps    []plugin.DependencyDesc
}

func (p *fakePlugin) Name() string                          { return p.name }
func (p *fakePlugin) Version() string                       { return p.version }
func (p *fakePlugin) Dependencies() []plugin.DependencyDesc { return p.deps }
func (p *fakePlugin) Register(_ *plugin.App) (*plugin.Registration, error) {
	return &plugin.Registration{}, nil
}
func (p *fakePlugin) Boot(_ *plugin.App) error { return nil }

// newMockApp returns an *App wired to a sqlmock database, plus the mock
// controller for setting expectations. driverName may be "" to exercise
// the dialectUnknown ("?") path, or "postgres"/"mysql"/"sqlite".
func newMockApp(t *testing.T, driverName string) (*App, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	app := &App{
		config:   newFakeConfig(map[string]string{"DB_DRIVER": driverName}),
		database: db,
	}
	return app, mock
}

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
