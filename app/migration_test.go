package app

import (
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-hypercube/go-hypercube/migration"
	"github.com/go-hypercube/go-hypercube/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// ---- registration ----

func TestRegisterMigration(t *testing.T) {
	app := &App{}

	m1 := &migration.Migration{Name: "0001_a", Up: []string{"up1"}, Down: []string{"down1"}}
	m2 := &migration.Migration{Name: "0002_b", Up: []string{"up2"}, Down: []string{"down2"}}

	err := app.RegisterMigration(m1, m2)
	require.NoError(t, err)

	got := app.Migrations()
	require.Len(t, got, 2)
	assert.Equal(t, frameworkDevNamespace, got[0].Namespace)
	assert.Equal(t, "0001_a", got[0].Name)
	assert.Equal(t, frameworkDevNamespace, got[1].Namespace)
	assert.Equal(t, "0002_b", got[1].Name)
}

func TestRegisterMigrationForNamespace_Appends(t *testing.T) {
	app := &App{}

	require.NoError(t, app.registerMigrationForNamespace("pluginA",
		&migration.Migration{Name: "0001"}))
	require.NoError(t, app.registerMigrationForNamespace("pluginB",
		&migration.Migration{Name: "0001"}))

	got := app.Migrations()
	require.Len(t, got, 2)
	assert.Equal(t, "pluginA", got[0].Namespace)
	assert.Equal(t, "pluginB", got[1].Namespace)
}

// ---- indexOfMigration ----

func TestIndexOfMigration(t *testing.T) {
	ordered := []*migration.Migration{
		{Name: "0001"},
		{Name: "0002"},
		{Name: "0003"},
	}

	assert.Equal(t, 0, indexOfMigration(ordered, "0001"))
	assert.Equal(t, 2, indexOfMigration(ordered, "0003"))
	assert.Equal(t, -1, indexOfMigration(ordered, "does-not-exist"))
	assert.Equal(t, -1, indexOfMigration(nil, "0001"))
}

// ---- ensureMigrationsTable ----

func TestEnsureMigrationsTable(t *testing.T) {
	app, mock := newMockApp(t, "")

	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := app.ensureMigrationsTable()
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureMigrationsTable_Error(t *testing.T) {
	app, mock := newMockApp(t, "")

	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
		WillReturnError(assert.AnError)

	err := app.ensureMigrationsTable()
	assert.ErrorIs(t, err, assert.AnError)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- isApplied (dialect-sensitive) ----

func TestIsApplied(t *testing.T) {
	t.Run("true, unknown dialect uses ? placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM hypercube_migrations WHERE namespace = \? AND name = \?\)`).
			WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		applied, err := app.isApplied("auth", "0001")
		require.NoError(t, err)
		assert.True(t, applied)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("false", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM hypercube_migrations WHERE namespace = \? AND name = \?\)`).
			WithArgs("auth", "0002").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		applied, err := app.isApplied("auth", "0002")
		require.NoError(t, err)
		assert.False(t, applied)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres dialect uses $1 $2 placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "postgres")
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM hypercube_migrations WHERE namespace = \$1 AND name = \$2\)`).
			WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		applied, err := app.isApplied("auth", "0001")
		require.NoError(t, err)
		assert.True(t, applied)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectQuery(`SELECT EXISTS`).
			WillReturnError(assert.AnError)

		_, err := app.isApplied("auth", "0001")
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- markApplied / markReverted ----
//
// NOTE: unlike isApplied, these two hardcode "?" placeholders regardless
// of dbDriver — this looks like a latent bug (a Postgres app would fail
// here since app.database wouldn't accept "?" positional params). The
// tests below pin down current behavior; if that gets fixed to use
// app.readDbDriver().placeholder(...), these expectations must change to
// match ($1, $2) under the "postgres" subtest.

// ---- markApplied / markReverted ----

func TestMarkApplied(t *testing.T) {
	t.Run("unknown dialect uses ? placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectExec(`INSERT INTO hypercube_migrations \(namespace, name\) VALUES \(\?, \?\)`).
			WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.markApplied("auth", "0001")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres dialect uses $1 $2 placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "postgres")

		mock.ExpectExec(`INSERT INTO hypercube_migrations \(namespace, name\) VALUES \(\$1, \$2\)`).
			WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.markApplied("auth", "0001")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("mysql dialect uses ? placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "mysql")

		mock.ExpectExec(`INSERT INTO hypercube_migrations \(namespace, name\) VALUES \(\?, \?\)`).
			WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.markApplied("auth", "0001")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestMarkApplied_Error(t *testing.T) {
	app, mock := newMockApp(t, "")

	mock.ExpectExec(`INSERT INTO hypercube_migrations`).
		WillReturnError(assert.AnError)

	err := app.markApplied("auth", "0001")
	assert.ErrorIs(t, err, assert.AnError)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkReverted(t *testing.T) {
	t.Run("unknown dialect uses ? placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectExec(`DELETE FROM hypercube_migrations WHERE namespace = \? AND name = \?`).
			WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := app.markReverted("auth", "0001")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres dialect uses $1 $2 placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "postgres")

		mock.ExpectExec(`DELETE FROM hypercube_migrations WHERE namespace = \$1 AND name = \$2`).
			WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := app.markReverted("auth", "0001")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestMarkReverted_Error(t *testing.T) {
	app, mock := newMockApp(t, "")

	mock.ExpectExec(`DELETE FROM hypercube_migrations`).
		WillReturnError(assert.AnError)

	err := app.markReverted("auth", "0001")
	assert.ErrorIs(t, err, assert.AnError)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- runStatements ----

func TestRunStatements(t *testing.T) {
	t.Run("all succeed, run in order", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectBegin()
		mock.ExpectExec(`create table a`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`create table b`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		tx, err := app.database.Begin()
		require.NoError(t, err)

		err = runStatements(tx, []string{"create table a", "create table b"})
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stops at first failing statement", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectBegin()
		mock.ExpectExec(`create table a`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`bad sql`).WillReturnError(assert.AnError)
		mock.ExpectRollback()

		tx, err := app.database.Begin()
		require.NoError(t, err)

		err = runStatements(tx, []string{"create table a", "bad sql", "never runs"})
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- RunMigrationsUpTo ----

func TestRunMigrationsUpTo(t *testing.T) {
	t.Run("applies not-yet-applied migrations up to and including target", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001", Up: []string{"create table a"}},
			&migration.Migration{Name: "0002", Up: []string{"create table b"}},
			&migration.Migration{Name: "0003", Up: []string{"create table c"}}, // beyond target, must not run
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		// 0001: not applied -> runs
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectBegin()
		mock.ExpectExec(`create table a`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`INSERT INTO hypercube_migrations`).WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		// 0002: already applied -> skipped entirely
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0002").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		// 0003 must never be queried/run since target is "0002"

		err := app.RunMigrationsUpTo("auth", "0002")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("target not found in namespace", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001"}))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := app.RunMigrationsUpTo("auth", "does-not-exist")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `migration "does-not-exist" not found in namespace "auth"`)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stops and returns error when an Up statement fails, later migrations not attempted", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001", Up: []string{"bad sql"}},
			&migration.Migration{Name: "0002", Up: []string{"create table b"}},
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectBegin()
		mock.ExpectExec(`bad sql`).WillReturnError(assert.AnError)
		mock.ExpectRollback()

		err := app.RunMigrationsUpTo("auth", "0002")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `apply "auth"/"0001"`)
		require.NoError(t, mock.ExpectationsWereMet()) // 0002 was never touched
	})

	t.Run("ensureMigrationsTable failure short-circuits", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnError(assert.AnError)

		err := app.RunMigrationsUpTo("auth", "0001")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ensure migrations table")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- RunMigrationsDownTo ----

func TestRunMigrationsDownTo(t *testing.T) {
	t.Run("reverts newest-first down to but not including target", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001", Down: []string{"drop table a"}}, // at/before target: untouched
			&migration.Migration{Name: "0002", Down: []string{"drop table b"}}, // reverted
			&migration.Migration{Name: "0003", Down: []string{"drop table c"}}, // reverted first
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		// 0003 reverted first (descending order)
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0003").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`drop table c`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`DELETE FROM hypercube_migrations`).WithArgs("auth", "0003").
			WillReturnResult(sqlmock.NewResult(0, 1))

		// 0002 reverted next
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0002").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`drop table b`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`DELETE FROM hypercube_migrations`).WithArgs("auth", "0002").
			WillReturnResult(sqlmock.NewResult(0, 1))

		// 0001 is the target itself -> never touched

		err := app.RunMigrationsDownTo("auth", "0001")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty target reverts everything applied", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{
				Name: "0001",
				Down: []string{"drop table a"},
			},
			&migration.Migration{
				Name: "0002",
				Down: []string{"drop table b"},
			},
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		// Latest migration is reverted first.
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM hypercube_migrations WHERE namespace = \? AND name = \?\)`).
			WithArgs("auth", "0002").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		mock.ExpectBegin()
		mock.ExpectExec(`drop table b`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		mock.ExpectExec(`DELETE FROM hypercube_migrations`).
			WithArgs("auth", "0002").
			WillReturnResult(sqlmock.NewResult(0, 1))

		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM hypercube_migrations WHERE namespace = \? AND name = \?\)`).
			WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		mock.ExpectBegin()
		mock.ExpectExec(`drop table a`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		mock.ExpectExec(`DELETE FROM hypercube_migrations`).
			WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := app.RunMigrationsDownTo("auth", "")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("never-applied migrations are skipped without reverting", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001", Down: []string{"drop table a"}},
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		// no Begin/Exec/Commit/DELETE expected

		err := app.RunMigrationsDownTo("auth", "")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("target not found in namespace", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001"}))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := app.RunMigrationsDownTo("auth", "does-not-exist")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `migration "does-not-exist" not found in namespace "auth"`)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- migrateNamespaceUpToLatest ----

func TestMigrateNamespaceUpToLatest(t *testing.T) {
	t.Run("no-op when namespace has no migrations", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		// no expectations set at all — function must return before touching the DB
		err := app.migrateNamespaceUpToLatest("empty-namespace")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("migrates up to the newest registered migration", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001", Up: []string{"create table a"}},
			&migration.Migration{Name: "0002", Up: []string{"create table b"}},
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0002").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		err := app.migrateNamespaceUpToLatest("auth")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- MigrateAllUp ----

// fakePlugin is a minimal plugin.Plugin for exercising plugin-first
// ordering in MigrateAllUp. Register/Boot are not exercised here since
// MigrateAllUp only reads app.plugins (assumed already populated and
// dependency-ordered by Setup/initPlugins).
type fakePlugin struct {
	name string
}

func (p *fakePlugin) Name() string           { return p.name }
func (p *fakePlugin) Dependencies() []string { return nil }
func (p *fakePlugin) Register(_ *plugin.App) (*plugin.Registration, error) {
	return &plugin.Registration{}, nil
}
func (p *fakePlugin) Boot(_ *plugin.App) error { return nil }

func TestMigrateAllUp(t *testing.T) {
	t.Run("errors if Setup has not been called", func(t *testing.T) {
		app := &App{didSetup: false}
		err := app.MigrateAllUp()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "did you forget to call Setup()")
	})

	t.Run("migrates plugin namespaces first in plugin order, then remaining namespaces", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "pluginB"},
			&fakePlugin{name: "pluginA"}, // deliberately not alpha order: this order must be honored, not re-sorted
		}

		require.NoError(t, app.registerMigrationForNamespace("pluginB",
			&migration.Migration{Name: "0001", Up: []string{"create table pb"}}))
		require.NoError(t, app.registerMigrationForNamespace("pluginA",
			&migration.Migration{Name: "0001", Up: []string{"create table pa"}}))
		require.NoError(t, app.registerMigrationForNamespace(frameworkDevNamespace,
			&migration.Migration{Name: "0001", Up: []string{"create table dev"}}))

		// pluginB migrated first
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("pluginB", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectBegin()
		mock.ExpectExec(`create table pb`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`INSERT INTO hypercube_migrations`).WithArgs("pluginB", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		// pluginA migrated second
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("pluginA", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectBegin()
		mock.ExpectExec(`create table pa`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`INSERT INTO hypercube_migrations`).WithArgs("pluginA", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		// frameworkDevNamespace ("owner") migrated last, since it's not a
		// currently-registered plugin
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs(frameworkDevNamespace, "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectBegin()
		mock.ExpectExec(`create table dev`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`INSERT INTO hypercube_migrations`).WithArgs(frameworkDevNamespace, "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.MigrateAllUp()
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stops at first namespace that fails", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true
		app.plugins = []plugin.Plugin{&fakePlugin{name: "pluginA"}}

		require.NoError(t, app.registerMigrationForNamespace("pluginA",
			&migration.Migration{Name: "0001", Up: []string{"bad sql"}}))
		require.NoError(t, app.registerMigrationForNamespace(frameworkDevNamespace,
			&migration.Migration{Name: "0001", Up: []string{"create table dev"}}))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("pluginA", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectBegin()
		mock.ExpectExec(`bad sql`).WillReturnError(assert.AnError)
		mock.ExpectRollback()

		err := app.MigrateAllUp()
		require.Error(t, err)
		// frameworkDevNamespace must never be reached
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- orderedMigrationNamespaces ----

func TestOrderedMigrationNamespaces(t *testing.T) {
	t.Run("plugins first in dependency order, then remaining sorted", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "pluginB"},
			&fakePlugin{name: "pluginA"}, // deliberately not alpha: order must be preserved
		}

		require.NoError(t, app.registerMigrationForNamespace("pluginB", &migration.Migration{Name: "0001"}))
		require.NoError(t, app.registerMigrationForNamespace("pluginA", &migration.Migration{Name: "0001"}))
		require.NoError(t, app.registerMigrationForNamespace(frameworkDevNamespace, &migration.Migration{Name: "0001"}))
		require.NoError(t, app.registerMigrationForNamespace("zzz-no-plugin", &migration.Migration{Name: "0001"}))

		got := app.orderedMigrationNamespaces()
		// pluginB, pluginA (plugin order preserved) then sorted remainder:
		// frameworkDevNamespace = "owner", "zzz-no-plugin"
		assert.Equal(t, []string{"pluginB", "pluginA", frameworkDevNamespace, "zzz-no-plugin"}, got)
	})

	t.Run("plugin with no registered migrations is skipped", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "pluginA"}, // has no migrations registered
		}
		require.NoError(t, app.registerMigrationForNamespace(frameworkDevNamespace, &migration.Migration{Name: "0001"}))

		got := app.orderedMigrationNamespaces()
		assert.Equal(t, []string{frameworkDevNamespace}, got)
	})

	t.Run("no plugins, only sorted remainder", func(t *testing.T) {
		app := &App{}
		require.NoError(t, app.registerMigrationForNamespace("billing", &migration.Migration{Name: "0001"}))
		require.NoError(t, app.registerMigrationForNamespace("auth", &migration.Migration{Name: "0001"}))

		got := app.orderedMigrationNamespaces()
		assert.Equal(t, []string{"auth", "billing"}, got)
	})

	t.Run("no migrations registered at all", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{&fakePlugin{name: "pluginA"}}

		got := app.orderedMigrationNamespaces()
		assert.Empty(t, got)
	})
}

// ---- appliedTimes ----

func TestAppliedTimes(t *testing.T) {
	t.Run("returns name -> applied_at map, unknown dialect uses ? placeholder", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "applied_at"}).
				AddRow("0001", t1).
				AddRow("0002", t2))

		got, err := app.appliedTimes("auth")
		require.NoError(t, err)
		assert.Equal(t, map[string]time.Time{"0001": t1, "0002": t2}, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres dialect uses $1 placeholder", func(t *testing.T) {
		app, mock := newMockApp(t, "postgres")

		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations WHERE namespace = \$1`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "applied_at"}))

		got, err := app.appliedTimes("auth")
		require.NoError(t, err)
		assert.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty result set", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "applied_at"}))

		got, err := app.appliedTimes("auth")
		require.NoError(t, err)
		assert.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations`).
			WillReturnError(assert.AnError)

		_, err := app.appliedTimes("auth")
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("row scan error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "applied_at"}).
				AddRow("0001", "not-a-time")) // wrong type triggers Scan error

		_, err := app.appliedTimes("auth")
		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- namespaceMigrationState ----

func TestNamespaceMigrationState(t *testing.T) {
	t.Run("mix of applied and pending, Current is highest applied Name", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001"},
			&migration.Migration{Name: "0002"},
			&migration.Migration{Name: "0003"},
		))

		t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "applied_at"}).
				AddRow("0001", t1).
				AddRow("0002", t2))
			// 0003 not applied

		state, err := app.namespaceMigrationState("auth")
		require.NoError(t, err)

		assert.Equal(t, "auth", state.Namespace)
		assert.Equal(t, "0002", state.Current)
		assert.Equal(t, 1, state.Pending)
		require.Len(t, state.Statuses, 3)

		assert.Equal(t, MigrationStatus{Namespace: "auth", Name: "0001", Applied: true, AppliedAt: &t1}, state.Statuses[0])
		assert.Equal(t, MigrationStatus{Namespace: "auth", Name: "0002", Applied: true, AppliedAt: &t2}, state.Statuses[1])
		assert.Equal(t, MigrationStatus{Namespace: "auth", Name: "0003", Applied: false, AppliedAt: nil}, state.Statuses[2])

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("nothing applied", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001"},
		))

		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "applied_at"}))

		state, err := app.namespaceMigrationState("auth")
		require.NoError(t, err)
		assert.Equal(t, "", state.Current)
		assert.Equal(t, 1, state.Pending)
		assert.False(t, state.Statuses[0].Applied)
		assert.Nil(t, state.Statuses[0].AppliedAt)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("everything applied", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001"},
			&migration.Migration{Name: "0002"},
		))

		t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "applied_at"}).
				AddRow("0001", t1).
				AddRow("0002", t2))

		state, err := app.namespaceMigrationState("auth")
		require.NoError(t, err)
		assert.Equal(t, "0002", state.Current)
		assert.Equal(t, 0, state.Pending)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("namespace with no registered migrations", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations WHERE namespace = \?`).
			WithArgs("empty-namespace").
			WillReturnRows(sqlmock.NewRows([]string{"name", "applied_at"}))

		state, err := app.namespaceMigrationState("empty-namespace")
		require.NoError(t, err)
		assert.Equal(t, "empty-namespace", state.Namespace)
		assert.Equal(t, "", state.Current)
		assert.Equal(t, 0, state.Pending)
		assert.Empty(t, state.Statuses)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("appliedTimes error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth", &migration.Migration{Name: "0001"}))

		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations`).
			WillReturnError(assert.AnError)

		_, err := app.namespaceMigrationState("auth")
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- MigrationState ----

func TestMigrationState(t *testing.T) {
	t.Run("ensureMigrationsTable failure short-circuits", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnError(assert.AnError)

		_, err := app.MigrationState()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ensure migrations table")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no namespaces registered returns empty slice, no error", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		got, err := app.MigrationState()
		require.NoError(t, err)
		assert.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("aggregates state across multiple namespaces in plugin-first order", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true
		app.plugins = []plugin.Plugin{&fakePlugin{name: "pluginA"}}

		require.NoError(t, app.registerMigrationForNamespace("pluginA",
			&migration.Migration{Name: "0001"},
			&migration.Migration{Name: "0002"},
		))
		require.NoError(t, app.registerMigrationForNamespace(frameworkDevNamespace,
			&migration.Migration{Name: "0001"},
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		// pluginA queried first
		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations WHERE namespace = \?`).
			WithArgs("pluginA").
			WillReturnRows(sqlmock.NewRows([]string{"name", "applied_at"}).
				AddRow("0001", t1))

		// frameworkDevNamespace queried second
		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations WHERE namespace = \?`).
			WithArgs(frameworkDevNamespace).
			WillReturnRows(sqlmock.NewRows([]string{"name", "applied_at"}))

		got, err := app.MigrationState()
		require.NoError(t, err)
		require.Len(t, got, 2)

		assert.Equal(t, "pluginA", got[0].Namespace)
		assert.Equal(t, "0001", got[0].Current)
		assert.Equal(t, 1, got[0].Pending)

		assert.Equal(t, frameworkDevNamespace, got[1].Namespace)
		assert.Equal(t, "", got[1].Current)
		assert.Equal(t, 1, got[1].Pending)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("per-namespace query error is wrapped with namespace context", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true
		require.NoError(t, app.registerMigrationForNamespace("auth", &migration.Migration{Name: "0001"}))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT name, applied_at FROM hypercube_migrations`).
			WillReturnError(assert.AnError)

		_, err := app.MigrationState()
		require.Error(t, err)
		assert.Contains(t, err.Error(), `get migration state for namespace "auth"`)
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
