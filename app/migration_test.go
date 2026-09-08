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

// ---- registration ----

func TestRegisterMigration(t *testing.T) {
	app := &App{}

	m1 := &migration.Migration{Name: "0001_a", Up: []string{"up1"}, Down: []string{"down1"}}
	m2 := &migration.Migration{Name: "0002_b", Up: []string{"up2"}, Down: []string{"down2"}}

	err := app.RegisterMigration(m1, m2)
	require.NoError(t, err)

	got := app.Migrations()
	require.Len(t, got, 2)
	assert.Equal(t, hostAppNamespace, got[0].Namespace)
	assert.Equal(t, "0001_a", got[0].Name)
	assert.Equal(t, hostAppNamespace, got[1].Namespace)
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
		require.NoError(t, app.registerMigrationForNamespace(hostAppNamespace,
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
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs(hostAppNamespace, "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectBegin()
		mock.ExpectExec(`create table dev`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`INSERT INTO hypercube_migrations`).WithArgs(hostAppNamespace, "0001").
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
		require.NoError(t, app.registerMigrationForNamespace(hostAppNamespace,
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
		require.NoError(t, app.registerMigrationForNamespace(hostAppNamespace, &migration.Migration{Name: "0001"}))
		require.NoError(t, app.registerMigrationForNamespace("zzz-no-plugin", &migration.Migration{Name: "0001"}))

		got := app.orderedMigrationNamespaces()
		// pluginB, pluginA (plugin order preserved) then sorted remainder:
		// frameworkDevNamespace = "owner", "zzz-no-plugin"
		assert.Equal(t, []string{"pluginB", "pluginA", hostAppNamespace, "zzz-no-plugin"}, got)
	})

	t.Run("plugin with no registered migrations is skipped", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "pluginA"}, // has no migrations registered
		}
		require.NoError(t, app.registerMigrationForNamespace(hostAppNamespace, &migration.Migration{Name: "0001"}))

		got := app.orderedMigrationNamespaces()
		assert.Equal(t, []string{hostAppNamespace}, got)
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

		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \? ORDER By applied_at DESC`).
			WithArgs("auth").
			WillReturnRows(
				sqlmock.NewRows([]string{"namespace", "name", "applied_at"}).
					AddRow("auth", "0001", t1).
					AddRow("auth", "0002", t2),
			)

		got, err := app.appliedMigrations("auth")
		require.NoError(t, err)
		assert.True(t, assert.ObjectsAreEqualValues(
			appliedMigrationEntry{
				namespace: "auth",
				name:      "0001",
				appliedAt: t1,
			},
			*got[0],
		))
		assert.True(t, assert.ObjectsAreEqualValues(
			appliedMigrationEntry{
				namespace: "auth",
				name:      "0002",
				appliedAt: t2,
			},
			*got[1],
		))

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres dialect uses $1 placeholder", func(t *testing.T) {
		app, mock := newMockApp(t, "postgres")

		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \$1 ORDER By applied_at DESC`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}))

		got, err := app.appliedMigrations("auth")
		require.NoError(t, err)
		assert.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty result set", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \? ORDER By applied_at DESC`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}))

		got, err := app.appliedMigrations("auth")
		require.NoError(t, err)
		assert.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \? ORDER By applied_at DESC`).
			WillReturnError(assert.AnError)

		_, err := app.appliedMigrations("auth")
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("row scan error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}).
				AddRow("auth", "0001", "not-a-time")) // wrong type triggers Scan error

		_, err := app.appliedMigrations("auth")
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

		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \? ORDER By applied_at DESC`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}).
				AddRow("auth", "0001", t1).
				AddRow("auth", "0002", t2))
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

		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \? ORDER By applied_at DESC`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}))

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

		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \? ORDER By applied_at DESC`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}).
				AddRow("auth", "0001", t1).
				AddRow("auth", "0002", t2))

		state, err := app.namespaceMigrationState("auth")
		require.NoError(t, err)
		assert.Equal(t, "0002", state.Current)
		assert.Equal(t, 0, state.Pending)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("namespace with no registered migrations", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \? ORDER By applied_at DESC`).
			WithArgs("empty-namespace").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}))

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

		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \? ORDER By applied_at DESC`).
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
		require.NoError(t, app.registerMigrationForNamespace(hostAppNamespace,
			&migration.Migration{Name: "0001"},
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		// pluginA queried first
		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \? ORDER By applied_at DESC`).
			WithArgs("pluginA").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}).
				AddRow("pluginA", "0001", t1))

		// frameworkDevNamespace queried second
		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations WHERE namespace = \? ORDER By applied_at DESC`).
			WithArgs(hostAppNamespace).
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}))

		got, err := app.MigrationState()
		require.NoError(t, err)
		require.Len(t, got, 2)

		assert.Equal(t, "pluginA", got[0].Namespace)
		assert.Equal(t, "0001", got[0].Current)
		assert.Equal(t, 1, got[0].Pending)

		assert.Equal(t, hostAppNamespace, got[1].Namespace)
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
		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations`).
			WillReturnError(assert.AnError)

		_, err := app.MigrationState()
		require.Error(t, err)
		assert.Contains(t, err.Error(), `get migration state for namespace "auth"`)
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- registerMigrationForNamespace duplicate rejection ----

func TestRegisterMigrationForNamespace_RejectsDuplicateName(t *testing.T) {
	t.Run("second registration of the same (namespace, name) is rejected", func(t *testing.T) {
		app := &App{}
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001_init"}))

		err := app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001_init"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `migration "0001_init" is already registered in namespace "auth"`)
	})

	t.Run("existing registration is left untouched after a rejected duplicate", func(t *testing.T) {
		app := &App{}
		original := &migration.Migration{Name: "0001_init", Up: []string{"create table a"}}
		require.NoError(t, app.registerMigrationForNamespace("auth", original))

		err := app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001_init", Up: []string{"create table b"}})
		require.Error(t, err)

		got := app.migrations.GetNamespace("auth")
		require.Len(t, got, 1)
		assert.Same(t, original, got[0], "the original migration must not be replaced")
	})

	t.Run("same name in a different namespace does not collide", func(t *testing.T) {
		app := &App{}
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001_init"}))

		err := app.registerMigrationForNamespace("billing",
			&migration.Migration{Name: "0001_init"})
		require.NoError(t, err)

		assert.Len(t, app.migrations.GetNamespace("auth"), 1)
		assert.Len(t, app.migrations.GetNamespace("billing"), 1)
	})

	t.Run("batch registration is atomic: a collision partway through registers none of the batch", func(t *testing.T) {
		app := &App{}
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0002_existing"}))

		err := app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001_new"},
			&migration.Migration{Name: "0002_existing"}, // collides
			&migration.Migration{Name: "0003_new"},
		)
		require.Error(t, err)

		got := app.migrations.GetNamespace("auth")
		require.Len(t, got, 1, "none of the new batch should be registered when any one of them collides")
		assert.Equal(t, "0002_existing", got[0].Name)
	})

	t.Run("collision within the same batch (no prior registration) is also rejected", func(t *testing.T) {
		app := &App{}

		err := app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001_dup"},
			&migration.Migration{Name: "0001_dup"},
		)
		require.Error(t, err)
	})

	t.Run("collision within the same batch is rejected with a distinct message", func(t *testing.T) {
		app := &App{}

		err := app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001_dup"},
			&migration.Migration{Name: "0001_dup"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `migration "0001_dup" appears more than once in this registration call for namespace "auth"`)
	})

	t.Run("in-batch collision registers none of the batch", func(t *testing.T) {
		app := &App{}

		err := app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001_first"},
			&migration.Migration{Name: "0002_dup"},
			&migration.Migration{Name: "0002_dup"},
		)
		require.Error(t, err)
		assert.Empty(t, app.migrations.GetNamespace("auth"), "no migration from the batch should be registered, including the ones before the collision")
	})

	t.Run("in-batch collision is detected even when the duplicate appears before the first occurrence in iteration order doesn't matter", func(t *testing.T) {
		app := &App{}

		err := app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001_a"},
			&migration.Migration{Name: "0001_a"},
			&migration.Migration{Name: "0002_b"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"0001_a" appears more than once`)
	})

	t.Run("RegisterMigration surfaces the same rejection", func(t *testing.T) {
		app := &App{}
		require.NoError(t, app.RegisterMigration(&migration.Migration{Name: "0001"}))

		err := app.RegisterMigration(&migration.Migration{Name: "0001"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})
}

// ---- MigrateAllDown ----

func TestMigrateAllDown(t *testing.T) {
	t.Run("errors if Setup has not been called", func(t *testing.T) {
		app := &App{didSetup: false}
		err := app.MigrateAllDown()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "did you forget to call Setup()")
	})

	t.Run("tears down non-plugin namespaces first, then plugins in reverse dependency order", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true
		// dependency order from initPlugins: pluginA before pluginB
		// (pluginB depends on pluginA) -> reverse teardown: pluginB then pluginA
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "pluginA"},
			&fakePlugin{name: "pluginB"},
		}

		require.NoError(t, app.registerMigrationForNamespace("pluginA",
			&migration.Migration{Name: "0001", Down: []string{"drop table pa"}}))
		require.NoError(t, app.registerMigrationForNamespace("pluginB",
			&migration.Migration{Name: "0001", Down: []string{"drop table pb"}}))
		require.NoError(t, app.registerMigrationForNamespace(hostAppNamespace,
			&migration.Migration{Name: "0001", Down: []string{"drop table dev"}}))

		// hostAppNamespace first (only non-plugin namespace)
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs(hostAppNamespace, "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`drop table dev`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`DELETE FROM hypercube_migrations`).WithArgs(hostAppNamespace, "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		// pluginB torn down before pluginA (reverse dependency order)
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("pluginB", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`drop table pb`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`DELETE FROM hypercube_migrations`).WithArgs("pluginB", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("pluginA", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`drop table pa`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`DELETE FROM hypercube_migrations`).WithArgs("pluginA", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := app.MigrateAllDown()
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stops at first namespace that fails", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true
		app.plugins = []plugin.Plugin{&fakePlugin{name: "pluginA"}}

		require.NoError(t, app.registerMigrationForNamespace(hostAppNamespace,
			&migration.Migration{Name: "0001", Down: []string{"bad sql"}}))
		require.NoError(t, app.registerMigrationForNamespace("pluginA",
			&migration.Migration{Name: "0001", Down: []string{"drop table pa"}}))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs(hostAppNamespace, "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`bad sql`).WillReturnError(assert.AnError)
		mock.ExpectRollback()

		err := app.MigrateAllDown()
		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet()) // pluginA never reached
	})
}

// ---- RollbackSteps ----

func TestRollbackSteps(t *testing.T) {
	t.Run("zero or negative steps is a no-op", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.RollbackSteps("auth", 0))
		require.NoError(t, app.RollbackSteps("auth", -1))
		require.NoError(t, mock.ExpectationsWereMet()) // no DB calls at all
	})

	t.Run("reverts exactly the N most-recently-applied migrations", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001", Down: []string{"drop table a"}},
			&migration.Migration{Name: "0002", Down: []string{"drop table b"}},
			&migration.Migration{Name: "0003", Down: []string{"drop table c"}},
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}).
				AddRow("auth", "0001", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).
				AddRow("auth", "0002", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)).
				AddRow("auth", "0003", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)))

		// rolling back 1 step -> target becomes 0002, so only 0003 reverts
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0003").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`drop table c`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`DELETE FROM hypercube_migrations`).WithArgs("auth", "0003").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := app.RollbackSteps("auth", 1)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("steps greater than or equal to applied count reverts everything", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001", Down: []string{"drop table a"}},
			&migration.Migration{Name: "0002", Down: []string{"drop table b"}},
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}).
				AddRow("auth", "0001", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).
				AddRow("auth", "0002", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0002").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`drop table b`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`DELETE FROM hypercube_migrations`).WithArgs("auth", "0002").
			WillReturnResult(sqlmock.NewResult(0, 1))

		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`drop table a`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`DELETE FROM hypercube_migrations`).WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := app.RollbackSteps("auth", 99)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("nothing applied is a no-op after checking state", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerMigrationForNamespace("auth",
			&migration.Migration{Name: "0001"}))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT namespace, name, applied_at FROM hypercube_migrations`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"namespace", "name", "applied_at"}))

		err := app.RollbackSteps("auth", 1)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ensureMigrationsTable failure short-circuits", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnError(assert.AnError)

		err := app.RollbackSteps("auth", 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ensure migrations table")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("tears down dependents before their dependencies", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true
		// pluginB depends on pluginA, so after initPlugins-style sorting,
		// app.plugins is [pluginA, pluginB] (dependency-first).
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "pluginA"},
			&fakePlugin{name: "pluginB"},
		}

		require.NoError(t, app.registerMigrationForNamespace("pluginA",
			&migration.Migration{Name: "0001", Down: []string{"drop table pa"}}))
		require.NoError(t, app.registerMigrationForNamespace("pluginB",
			&migration.Migration{Name: "0001", Down: []string{"drop table pb"}}))

		// pluginB (the dependent) must be torn down FIRST
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("pluginB", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`drop table pb`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`DELETE FROM hypercube_migrations`).WithArgs("pluginB", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		// pluginA (the dependency) torn down SECOND
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("pluginA", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectBegin()
		mock.ExpectExec(`drop table pa`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
		mock.ExpectExec(`DELETE FROM hypercube_migrations`).WithArgs("pluginA", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := app.MigrateAllDown()
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet(), "sqlmock enforces call order, so this fails if pluginA were torn down before pluginB")
	})
}
