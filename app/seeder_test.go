package app

import (
	"testing"
	"time"

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

func TestRegisterSeederForNamespace_OverridesExistingByName(t *testing.T) {
	t.Run("re-registering the same (namespace, name) replaces in place, does not append", func(t *testing.T) {
		app := &App{}

		original := &trackingFakeSeeder{name: "0001_users"}
		replacement := &trackingFakeSeeder{name: "0001_users"}

		require.NoError(t, app.registerSeederForNamespace("auth", original))
		require.NoError(t, app.registerSeederForNamespace("auth", replacement))

		got := app.Seeders()
		require.Len(t, got, 1, "must not accumulate duplicate entries for the same namespace+name")
		assert.Same(t, replacement, got[0].Seeder, "the later registration must win")
	})

	t.Run("replacement preserves original position in registration order", func(t *testing.T) {
		app := &App{}

		s1 := &trackingFakeSeeder{name: "0001"}
		s2 := &trackingFakeSeeder{name: "0002"}
		s3 := &trackingFakeSeeder{name: "0003"}
		require.NoError(t, app.registerSeederForNamespace("auth", s1, s2, s3))

		replacement := &trackingFakeSeeder{name: "0002"}
		require.NoError(t, app.registerSeederForNamespace("auth", replacement))

		got := app.Seeders()
		require.Len(t, got, 3)
		assert.Same(t, s1, got[0].Seeder)
		assert.Same(t, replacement, got[1].Seeder, "replacement should occupy the original slot, not move to the end")
		assert.Same(t, s3, got[2].Seeder)
	})

	t.Run("same name in a different namespace is not affected", func(t *testing.T) {
		app := &App{}

		authSeeder := &trackingFakeSeeder{name: "init"}
		billingSeeder := &trackingFakeSeeder{name: "init"}

		require.NoError(t, app.registerSeederForNamespace("auth", authSeeder))
		require.NoError(t, app.registerSeederForNamespace("billing", billingSeeder))

		seeders := app.Seeders()
		require.Equal(t, len(seeders), 2, "same seeder name in different namespaces must both be kept")
		assert.Same(t, authSeeder, seeders.GetSeeder("auth", "init"))
		assert.Same(t, billingSeeder, seeders.GetSeeder("billing", "init"))
	})

	t.Run("GetSeeder returns the replacement after override", func(t *testing.T) {
		app := &App{}

		original := &trackingFakeSeeder{name: "0001"}
		replacement := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("auth", original))
		require.NoError(t, app.registerSeederForNamespace("auth", replacement))

		got := app.Seeders().GetSeeder("auth", "0001")
		require.NotNil(t, got)
		assert.Same(t, replacement, got)
	})

	t.Run("RunSeedersForNamespace runs the replacement, not the original, exactly once", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		original := &trackingFakeSeeder{name: "0001"}
		replacement := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("auth", original))
		require.NoError(t, app.registerSeederForNamespace("auth", replacement))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs("auth", "0001").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec(`INSERT INTO hypercube_seeders`).WithArgs("auth", "0001").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := app.RunSeedersForNamespace("auth", false)
		require.NoError(t, err)

		assert.Equal(t, 0, original.runs, "the overridden original must never run")
		assert.Equal(t, 1, replacement.runs, "only the replacement should run")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("registering multiple seeders in one call, one of which overrides", func(t *testing.T) {
		app := &App{}

		existing := &trackingFakeSeeder{name: "0001"}
		require.NoError(t, app.registerSeederForNamespace("auth", existing))

		replacement := &trackingFakeSeeder{name: "0001"}
		brandNew := &trackingFakeSeeder{name: "0002"}
		require.NoError(t, app.registerSeederForNamespace("auth", replacement, brandNew))

		got := app.Seeders()
		require.Len(t, got, 2)
		assert.Same(t, replacement, got.GetSeeder("auth", "0001"))
		assert.Same(t, brandNew, got.GetSeeder("auth", "0002"))
	})
}

// ---- orderedSeederNamespaces ----

func TestOrderedSeederNamespaces(t *testing.T) {
	t.Run("plugins first in dependency order, then remaining sorted", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{
			&fakePlugin{name: "pluginB"},
			&fakePlugin{name: "pluginA"}, // order must be preserved
		}

		require.NoError(t, app.registerSeederForNamespace("pluginB", &trackingFakeSeeder{name: "0001"}))
		require.NoError(t, app.registerSeederForNamespace("pluginA", &trackingFakeSeeder{name: "0001"}))
		require.NoError(t, app.registerSeederForNamespace(hostAppNamespace, &trackingFakeSeeder{name: "0001"}))
		require.NoError(t, app.registerSeederForNamespace("zzz-no-plugin", &trackingFakeSeeder{name: "0001"}))

		got := app.orderedSeederNamespaces()
		assert.Equal(t, []string{"pluginB", "pluginA", hostAppNamespace, "zzz-no-plugin"}, got)
	})

	t.Run("plugin with no registered seeders is skipped", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{&fakePlugin{name: "pluginA"}}
		require.NoError(t, app.registerSeederForNamespace(hostAppNamespace, &trackingFakeSeeder{name: "0001"}))

		got := app.orderedSeederNamespaces()
		assert.Equal(t, []string{hostAppNamespace}, got)
	})

	t.Run("no plugins, only sorted remainder", func(t *testing.T) {
		app := &App{}
		require.NoError(t, app.registerSeederForNamespace("billing", &trackingFakeSeeder{name: "0001"}))
		require.NoError(t, app.registerSeederForNamespace("auth", &trackingFakeSeeder{name: "0001"}))

		got := app.orderedSeederNamespaces()
		assert.Equal(t, []string{"auth", "billing"}, got)
	})

	t.Run("no seeders registered at all", func(t *testing.T) {
		app := &App{}
		app.plugins = []plugin.Plugin{&fakePlugin{name: "pluginA"}}

		got := app.orderedSeederNamespaces()
		assert.Empty(t, got)
	})
}

// ---- seederRunTimes ----

func TestSeederRunTimes(t *testing.T) {
	t.Run("returns name -> run_at map, unknown dialect uses ? placeholder", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "run_at"}).
				AddRow("0001", t1).
				AddRow("0002", t2))

		got, err := app.seederRunTimes("auth")
		require.NoError(t, err)
		assert.Equal(t, map[string]time.Time{"0001": t1, "0002": t2}, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("postgres dialect uses $1 placeholder", func(t *testing.T) {
		app, mock := newMockApp(t, "postgres")

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders WHERE namespace = \$1`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "run_at"}))

		got, err := app.seederRunTimes("auth")
		require.NoError(t, err)
		assert.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("empty result set", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "run_at"}))

		got, err := app.seederRunTimes("auth")
		require.NoError(t, err)
		assert.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders`).
			WillReturnError(assert.AnError)

		_, err := app.seederRunTimes("auth")
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("row scan error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "run_at"}).
				AddRow("0001", "not-a-time"))

		_, err := app.seederRunTimes("auth")
		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- namespaceSeederState ----

func TestNamespaceSeederState(t *testing.T) {
	t.Run("mix of run and pending", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerSeederForNamespace("auth",
			&trackingFakeSeeder{name: "0001"},
			&trackingFakeSeeder{name: "0002"},
			&trackingFakeSeeder{name: "0003"},
		))

		t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "run_at"}).
				AddRow("0001", t1).
				AddRow("0002", t2))
			// 0003 not run

		state, err := app.namespaceSeederState("auth")
		require.NoError(t, err)

		assert.Equal(t, "auth", state.Namespace)
		assert.Equal(t, 1, state.Pending)
		require.Len(t, state.Statuses, 3)

		assert.Equal(t, SeederStatus{Namespace: "auth", Name: "0001", HasRun: true, RanAt: &t1}, state.Statuses[0])
		assert.Equal(t, SeederStatus{Namespace: "auth", Name: "0002", HasRun: true, RanAt: &t2}, state.Statuses[1])
		assert.Equal(t, SeederStatus{Namespace: "auth", Name: "0003", HasRun: false, RanAt: nil}, state.Statuses[2])

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("nothing run", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerSeederForNamespace("auth", &trackingFakeSeeder{name: "0001"}))

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "run_at"}))

		state, err := app.namespaceSeederState("auth")
		require.NoError(t, err)
		assert.Equal(t, 1, state.Pending)
		assert.False(t, state.Statuses[0].HasRun)
		assert.Nil(t, state.Statuses[0].RanAt)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("everything run", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerSeederForNamespace("auth",
			&trackingFakeSeeder{name: "0001"},
			&trackingFakeSeeder{name: "0002"},
		))

		t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders WHERE namespace = \?`).
			WithArgs("auth").
			WillReturnRows(sqlmock.NewRows([]string{"name", "run_at"}).
				AddRow("0001", t1).
				AddRow("0002", t2))

		state, err := app.namespaceSeederState("auth")
		require.NoError(t, err)
		assert.Equal(t, 0, state.Pending)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("namespace with no registered seeders", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders WHERE namespace = \?`).
			WithArgs("empty-namespace").
			WillReturnRows(sqlmock.NewRows([]string{"name", "run_at"}))

		state, err := app.namespaceSeederState("empty-namespace")
		require.NoError(t, err)
		assert.Equal(t, "empty-namespace", state.Namespace)
		assert.Equal(t, 0, state.Pending)
		assert.Empty(t, state.Statuses)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("seederRunTimes error propagates", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerSeederForNamespace("auth", &trackingFakeSeeder{name: "0001"}))

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders`).
			WillReturnError(assert.AnError)

		_, err := app.namespaceSeederState("auth")
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- SeederState ----

func TestSeederState(t *testing.T) {
	t.Run("ensureSeedersTable failure short-circuits", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnError(assert.AnError)

		_, err := app.SeederState()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ensure seeders table")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no namespaces registered returns empty slice, no error", func(t *testing.T) {
		app, mock := newMockApp(t, "")

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		got, err := app.SeederState()
		require.NoError(t, err)
		assert.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("aggregates state across multiple namespaces in plugin-first order", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		app.plugins = []plugin.Plugin{&fakePlugin{name: "pluginA"}}

		require.NoError(t, app.registerSeederForNamespace("pluginA",
			&trackingFakeSeeder{name: "0001"},
			&trackingFakeSeeder{name: "0002"},
		))
		require.NoError(t, app.registerSeederForNamespace(hostAppNamespace,
			&trackingFakeSeeder{name: "0001"},
		))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders WHERE namespace = \?`).
			WithArgs("pluginA").
			WillReturnRows(sqlmock.NewRows([]string{"name", "run_at"}).
				AddRow("0001", t1))

		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders WHERE namespace = \?`).
			WithArgs(hostAppNamespace).
			WillReturnRows(sqlmock.NewRows([]string{"name", "run_at"}))

		got, err := app.SeederState()
		require.NoError(t, err)
		require.Len(t, got, 2)

		assert.Equal(t, "pluginA", got[0].Namespace)
		assert.Equal(t, 1, got[0].Pending)

		assert.Equal(t, hostAppNamespace, got[1].Namespace)
		assert.Equal(t, 1, got[1].Pending)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("per-namespace query error is wrapped with namespace context", func(t *testing.T) {
		app, mock := newMockApp(t, "")
		require.NoError(t, app.registerSeederForNamespace("auth", &trackingFakeSeeder{name: "0001"}))

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS hypercube_seeders`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT name, run_at FROM hypercube_seeders`).
			WillReturnError(assert.AnError)

		_, err := app.SeederState()
		require.Error(t, err)
		assert.Contains(t, err.Error(), `get seeder state for namespace "auth"`)
		assert.ErrorIs(t, err, assert.AnError)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
