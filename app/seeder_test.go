package app

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-hypercube/go-hypercube/plugin"
	"github.com/go-hypercube/go-hypercube/seeder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trackingFakeSeeder records how many times Run was invoked, and can
// be made to fail on demand.
type trackingFakeSeeder struct {
	name    string
	runs    int
	failErr error
}

func (s *trackingFakeSeeder) Name() string { return s.name }
func (s *trackingFakeSeeder) Run(_ *seeder.App) error {
	s.runs++
	return s.failErr
}

// ---- registration ----

func TestRegisterSeeder(t *testing.T) {
	app := &App{}

	s1 := &trackingFakeSeeder{name: "0001_users"}
	s2 := &trackingFakeSeeder{name: "0002_roles"}

	err := app.RegisterSeeder(s1, s2)
	require.NoError(t, err)

	got := app.Seeders()
	require.Len(t, got, 2)
	assert.Equal(t, hostAppNamespace, got[0].Namespace)
	assert.Equal(t, "0001_users", got[0].Name())
	assert.Equal(t, hostAppNamespace, got[1].Namespace)
	assert.Equal(t, "0002_roles", got[1].Name())
}

func TestRegisterSeederForNamespace_Appends(t *testing.T) {
	app := &App{}

	require.NoError(t, app.registerSeederForNamespace("pluginA", &trackingFakeSeeder{name: "0001"}))
	require.NoError(t, app.registerSeederForNamespace("pluginB", &trackingFakeSeeder{name: "0001"}))

	got := app.Seeders()
	require.Len(t, got, 2)
	assert.Equal(t, "pluginA", got[0].Namespace)
	assert.Equal(t, "pluginB", got[1].Namespace)
}

// ---- ensureSeedersTable ----

func TestEnsureSeedersTable(t *testing.T) {
	app, mock := newMockApp(t, "")

	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := app.ensureSeedersTable()
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureSeedersTable_Error(t *testing.T) {
	app, mock := newMockApp(t, "")

	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
		WillReturnError(assert.AnError)

	err := app.ensureSeedersTable()
	assert.ErrorIs(t, err, assert.AnError)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- hasRun ----

func TestHasRun(t *testing.T) {
	t.Run("true, unknown dialect uses ? placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM hypercube_seeders WHERE namespace = \? AND name = \?\)`).
			WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		ran, err := app.hasRun("auth", "0001")
		require.NoError(t, err)
		assert.True(t, ran)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("false", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM hypercube_seeders WHERE namespace = \? AND name = \?\)`).
			WithArgs("auth", "0002").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		ran, err := app.hasRun("auth", "0002")
		require.NoError(t, err)
		assert.False(t, ran)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres dialect uses $1 $2 placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "postgres")
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM hypercube_seeders WHERE namespace = \$1 AND name = \$2\)`).
			WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		ran, err := app.hasRun("auth", "0001")
		require.NoError(t, err)
		assert.True(t, ran)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectQuery(`SELECT EXISTS`).WillReturnError(assert.AnError)

		_, err := app.hasRun("auth", "0001")
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- markRun / clearRun ----

func TestMarkRun(t *testing.T) {
	t.Run("unknown dialect uses ? placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectExec(`INSERT INTO hypercube_seeders \(namespace, name\) VALUES \(\?, \?\)`).
			WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.markRun("auth", "0001")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres dialect uses $1 $2 placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "postgres")
		mock.ExpectExec(`INSERT INTO hypercube_seeders \(namespace, name\) VALUES \(\$1, \$2\)`).
			WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.markRun("auth", "0001")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WillReturnError(assert.AnError)

		err := app.markRun("auth", "0001")
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestClearRun(t *testing.T) {
	t.Run("unknown dialect uses ? placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectExec(`DELETE FROM hypercube_seeders WHERE namespace = \? AND name = \?`).
			WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := app.clearRun("auth", "0001")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres dialect uses $1 $2 placeholders", func(t *testing.T) {
		app, mock := newMockApp(t, "postgres")
		mock.ExpectExec(`DELETE FROM hypercube_seeders WHERE namespace = \$1 AND name = \$2`).
			WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := app.clearRun("auth", "0001")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectExec(`DELETE FROM hypercube_seeders`).WillReturnError(assert.AnError)

		err := app.clearRun("auth", "0001")
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- RunSeeder ----

func TestRunSeeder(t *testing.T) {
	t.Run("not yet run: runs and records", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		s := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("auth", s))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.RunSeeder("auth", "0001", false)
		require.NoError(t, err)
		assert.Equal(t, 1, s.runs)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("already run, not forced: skipped without running or re-recording", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		s := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("auth", s))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		// no INSERT/DELETE expected

		err := app.RunSeeder("auth", "0001", false)
		require.NoError(t, err)
		assert.Equal(t, 0, s.runs)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("already run, forced: hasRun not even checked, runs again, clears then re-records", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		s := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("auth", s))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		// no SELECT EXISTS: force skips the hasRun check entirely
		mock.ExpectExec(`DELETE FROM hypercube_seeders`).WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.RunSeeder("auth", "0001", true)
		require.NoError(t, err)
		assert.Equal(t, 1, s.runs)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("seeder not found in namespace", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := app.RunSeeder("auth", "does-not-exist", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `seeder "does-not-exist" not found in namespace "auth"`)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("seeder Run failure: no record written, error wrapped", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		s := &trackingFakeSeeder{name: "0001", failErr: assert.AnError}
		require.NoError(t, app.registerSeederForNamespace("auth", s))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		// no INSERT expected: Run failed

		err := app.RunSeeder("auth", "0001", false)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
		assert.Contains(t, err.Error(), `run seeder "auth"/"0001"`)
		assert.Equal(t, 1, s.runs) // it did attempt to run
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ensureSeedersTable failure short-circuits", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnError(assert.AnError)

		err := app.RunSeeder("auth", "0001", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ensure seeders table")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("hasRun query failure propagates without running seeder", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		s := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("auth", s))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WillReturnError(assert.AnError)

		err := app.RunSeeder("auth", "0001", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check run state")
		assert.Equal(t, 0, s.runs)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- RunSeedersForNamespace ----

func TestRunSeedersForNamespace(t *testing.T) {
	t.Run("runs all not-yet-run seeders in name order", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		s1 := &trackingFakeSeeder{name: "0001"}
		s2 := &trackingFakeSeeder{name: "0002"}
		require.NoError(t, app.registerSeederForNamespace("auth", s2, s1)) // registered out of order

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0002").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WithArgs("auth", "0002").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.RunSeedersForNamespace("auth", false)
		require.NoError(t, err)
		assert.Equal(t, 1, s1.runs)
		assert.Equal(t, 1, s2.runs)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stops at first failure, later seeders not attempted", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		s1 := &trackingFakeSeeder{name: "0001", failErr: assert.AnError}
		s2 := &trackingFakeSeeder{name: "0002"}
		require.NoError(t, app.registerSeederForNamespace("auth", s1, s2))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		// insert never happens: Run failed

		err := app.RunSeedersForNamespace("auth", false)
		require.Error(t, err)
		assert.Equal(t, 1, s1.runs)
		assert.Equal(t, 0, s2.runs) // never attempted
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty namespace is a no-op", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		err := app.RunSeedersForNamespace("empty-namespace", false)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("force re-runs all seeders regardless of prior state", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		s1 := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("auth", s1))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		// no SELECT EXISTS under force
		mock.ExpectExec(`DELETE FROM hypercube_seeders`).WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.RunSeedersForNamespace("auth", true)
		require.NoError(t, err)
		assert.Equal(t, 1, s1.runs)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- SeedAll ----

func TestSeedAll(t *testing.T) {
	t.Run("errors if Setup has not been called", func(t *testing.T) {
		app := &App{didSetup: false}
		err := app.SeedAll(false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "did you forget to call Setup()")
	})

	t.Run("seeds plugin namespaces first in plugin order, then remaining namespaces", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "pluginB"},
			&fakePlugin{name: "pluginA"}, // order must be preserved, not re-sorted
		}

		sb := &trackingFakeSeeder{name: "0001"}
		sa := &trackingFakeSeeder{name: "0001"}
		sdev := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("pluginB", sb))
		require.NoError(t, app.registerSeederForNamespace("pluginA", sa))
		require.NoError(t, app.registerSeederForNamespace(hostAppNamespace, sdev))

		// pluginB first
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("pluginB", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WithArgs("pluginB", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		// pluginA second
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("pluginA", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WithArgs("pluginA", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		// frameworkDevNamespace last
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs(hostAppNamespace, "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WithArgs(hostAppNamespace, "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.SeedAll(false)
		require.NoError(t, err)
		assert.Equal(t, 1, sb.runs)
		assert.Equal(t, 1, sa.runs)
		assert.Equal(t, 1, sdev.runs)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stops at first namespace that fails", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true
		app.plugins = []plugin.Plugin{&fakePlugin{name: "pluginA"}}

		sa := &trackingFakeSeeder{name: "0001", failErr: assert.AnError}
		sdev := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("pluginA", sa))
		require.NoError(t, app.registerSeederForNamespace(hostAppNamespace, sdev))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("pluginA", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		// insert never happens: Run failed

		err := app.SeedAll(false)
		require.Error(t, err)
		assert.Equal(t, 1, sa.runs)
		assert.Equal(t, 0, sdev.runs) // frameworkDevNamespace never reached
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("force propagates to every namespace", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.didSetup = true
		app.plugins = []plugin.Plugin{&fakePlugin{name: "pluginA"}}

		sa := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("pluginA", sa))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		// no SELECT EXISTS: forced
		mock.ExpectExec(`DELETE FROM hypercube_seeders`).WithArgs("pluginA", "0001").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WithArgs("pluginA", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.SeedAll(true)
		require.NoError(t, err)
		assert.Equal(t, 1, sa.runs)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
