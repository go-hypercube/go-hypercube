package app

import (
	"fmt"
	"strings"

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
		if name == frameworkDevNamespace {
			return fmt.Errorf("cannot use %q as plugin name: reserved for framework internal use", name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate plugin name %q: a plugin with this name is already registered or appears more than once in this call", name)
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
			pluginGraph.AddEdge(p.Name(), dep)
		}
	}
	if err := app.checkForMissingPlugins(pluginGraph); err != nil {
		return err
	}

	topologicalOrder, cyclePath := pluginGraph.GetTopologicalOrCycle()
	if cyclePath != nil {
		return fmt.Errorf(
			"cannot resolve plugin order: a circular dependency exists among plugins [%s]. each plugin must have acyclic dependencies",
			strings.Join(cyclePath, " → "),
		)
	}
	app.SortPluginsByIdOrder(topologicalOrder)

	return nil
}

func (app *App) SortPluginsByIdOrder(orderedIds []string) {
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

func (app *App) checkForMissingPlugins(g *structure.Graph[string]) error {
	var missing []string
	for p, deps := range g.Vertices {
		for _, dep := range deps.Slice() {
			if _, exists := g.Vertices[dep]; !exists {
				missing = append(missing, fmt.Sprintf(
					"plugin '%s' depends on '%s', but '%s' was not added to the app",
					p, dep, dep,
				))
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing plugin dependencies:\n%s", strings.Join(missing, "\n"))
	}
	return nil
}
