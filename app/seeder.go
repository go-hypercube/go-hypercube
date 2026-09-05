package app

import (
	"fmt"

	"github.com/go-hypercube/go-hypercube/seeder"
)

// Seeders returns all seeders registered with the app across every
// namespace (both framework-owned and plugin-owned), in registration
// order.
func (app *App) Seeders() seeder.NamespacedSlice { return app.seeders }

// RegisterSeeder registers seeders under the framework's own reserved
// namespace (frameworkDevNamespace), as opposed to a plugin's
// namespace.
func (app *App) RegisterSeeder(seeders ...seeder.Seeder) error {
	return app.registerSeederForNamespace(hostAppNamespace, seeders...)
}

// registerSeederForNamespace wraps each of seeders in a
// seeder.Namespaced under namespace and appends them to app.seeders.
func (app *App) registerSeederForNamespace(namespace string, seeders ...seeder.Seeder) error {
	namespacedSeeders := make([]*seeder.Namespaced, len(seeders))
	for i, s := range seeders {
		namespacedSeeders[i] = seeder.NewNamespaced(namespace, s)
	}
	app.seeders = append(app.seeders, namespacedSeeders...)
	return nil
}

// seedersTable is the name of the bookkeeping table the framework uses
// to record which (namespace, seeder name) pairs have already run.
const seedersTable = "hypercube_seeders"

// ensureSeedersTable creates the seeder tracking table if it does not
// already exist. Safe to call repeatedly.
func (app *App) ensureSeedersTable() error {
	_, err := app.database.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			namespace TEXT NOT NULL,
			name      TEXT NOT NULL,
			run_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (namespace, name)
		)`, seedersTable))
	return err
}

// hasRun reports whether the seeder (namespace, name) has already been
// recorded as run.
func (app *App) hasRun(namespace, name string) (bool, error) {
	dbDriver := app.readDbDriver()
	query := fmt.Sprintf(
		`SELECT EXISTS(SELECT 1 FROM %s WHERE namespace = %s AND name = %s)`,
		seedersTable, dbDriver.placeholder(1), dbDriver.placeholder(2),
	)
	var exists bool
	if err := app.database.QueryRow(query, namespace, name).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// markRun records that (namespace, name) has run. INSERT OR REPLACE /
// upsert semantics aren't needed here since callers only call markRun
// after confirming (via hasRun, or unconditionally under --force) that
// it's safe to (re-)record — see runSeeder below, which deletes any
// existing record before re-inserting on a forced re-run.
func (app *App) markRun(namespace, name string) error {
	dbDriver := app.readDbDriver()
	query := fmt.Sprintf(
		`INSERT INTO %s (namespace, name) VALUES (%s, %s)`,
		seedersTable, dbDriver.placeholder(1), dbDriver.placeholder(2),
	)
	_, err := app.database.Exec(query, namespace, name)
	return err
}

// clearRun removes the run record for (namespace, name), if any. Used
// by runSeeder to reset state before a forced re-run so markRun's
// INSERT doesn't collide with the existing primary key.
func (app *App) clearRun(namespace, name string) error {
	dbDriver := app.readDbDriver()
	query := fmt.Sprintf(
		`DELETE FROM %s WHERE namespace = %s AND name = %s`,
		seedersTable, dbDriver.placeholder(1), dbDriver.placeholder(2),
	)
	_, err := app.database.Exec(query, namespace, name)
	return err
}

// RunSeeder runs the seeder registered under namespace with the given
// name.
//
// If the seeder has already run and force is false, RunSeeder is a
// no-op and returns nil. If force is true, the seeder runs regardless
// of prior state, and its run record is refreshed (cleared, then
// re-recorded) rather than skipped or duplicated.
//
// Returns an error if no seeder matches (namespace, name), or if
// running it fails — in which case no run record is written, so a
// subsequent call (with or without force) will attempt it again.
func (app *App) RunSeeder(namespace, name string, force bool) error {
	if err := app.ensureSeedersTable(); err != nil {
		return fmt.Errorf("ensure seeders table: %w", err)
	}

	s := app.seeders.GetSeeder(namespace, name)
	if s == nil {
		return fmt.Errorf("seeder %q not found in namespace %q", name, namespace)
	}

	if !force {
		ran, err := app.hasRun(namespace, name)
		if err != nil {
			return fmt.Errorf("check run state of %q: %w", name, err)
		}
		if ran {
			return nil
		}
	}

	if err := s.Run(seeder.NewAppForSeeder(s, app.database, app.cache, app.services)); err != nil {
		return fmt.Errorf("run seeder %q/%q: %w", namespace, name, err)
	}

	if force {
		if err := app.clearRun(namespace, name); err != nil {
			return fmt.Errorf("clear previous run record for %q/%q: %w", namespace, name, err)
		}
	}
	if err := app.markRun(namespace, name); err != nil {
		return fmt.Errorf("record run for %q/%q: %w", namespace, name, err)
	}
	return nil
}

// RunSeedersForNamespace runs every seeder registered under namespace,
// in Name order. Seeders that have already run are skipped unless
// force is true, in which case every seeder in the namespace re-runs
// regardless of prior state.
//
// Returns the first error encountered from RunSeeder; seeders before
// the failure remain in their newly-run state.
func (app *App) RunSeedersForNamespace(namespace string, force bool) error {
	for _, s := range app.seeders.GetNamespace(namespace) {
		if err := app.RunSeeder(namespace, s.Name(), force); err != nil {
			return err
		}
	}
	return nil
}

// SeedAll runs every registered seeder across every namespace.
//
// Order matters here, mirroring MigrateAllUp: plugin namespaces are
// seeded first, in the exact order app.plugins is already sorted into
// by initPlugins (dependency order). Any remaining namespaces (for
// example frameworkDevNamespace) are seeded afterward, in sorted order.
//
// If force is false, seeders that have already run are skipped. If
// force is true, every seeder in every namespace re-runs.
//
// SeedAll must be called after Setup(); it returns an error if Setup()
// has not run yet.
//
// Returns the first error encountered; namespaces seeded so far remain
// in their newly-run state.
func (app *App) SeedAll(force bool) error {
	if !app.didSetup {
		return fmt.Errorf("cannot seed before setting up the framework; did you forget to call Setup()")
	}

	seeded := make(map[string]struct{}, len(app.plugins))

	for _, p := range app.plugins {
		namespace := p.Name()
		if err := app.RunSeedersForNamespace(namespace, force); err != nil {
			return err
		}
		seeded[namespace] = struct{}{}
	}

	for _, namespace := range app.seeders.Namespaces() {
		if _, done := seeded[namespace]; done {
			continue
		}
		if err := app.RunSeedersForNamespace(namespace, force); err != nil {
			return err
		}
	}
	return nil
}
