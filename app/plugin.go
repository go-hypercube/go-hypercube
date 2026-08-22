package app

import (
	"fmt"

	"github.com/go-hypercube/go-hypercube/plugin"
)

func (app *App) UsePlugin(plugins ...plugin.Plugin) error {
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
