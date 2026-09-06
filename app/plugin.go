package app

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/go-hypercube/go-hypercube/internal/structure"
	"github.com/go-hypercube/go-hypercube/plugin"
)

func (app *App) UsePlugin(plugins ...plugin.Plugin) error {
	if app.didSetup {
		return fmt.Errorf("cannot add or use a plugin after setting up the framework")
	}

	seen := make(map[string]struct{}, len(app.plugins)+len(plugins))
	for _, p := range app.plugins {
		seen[p.Name()] = struct{}{}
	}

	for _, newPlugin := range plugins {
		name := newPlugin.Name()
		if name == hostAppNamespace {
			return fmt.Errorf("cannot use %q as plugin name: reserved for framework internal use", name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate plugin name %q: a plugin with this name is already registered or appears more than once in this call", name)
		}
		if newPlugin.Version() != "" && !semver.IsValid(newPlugin.Version()) {
			return fmt.Errorf("plugin %q has an invalid version %q: must be a valid semantic version (e.g. \"v1.2.0\")", name, newPlugin.Version())
		}
		seen[name] = struct{}{}
	}

	app.plugins = append(app.plugins, plugins...)
	return nil
}

func (app *App) Plugins() []plugin.Plugin { return app.plugins }

func (app *App) initPlugins() error {
	pluginGraph := structure.NewGraph[string]()
	for _, p := range app.plugins {
		pluginGraph.AddVertex(p.Name())
		for _, dep := range p.Dependencies() {
			pluginGraph.AddEdge(p.Name(), dep.ID)
		}
	}
	if err := app.checkPluginDependencies(pluginGraph); err != nil {
		return err
	}

	// Edges are built as p.Name() -> dep.ID ("plugin depends on dep"),
	// so GetTopologicalOrCycle's plain order would put dependents
	// before their dependencies. GetReverseTopologicalOrCycle gives the
	// order plugin initialization actually needs: each plugin's
	// dependencies fully initialized before the plugin itself.
	dependencyFirstOrder, cyclePath := pluginGraph.GetReverseTopologicalOrCycle()
	if cyclePath != nil {
		return fmt.Errorf(
			"cannot resolve plugin order: a circular dependency exists among plugins [%s]. each plugin must have acyclic dependencies",
			strings.Join(cyclePath, " → "),
		)
	}
	app.sortPluginsByIdOrder(dependencyFirstOrder)

	return nil
}

// checkPluginDependencies walks every dependency edge in g and reports
// two kinds of problems in a single pass:
//
//   - a plugin depends on another plugin that was never added to the
//     app at all (g has no vertex for it), and
//   - a plugin depends on a version of another plugin that IS
//     registered, but whose actual version doesn't satisfy the
//     dependency's required minimum (see isVersionCompatible for the
//     exact compatibility rule).
//
// A dependency with an empty DependencyDesc.Version accepts any
// version of the target plugin and is only checked for existence, not
// compatibility. All problems found are collected and returned
// together in a single error, rather than stopping at the first one,
// so a developer can fix every broken dependency in one pass instead
// of playing whack-a-mole.
func (app *App) checkPluginDependencies(g *structure.Graph[string]) error {
	byName := make(map[string]plugin.Plugin, len(app.plugins))
	for _, p := range app.plugins {
		byName[p.Name()] = p
	}

	var problems []string
	for name, deps := range g.Vertices {
		for _, depID := range deps {
			depPlugin, exists := byName[depID]
			if !exists {
				problems = append(problems, fmt.Sprintf(
					"plugin '%s' depends on '%s', but '%s' was not added to the app",
					name, depID, depID,
				))
				continue
			}

			depDesc := findDependencyDesc(byName[name], depID)
			if depDesc.Version == "" {
				continue // any version is acceptable
			}

			if !semver.IsValid(depDesc.Version) {
				problems = append(problems, fmt.Sprintf(
					"plugin '%s' declares an invalid required version %q for dependency '%s'",
					name, depDesc.Version, depID,
				))
				continue
			}

			actual := depPlugin.Version()
			if !semver.IsValid(actual) {
				problems = append(problems, fmt.Sprintf(
					"plugin '%s' has an invalid version %q, so '%s' requiring it cannot be checked",
					depID, actual, name,
				))
				continue
			}

			if !isVersionCompatible(actual, depDesc.Version) {
				problems = append(problems, fmt.Sprintf(
					"plugin '%s' requires '%s' at a version compatible with %s (>= %s, < next major), but %s is registered",
					name, depID, depDesc.Version, depDesc.Version, actual,
				))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("plugin dependency problems:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

// findDependencyDesc returns the DependencyDesc within p.Dependencies()
// whose ID matches depID. p is expected to always have such an entry,
// since callers only invoke this for edges already present in the
// dependency graph (which is itself built directly from
// p.Dependencies()) — it returns a zero-value DependencyDesc if no
// match is found, which is treated as "any version acceptable".
func findDependencyDesc(p plugin.Plugin, depID string) plugin.DependencyDesc {
	for _, d := range p.Dependencies() {
		if d.ID == depID {
			return d
		}
	}
	return plugin.DependencyDesc{}
}

// isVersionCompatible reports whether actual satisfies a caret-style
// ("^") minimum version constraint of required: actual must be greater
// than or equal to required, and must share the same major version. For
// example, with required "v1.0.0": "v1.0.0" and "v1.99.99" are
// compatible, but "v0.9.0" and "v2.0.0" are not.
//
// Both actual and required must already be valid semver strings
// accepted by golang.org/x/mod/semver — callers are expected to check
// semver.IsValid beforehand.
func isVersionCompatible(actual, required string) bool {
	if semver.Major(actual) != semver.Major(required) {
		return false
	}
	return semver.Compare(actual, required) >= 0
}

func (app *App) sortPluginsByIdOrder(orderedIds []string) {
	l := len(app.plugins)
	pluginLookup := make(map[string]plugin.Plugin, l)
	for _, p := range app.plugins {
		pluginLookup[p.Name()] = p
	}

	sorted := make([]plugin.Plugin, 0, l)
	for _, id := range orderedIds {
		if p, ok := pluginLookup[id]; ok {
			sorted = append(sorted, p)
		}
	}
	app.plugins = sorted
}
