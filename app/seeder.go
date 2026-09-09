package app

import (
	"fmt"
	"time"

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
// seeder.Namespaced under namespace and adds them to app.seeders.
//
// If a seeder with the same (namespace, Name()) is already registered,
// the existing entry is replaced in place rather than appended
// alongside it — the new registration wins, and app.seeders never ends
// up holding two entries for the same (namespace, name) pair. This
// matters both for correctness of GetSeeder/GetNamespace lookups (which
// would otherwise silently return whichever entry happens to come
// first) and so RunSeedersForNamespace/SeedAll don't run the same
// logical seeder twice under one name.
func (app *App) registerSeederForNamespace(namespace string, seeders ...seeder.Seeder) error {
	for _, s := range seeders {
		namespaced := seeder.NewNamespaced(namespace, s)

		replaced := false
		for i, existing := range app.seeders {
			if existing.Namespace == namespace && existing.Name() == s.Name() {
				app.seeders[i] = namespaced
				replaced = true
				break
			}
		}
		if !replaced {
			app.seeders = append(app.seeders, namespaced)
		}
	}
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

	if err := s.Run(seeder.NewAppForSeeder(
		&seeder.Options{
			Seeder:    s,
			Database:  app.database,
			Cache:     app.cache,
			Logger:    app.logger.With("namespace", namespace, "seeder", s.Name()),
			Container: app.services,
		})); err != nil {
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

// SeederStatus describes a single registered seeder's status within
// its namespace: whether it has run, and when.
type SeederStatus struct {
	Namespace string
	Name      string
	HasRun    bool
	RanAt     *time.Time // nil if HasRun is false
}

// NamespaceSeederState summarizes the seeding state of a single
// namespace: every registered seeder (in ascending Name order) along
// with its run status, plus a count of how many are still pending.
//
// Unlike NamespaceMigrationState, there is no single "Current" seeder
// name — seeders aren't a linear, sequentially-applied history the way
// migrations are, so any subset can be run independently or re-run via
// force.
type NamespaceSeederState struct {
	Namespace string
	Pending   int // count of registered seeders not yet run
	Statuses  []SeederStatus
}

// SeederState returns the current run state of every namespace that
// has at least one registered seeder, ordered the same way SeedAll
// orders namespaces: plugin namespaces first (in app.plugins'
// dependency order), then any remaining namespaces (e.g.
// hostAppNamespace) in sorted order.
//
// SeederState reads the tracking table directly rather than calling
// hasRun per seeder, so it reflects the database's current state in a
// single query per namespace even for namespaces with many seeders.
//
// Returns an error if the seeders table doesn't exist yet (call
// ensureSeedersTable first, e.g. by running any seeder) or the query
// fails.
func (app *App) SeederState() ([]*NamespaceSeederState, error) {
	if err := app.ensureSeedersTable(); err != nil {
		return nil, fmt.Errorf("ensure seeders table: %w", err)
	}

	orderedNamespaces := app.orderedSeederNamespaces()
	result := make([]*NamespaceSeederState, 0, len(orderedNamespaces))

	for _, namespace := range orderedNamespaces {
		state, err := app.namespaceSeederState(namespace)
		if err != nil {
			return nil, fmt.Errorf("get seeder state for namespace %q: %w", namespace, err)
		}
		result = append(result, state)
	}
	return result, nil
}

// namespaceSeederState builds the NamespaceSeederState for a single
// namespace by fetching all run (name -> run_at) pairs in one query,
// then walking the namespace's registered seeders in order.
func (app *App) namespaceSeederState(namespace string) (*NamespaceSeederState, error) {
	ordered := app.seeders.GetNamespace(namespace) // sorted by Name ascending

	runTimes, err := app.seederRunTimes(namespace)
	if err != nil {
		return nil, err
	}

	state := &NamespaceSeederState{
		Namespace: namespace,
		Statuses:  make([]SeederStatus, 0, len(ordered)),
	}

	for _, s := range ordered {
		ranAt, ok := runTimes[s.Name()]
		status := SeederStatus{
			Namespace: namespace,
			Name:      s.Name(),
			HasRun:    ok,
		}
		if ok {
			t := ranAt
			status.RanAt = &t
		} else {
			state.Pending++
		}
		state.Statuses = append(state.Statuses, status)
	}
	return state, nil
}

// seederRunTimes returns a map of seeder name -> run_at for every
// seeder currently recorded as run under namespace.
func (app *App) seederRunTimes(namespace string) (map[string]time.Time, error) {
	dbDriver := app.readDbDriver()
	query := fmt.Sprintf(
		`SELECT name, run_at FROM %s WHERE namespace = %s`,
		seedersTable, dbDriver.placeholder(1),
	)
	rows, err := app.database.Query(query, namespace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]time.Time)
	for rows.Next() {
		var name string
		var ranAt time.Time
		if err := rows.Scan(&name, &ranAt); err != nil {
			return nil, err
		}
		result[name] = ranAt
	}
	return result, rows.Err()
}

// orderedSeederNamespaces returns the namespaces that have at least one
// registered seeder, ordered the same way SeedAll runs them: plugin
// namespaces first (in app.plugins' dependency order), then any
// remaining namespaces in sorted order.
func (app *App) orderedSeederNamespaces() []string {
	registered := make(map[string]struct{})
	for _, namespace := range app.seeders.Namespaces() {
		registered[namespace] = struct{}{}
	}

	seen := make(map[string]struct{}, len(app.plugins))
	ordered := make([]string, 0, len(registered))

	for _, p := range app.plugins {
		namespace := p.Name()
		if _, has := registered[namespace]; !has {
			continue
		}
		ordered = append(ordered, namespace)
		seen[namespace] = struct{}{}
	}

	for _, namespace := range app.seeders.Namespaces() { // already sorted
		if _, done := seen[namespace]; done {
			continue
		}
		ordered = append(ordered, namespace)
	}
	return ordered
}
