package app

import (
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/go-hypercube/go-hypercube/migration"
)

// Migrations returns all migrations registered with the app across every
// namespace (both framework-owned and plugin-owned), in registration
// order. Use migration.NamespacedSlice's own helpers (GetNamespace,
// GroupByNamespace, Namespaces, etc.) to filter or group the result.
func (app *App) Migrations() migration.NamespacedSlice { return app.migrations }

// RegisterMigration registers migrations under the framework's own
// reserved namespace (frameworkDevNamespace), as opposed to a plugin's
// namespace. It is a thin wrapper around registerMigrationForNamespace
// for framework-owned migrations rather than ones contributed by a
// plugin.
func (app *App) RegisterMigration(migrations ...*migration.Migration) error {
	return app.registerMigrationForNamespace(frameworkDevNamespace, migrations...)
}

// RegisterMigrationFromFs reads every *.sql migration file at the root
// of the given embed.FS (see migration.ExtractFromEmbedFs for file
// naming and section-marker requirements), parses them into
// *migration.Migration values, and registers them under the framework's
// own namespace via RegisterMigration.
//
// Returns an error if the embedded filesystem cannot be read or any
// migration file fails to parse.
func (app *App) RegisterMigrationFromFs(files embed.FS) error {
	migrationFiles, err := migration.ExtractFromEmbedFs(files)
	if err != nil {
		return err
	}
	return app.RegisterMigration(migrationFiles...)
}

// registerMigrationForNamespace wraps each of migrations in a
// migration.Namespaced under namespace and appends them to
// app.migrations. It never returns a non-nil error today, but keeps an
// error return so registration can gain validation (e.g. duplicate-name
// checks) later without changing the call sites in app/plugin.go and
// RegisterMigration.
func (app *App) registerMigrationForNamespace(namespace string, migrations ...*migration.Migration) error {
	namespacedMigrations := make([]*migration.Namespaced, len(migrations))
	for i, m := range migrations {
		namespacedMigrations[i] = migration.NewNamespaced(namespace, m)
	}
	app.migrations = append(app.migrations, namespacedMigrations...)
	return nil
}

// migrationsTable is the name of the bookkeeping table the framework
// uses to record which (namespace, migration name) pairs have already
// been applied. It is created lazily on first use.
const migrationsTable = "hypercube_migrations"

// ensureMigrationsTable creates the tracking table if it does not
// already exist. Safe to call repeatedly.
func (app *App) ensureMigrationsTable() error {
	_, err := app.database.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			namespace TEXT NOT NULL,
			name      TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (namespace, name)
		)`, migrationsTable))
	return err
}

// isApplied reports whether the migration (namespace, name) has already
// been recorded as applied.
func (app *App) isApplied(namespace, name string) (bool, error) {
	dbDriver := app.readDbDriver()
	query := fmt.Sprintf(
		`SELECT EXISTS(SELECT 1 FROM %s WHERE namespace = %s AND name = %s)`,
		migrationsTable, dbDriver.placeholder(1), dbDriver.placeholder(2),
	)
	var exists bool
	if err := app.database.QueryRow(query, namespace, name).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// markApplied records that (namespace, name) has been applied.
func (app *App) markApplied(namespace, name string) error {
	dbDriver := app.readDbDriver()
	query := fmt.Sprintf(
		`INSERT INTO %s (namespace, name) VALUES (%s, %s)`,
		migrationsTable, dbDriver.placeholder(1), dbDriver.placeholder(2),
	)
	_, err := app.database.Exec(query, namespace, name)
	return err
}

// markReverted removes the applied record for (namespace, name).
func (app *App) markReverted(namespace, name string) error {
	dbDriver := app.readDbDriver()
	query := fmt.Sprintf(
		`DELETE FROM %s WHERE namespace = %s AND name = %s`,
		migrationsTable, dbDriver.placeholder(1), dbDriver.placeholder(2),
	)
	_, err := app.database.Exec(query, namespace, name)
	return err
}

// runStatements executes each SQL statement in statements in order, inside
// the given transaction.
func runStatements(tx *sql.Tx, statements []string) error {
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec statement %q: %w", stmt, err)
		}
	}
	return nil
}

// RunMigrationsUpTo applies, in ascending Name order, every not-yet-applied
// migration registered under namespace up to and including the migration
// named target. Already-applied migrations are skipped. Each migration's
// Up statements run inside its own transaction, and is recorded as
// applied only if that transaction commits successfully.
//
// Returns an error if namespace has no migration named target, or if
// applying any migration along the way fails — in which case migrations
// before the failure remain applied and the caller should inspect the
// error to decide whether to retry or roll back manually.
func (app *App) RunMigrationsUpTo(namespace, target string) error {
	if err := app.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	ordered := app.migrations.GetNamespace(namespace) // sorted by Name
	targetIdx := indexOfMigration(ordered, target)
	if targetIdx == -1 {
		return fmt.Errorf("migration %q not found in namespace %q", target, namespace)
	}

	for _, m := range ordered[:targetIdx+1] {
		applied, err := app.isApplied(namespace, m.Name)
		if err != nil {
			return fmt.Errorf("check applied state of %q: %w", m.Name, err)
		}
		if applied {
			continue
		}

		tx, err := app.database.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %q: %w", m.Name, err)
		}
		if err := runStatements(tx, m.Up); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %q/%q: %w", namespace, m.Name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %q/%q: %w", namespace, m.Name, err)
		}
		if err := app.markApplied(namespace, m.Name); err != nil {
			return fmt.Errorf("record applied %q/%q: %w", namespace, m.Name, err)
		}
	}
	return nil
}

// RunMigrationsDownTo reverts, in descending Name order, every applied
// migration registered under namespace that comes *after* target, so
// that target becomes the most-recently-applied migration in namespace.
// Migrations at or before target, and migrations that were never
// applied, are left untouched.
//
// Pass an empty target to revert every applied migration in namespace.
//
// Returns an error if namespace has a non-empty target that doesn't
// name a registered migration, or if reverting any migration fails —
// in which case migrations before the failure remain reverted and the
// caller should inspect the error before retrying.
func (app *App) RunMigrationsDownTo(namespace, target string) error {
	if err := app.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	ordered := app.migrations.GetNamespace(namespace) // sorted by Name ascending

	startIdx := 0 // empty target means revert everything
	if target != "" {
		targetIdx := indexOfMigration(ordered, target)
		if targetIdx == -1 {
			return fmt.Errorf(
				"migration %q not found in namespace %q",
				target,
				namespace,
			)
		}
		startIdx = targetIdx + 1
	}

	// Revert from the newest migration back down to (but not including) target.
	for i := len(ordered) - 1; i >= startIdx; i-- {
		m := ordered[i]
		applied, err := app.isApplied(namespace, m.Name)
		if err != nil {
			return fmt.Errorf("check applied state of %q: %w", m.Name, err)
		}
		if !applied {
			continue
		}

		tx, err := app.database.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %q: %w", m.Name, err)
		}
		if err := runStatements(tx, m.Down); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("revert %q/%q: %w", namespace, m.Name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit revert %q/%q: %w", namespace, m.Name, err)
		}
		if err := app.markReverted(namespace, m.Name); err != nil {
			return fmt.Errorf("record reverted %q/%q: %w", namespace, m.Name, err)
		}
	}
	return nil
}

// indexOfMigration returns the index of the migration named name within
// ordered, or -1 if not present.
func indexOfMigration(ordered []*migration.Migration, name string) int {
	for i, m := range ordered {
		if m.Name == name {
			return i
		}
	}
	return -1
}

// MigrateAllUp brings every namespace's migrations fully up to date
// (applies every not-yet-applied migration, in Name order).
//
// Order matters here: plugin namespaces are migrated first, in the
// exact order app.plugins is already sorted into by initPlugins (i.e.
// plugin dependency order — a plugin's migrations run only after every
// plugin it depends on has had its own migrations applied). Any
// remaining namespaces present in app.migrations that do not belong to
// a currently-registered plugin (for example frameworkDevNamespace, or
// leftover namespaces from a plugin that was later removed) are
// migrated afterward, in a deterministic (sorted) order.
//
// MigrateAllUp must be called after Setup() so that app.plugins is
// already dependency-ordered; it returns an error if Setup() has not
// run yet.
//
// Returns the first error encountered from RunMigrationsUpTo, leaving
// namespaces migrated so far in their newly-applied state.
func (app *App) MigrateAllUp() error {
	if !app.didSetup {
		return fmt.Errorf("cannot migrate before setting up the framework; did you forget to call Setup()")
	}

	migrated := make(map[string]struct{}, len(app.plugins))

	// Plugins first, in their already-resolved dependency order.
	for _, p := range app.plugins {
		namespace := p.Name()
		if err := app.migrateNamespaceUpToLatest(namespace); err != nil {
			return err
		}
		migrated[namespace] = struct{}{}
	}

	// Then any remaining namespaces (e.g. frameworkDevNamespace) that
	// aren't tied to a currently-registered plugin, in sorted order for
	// determinism.
	for _, namespace := range app.migrations.Namespaces() {
		if _, done := migrated[namespace]; done {
			continue
		}
		if err := app.migrateNamespaceUpToLatest(namespace); err != nil {
			return err
		}
	}
	return nil
}

// migrateNamespaceUpToLatest applies every not-yet-applied migration in
// namespace, up to the newest one registered. It is a no-op if
// namespace has no registered migrations.
func (app *App) migrateNamespaceUpToLatest(namespace string) error {
	ordered := app.migrations.GetNamespace(namespace)
	if len(ordered) == 0 {
		return nil
	}
	latest := ordered[len(ordered)-1].Name
	return app.RunMigrationsUpTo(namespace, latest)
}

// MigrationStatus describes a single registered migration's position
// within its namespace and whether it has been applied.
type MigrationStatus struct {
	Namespace string
	Name      string
	Applied   bool
	AppliedAt *time.Time // nil if Applied is false
}

// NamespaceMigrationState summarizes the migration state of a single
// namespace: every registered migration (in ascending Name order) along
// with its applied status, plus the name of the most-recently-applied
// migration (the namespace's "current version").
type NamespaceMigrationState struct {
	Namespace string
	Current   string // name of the latest applied migration, "" if none applied
	Pending   int    // count of registered migrations not yet applied
	Statuses  []MigrationStatus
}

// MigrationState returns the current migration state of every namespace
// that has at least one registered migration, ordered the same way
// MigrateAllUp orders namespaces: plugin namespaces first (in
// app.plugins' dependency order), then any remaining namespaces (e.g.
// frameworkDevNamespace) in sorted order.
//
// MigrationState reads the tracking table directly rather than calling
// isApplied per migration, so it reflects the database's current state
// in a single query per namespace even for namespaces with many
// migrations.
//
// Returns an error if the migrations table doesn't exist yet (call
// ensureMigrationsTable first, e.g. by running any migration) or the
// query fails.
func (app *App) MigrationState() ([]*NamespaceMigrationState, error) {
	if !app.didSetup {
		return nil, fmt.Errorf("cannot pull the migration state before setting up the framework; did you forget to call Setup()")
	}

	if err := app.ensureMigrationsTable(); err != nil {
		return nil, fmt.Errorf("ensure migrations table: %w", err)
	}

	orderedNamespaces := app.orderedMigrationNamespaces()
	result := make([]*NamespaceMigrationState, 0, len(orderedNamespaces))

	for _, namespace := range orderedNamespaces {
		state, err := app.namespaceMigrationState(namespace)
		if err != nil {
			return nil, fmt.Errorf("get migration state for namespace %q: %w", namespace, err)
		}
		result = append(result, state)
	}
	return result, nil
}

// namespaceMigrationState builds the MigrationState for a single
// namespace by fetching all applied (name -> applied_at) pairs in one
// query, then walking the namespace's registered migrations in order.
func (app *App) namespaceMigrationState(namespace string) (*NamespaceMigrationState, error) {
	ordered := app.migrations.GetNamespace(namespace) // sorted by Name ascending

	applied, err := app.appliedTimes(namespace)
	if err != nil {
		return nil, err
	}

	state := &NamespaceMigrationState{
		Namespace: namespace,
		Statuses:  make([]MigrationStatus, 0, len(ordered)),
	}

	for _, m := range ordered {
		appliedAt, ok := applied[m.Name]
		status := MigrationStatus{
			Namespace: namespace,
			Name:      m.Name,
			Applied:   ok,
		}
		if ok {
			t := appliedAt
			status.AppliedAt = &t
			state.Current = m.Name // ordered ascending, so the last match wins
		} else {
			state.Pending++
		}
		state.Statuses = append(state.Statuses, status)
	}
	return state, nil
}

// appliedTimes returns a map of migration name -> applied_at for every
// migration currently recorded as applied under namespace.
func (app *App) appliedTimes(namespace string) (map[string]time.Time, error) {
	dbDriver := app.readDbDriver()
	query := fmt.Sprintf(
		`SELECT name, applied_at FROM %s WHERE namespace = %s`,
		migrationsTable, dbDriver.placeholder(1),
	)
	rows, err := app.database.Query(query, namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]time.Time)
	for rows.Next() {
		var name string
		var appliedAt time.Time
		if err := rows.Scan(&name, &appliedAt); err != nil {
			return nil, err
		}
		result[name] = appliedAt
	}
	return result, rows.Err()
}

// orderedMigrationNamespaces returns the namespaces that have at least
// one registered migration, ordered the same way MigrateAllUp applies
// them: plugin namespaces first (in app.plugins' dependency order),
// then any remaining namespaces in sorted order.
func (app *App) orderedMigrationNamespaces() []string {
	registered := make(map[string]struct{})
	for _, namespace := range app.migrations.Namespaces() {
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

	for _, namespace := range app.migrations.Namespaces() { // already sorted
		if _, done := seen[namespace]; done {
			continue
		}
		ordered = append(ordered, namespace)
	}
	return ordered
}
