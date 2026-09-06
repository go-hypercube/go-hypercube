package app

import (
	"testing"

	"github.com/go-hypercube/go-hypercube/internal/structure"
	"github.com/go-hypercube/go-hypercube/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- UsePlugin ----

func TestUsePlugin(t *testing.T) {
	t.Run("registers plugins successfully", func(t *testing.T) {
		app := &App{}
		p1 := &fakePlugin{name: "auth", version: "v1.0.0"}
		p2 := &fakePlugin{name: "billing", version: "v2.1.0"}

		err := app.UsePlugin(p1, p2)
		require.NoError(t, err)
		assert.Equal(t, []plugin.Plugin{p1, p2}, app.Plugins())
	})

	t.Run("rejects use after Setup", func(t *testing.T) {
		app := &App{didSetup: true}
		err := app.UsePlugin(&fakePlugin{name: "auth"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot add or use a plugin after setting up the framework")
	})

	t.Run("rejects plugin named after the reserved host-app namespace", func(t *testing.T) {
		app := &App{}
		err := app.UsePlugin(&fakePlugin{name: hostAppNamespace})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `cannot use "owner" as plugin name`)
	})

	t.Run("rejects duplicate plugin name within the same call", func(t *testing.T) {
		app := &App{}
		err := app.UsePlugin(&fakePlugin{name: "auth"}, &fakePlugin{name: "auth"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `duplicate plugin name "auth"`)
	})

	t.Run("rejects duplicate plugin name across separate calls", func(t *testing.T) {
		app := &App{}
		require.NoError(t, app.UsePlugin(&fakePlugin{name: "auth"}))

		err := app.UsePlugin(&fakePlugin{name: "auth"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `duplicate plugin name "auth"`)
	})

	t.Run("accepts a plugin with no version declared", func(t *testing.T) {
		app := &App{}
		err := app.UsePlugin(&fakePlugin{name: "auth", version: ""})
		require.NoError(t, err)
	})

	t.Run("rejects a plugin with an invalid version string", func(t *testing.T) {
		app := &App{}
		err := app.UsePlugin(&fakePlugin{name: "auth", version: "not-a-version"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `plugin "auth" has an invalid version "not-a-version"`)
	})

	t.Run("rejects a version missing the required v prefix", func(t *testing.T) {
		app := &App{}
		err := app.UsePlugin(&fakePlugin{name: "auth", version: "1.0.0"}) // semver pkg requires "v1.0.0"
		require.Error(t, err)
		assert.Contains(t, err.Error(), `invalid version "1.0.0"`)
	})

	t.Run("failed call does not partially register plugins", func(t *testing.T) {
		app := &App{}
		err := app.UsePlugin(
			&fakePlugin{name: "good", version: "v1.0.0"},
			&fakePlugin{name: "bad", version: "not-a-version"},
		)
		require.Error(t, err)
		assert.Empty(t, app.Plugins(), "no plugins should be added when any plugin in the call is invalid")
	})
}

// ---- isVersionCompatible ----

func TestIsVersionCompatible(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		required string
		want     bool
	}{
		{"exact match", "v1.0.0", "v1.0.0", true},
		{"higher patch, same major", "v1.99.99", "v1.0.0", true},
		{"higher minor, same major", "v1.5.0", "v1.0.0", true},
		{"lower patch fails minimum", "v0.9.0", "v1.0.0", false},
		{"next major fails despite being numerically greater", "v2.0.0", "v1.0.0", false},
		{"previous major fails even if numerically close", "v0.99.99", "v1.0.0", false},
		{"major v0 exact", "v0.5.0", "v0.5.0", true},
		{"major v0 to v0 lower minor fails", "v0.4.0", "v0.5.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isVersionCompatible(tt.actual, tt.required))
		})
	}
}

// ---- checkPluginDependencies ----

func TestCheckPluginDependencies(t *testing.T) {
	t.Run("no problems: dependency exists and version compatible", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "auth", version: "v1.5.0"},
			&fakePlugin{name: "billing", version: "v1.0.0", deps: []plugin.DependencyDesc{
				{ID: "auth", Version: "v1.0.0"},
			}},
		}

		g := buildDependencyGraph(app.plugins)
		err := app.checkPluginDependencies(g)
		require.NoError(t, err)
	})

	t.Run("no problems: dependency with empty version accepts any version", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "auth", version: "v0.0.1"},
			&fakePlugin{name: "billing", deps: []plugin.DependencyDesc{
				{ID: "auth", Version: ""},
			}},
		}

		g := buildDependencyGraph(app.plugins)
		err := app.checkPluginDependencies(g)
		require.NoError(t, err)
	})

	t.Run("missing dependency", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "billing", deps: []plugin.DependencyDesc{
				{ID: "auth", Version: ""},
			}},
		}

		g := buildDependencyGraph(app.plugins)
		err := app.checkPluginDependencies(g)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `plugin 'billing' depends on 'auth', but 'auth' was not added to the app`)
	})

	t.Run("registered dependency below required minimum version", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "auth", version: "v0.9.0"},
			&fakePlugin{name: "billing", deps: []plugin.DependencyDesc{
				{ID: "auth", Version: "v1.0.0"},
			}},
		}

		g := buildDependencyGraph(app.plugins)
		err := app.checkPluginDependencies(g)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `plugin 'billing' requires 'auth' at a version compatible with v1.0.0`)
		assert.Contains(t, err.Error(), `but v0.9.0 is registered`)
	})

	t.Run("registered dependency on next major version fails", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "auth", version: "v2.0.0"},
			&fakePlugin{name: "billing", deps: []plugin.DependencyDesc{
				{ID: "auth", Version: "v1.0.0"},
			}},
		}

		g := buildDependencyGraph(app.plugins)
		err := app.checkPluginDependencies(g)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `plugin 'billing' requires 'auth' at a version compatible with v1.0.0`)
	})

	t.Run("invalid required version string on the dependent", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "auth", version: "v1.0.0"},
			&fakePlugin{name: "billing", deps: []plugin.DependencyDesc{
				{ID: "auth", Version: "not-a-version"},
			}},
		}

		g := buildDependencyGraph(app.plugins)
		err := app.checkPluginDependencies(g)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `plugin 'billing' declares an invalid required version "not-a-version" for dependency 'auth'`)
	})

	t.Run("invalid version on the registered dependency itself", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "auth", version: "garbage"},
			&fakePlugin{name: "billing", deps: []plugin.DependencyDesc{
				{ID: "auth", Version: "v1.0.0"},
			}},
		}

		g := buildDependencyGraph(app.plugins)
		err := app.checkPluginDependencies(g)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `plugin 'auth' has an invalid version "garbage", so 'billing' requiring it cannot be checked`)
	})

	t.Run("collects multiple problems into a single error", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "billing", deps: []plugin.DependencyDesc{
				{ID: "auth", Version: ""},      // missing
				{ID: "reporting", Version: ""}, // also missing
			}},
		}

		g := buildDependencyGraph(app.plugins)
		err := app.checkPluginDependencies(g)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auth")
		assert.Contains(t, err.Error(), "reporting")
	})

	t.Run("no plugins, no dependencies: no error", func(t *testing.T) {
		app := &App{}
		g := structure.NewGraph[string]()
		err := app.checkPluginDependencies(g)
		require.NoError(t, err)
	})
}

// buildDependencyGraph mirrors the graph-construction logic in
// initPlugins, so checkPluginDependencies can be tested directly
// against a graph built the same way initPlugins builds it.
func buildDependencyGraph(plugins []plugin.Plugin) *structure.Graph[string] {
	g := structure.NewGraph[string]()
	for _, p := range plugins {
		g.AddVertex(p.Name())
		for _, dep := range p.Dependencies() {
			g.AddEdge(p.Name(), dep.ID)
		}
	}
	return g
}

// ---- initPlugins ----

func TestInitPlugins(t *testing.T) {
	t.Run("no plugins: no-op", func(t *testing.T) {
		app := &App{}
		err := app.initPlugins()
		require.NoError(t, err)
	})

	t.Run("sorts plugins into dependency order", func(t *testing.T) {
		app := &App{}
		pAuth := &fakePlugin{name: "auth", version: "v1.0.0"}
		pBilling := &fakePlugin{name: "billing", version: "v1.0.0", deps: []plugin.DependencyDesc{
			{ID: "auth", Version: ""},
		}}
		// registered out of dependency order
		app.plugins = []plugin.Plugin{pBilling, pAuth}

		err := app.initPlugins()
		require.NoError(t, err)

		require.Len(t, app.plugins, 2)
		assert.Equal(t, "auth", app.plugins[0].Name(), "dependency must come before dependent")
		assert.Equal(t, "billing", app.plugins[1].Name())
	})

	t.Run("sorts a multi-level dependency chain correctly", func(t *testing.T) {
		app := &App{}
		// billing depends on auth, reporting depends on billing:
		// reporting -> billing -> auth
		pAuth := &fakePlugin{name: "auth"}
		pBilling := &fakePlugin{name: "billing", deps: []plugin.DependencyDesc{{ID: "auth"}}}
		pReporting := &fakePlugin{name: "reporting", deps: []plugin.DependencyDesc{{ID: "billing"}}}

		// registered in a scrambled order
		app.plugins = []plugin.Plugin{pReporting, pAuth, pBilling}

		err := app.initPlugins()
		require.NoError(t, err)

		require.Len(t, app.plugins, 3)
		assert.Equal(t, "auth", app.plugins[0].Name())
		assert.Equal(t, "billing", app.plugins[1].Name())
		assert.Equal(t, "reporting", app.plugins[2].Name())
	})

	t.Run("independent plugins with no shared dependencies can appear in any relative order, but each dependency still precedes its dependent", func(t *testing.T) {
		app := &App{}
		pAuth := &fakePlugin{name: "auth"}
		pBilling := &fakePlugin{name: "billing", deps: []plugin.DependencyDesc{{ID: "auth"}}}
		pStandalone := &fakePlugin{name: "standalone"} // no deps at all

		app.plugins = []plugin.Plugin{pBilling, pStandalone, pAuth}

		err := app.initPlugins()
		require.NoError(t, err)

		require.Len(t, app.plugins, 3)
		authIdx := indexOfPlugin(app.plugins, "auth")
		billingIdx := indexOfPlugin(app.plugins, "billing")
		require.NotEqual(t, -1, authIdx)
		require.NotEqual(t, -1, billingIdx)
		assert.Less(t, authIdx, billingIdx, "auth must be initialized before billing regardless of where standalone lands")
	})

	t.Run("propagates missing-dependency error and does not reorder", func(t *testing.T) {
		app := &App{}
		pBilling := &fakePlugin{name: "billing", deps: []plugin.DependencyDesc{
			{ID: "auth", Version: ""},
		}}
		app.plugins = []plugin.Plugin{pBilling}

		err := app.initPlugins()
		require.Error(t, err)
		assert.Contains(t, err.Error(), `plugin 'billing' depends on 'auth'`)
	})

	t.Run("propagates version-incompatible dependency error", func(t *testing.T) {
		app := &App{}
		pAuth := &fakePlugin{name: "auth", version: "v0.1.0"}
		pBilling := &fakePlugin{name: "billing", deps: []plugin.DependencyDesc{
			{ID: "auth", Version: "v1.0.0"},
		}}
		app.plugins = []plugin.Plugin{pAuth, pBilling}

		err := app.initPlugins()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires 'auth' at a version compatible with v1.0.0")
	})

	t.Run("propagates circular dependency error", func(t *testing.T) {
		app := &App{}
		pA := &fakePlugin{name: "a", deps: []plugin.DependencyDesc{{ID: "b"}}}
		pB := &fakePlugin{name: "b", deps: []plugin.DependencyDesc{{ID: "a"}}}
		app.plugins = []plugin.Plugin{pA, pB}

		err := app.initPlugins()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "circular dependency exists among plugins")
	})

	t.Run("dependency graph errors are caught before the cycle check would even matter", func(t *testing.T) {
		// A missing dependency must be reported even if unrelated
		// plugins in the same call also form a cycle — missing-plugin
		// problems should surface rather than being masked by cycle
		// detection running first.
		app := &App{}
		pA := &fakePlugin{name: "a", deps: []plugin.DependencyDesc{{ID: "b"}}}
		pB := &fakePlugin{name: "b", deps: []plugin.DependencyDesc{{ID: "a"}}}
		pC := &fakePlugin{name: "c", deps: []plugin.DependencyDesc{{ID: "does-not-exist"}}}
		app.plugins = []plugin.Plugin{pA, pB, pC}

		err := app.initPlugins()
		require.Error(t, err)
		assert.Contains(t, err.Error(), `depends on 'does-not-exist'`)
	})
}

// ---- sortPluginsByIdOrder ----

func TestSortPluginsByIdOrder(t *testing.T) {
	t.Run("reorders plugins to match the given id order", func(t *testing.T) {
		app := &App{}
		pA := &fakePlugin{name: "a"}
		pB := &fakePlugin{name: "b"}
		pC := &fakePlugin{name: "c"}
		app.plugins = []plugin.Plugin{pA, pB, pC}

		app.sortPluginsByIdOrder([]string{"c", "a", "b"})

		require.Len(t, app.plugins, 3)
		assert.Equal(t, "c", app.plugins[0].Name())
		assert.Equal(t, "a", app.plugins[1].Name())
		assert.Equal(t, "b", app.plugins[2].Name())
	})

	t.Run("ids not present in app.plugins are silently skipped", func(t *testing.T) {
		app := &App{}
		pA := &fakePlugin{name: "a"}
		app.plugins = []plugin.Plugin{pA}

		app.sortPluginsByIdOrder([]string{"does-not-exist", "a"})

		require.Len(t, app.plugins, 1)
		assert.Equal(t, "a", app.plugins[0].Name())
	})

	t.Run("plugins not mentioned in orderedIds are dropped", func(t *testing.T) {
		app := &App{}
		pA := &fakePlugin{name: "a"}
		pB := &fakePlugin{name: "b"}
		app.plugins = []plugin.Plugin{pA, pB}

		app.sortPluginsByIdOrder([]string{"a"}) // "b" omitted

		require.Len(t, app.plugins, 1)
		assert.Equal(t, "a", app.plugins[0].Name())
	})

	t.Run("empty orderedIds empties app.plugins", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{&fakePlugin{name: "a"}}

		app.sortPluginsByIdOrder(nil)

		assert.Empty(t, app.plugins)
	})
}

// indexOfPlugin returns the index of the plugin named name within
// plugins, or -1 if not present.
func indexOfPlugin(plugins []plugin.Plugin, name string) int {
	for i, p := range plugins {
		if p.Name() == name {
			return i
		}
	}
	return -1
}
