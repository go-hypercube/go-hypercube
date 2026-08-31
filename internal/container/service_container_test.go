package container

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Define mock interfaces and structs to test type resolution behaviors
type Database interface {
	Connect() string
}

type mockMySQL struct{}

func (m *mockMySQL) Connect() string { return "mysql connected" }

type mockPostgres struct{}

func (m *mockPostgres) Connect() string { return "postgres connected" }

func TestContainer_BasicBindAndResolve(t *testing.T) {
	c := NewServiceContainer()

	// Bind a simple primitive string
	Bind[string](c, "hello universe")

	// Verify discovery with TryResolve
	val, found := TryResolve[string](c)
	require.True(t, found, "String should be registered")
	assert.Equal(t, "hello universe", val)

	// Verify discovery with Resolve
	assert.Equal(t, "hello universe", Resolve[string](c))
}

func TestContainer_NamedScoping(t *testing.T) {
	c := NewServiceContainer()

	// Bind multiple variants of the same type under unique names
	Bind[int](c, 80, "http-port")
	Bind[int](c, 443, "https-port")
	Bind[int](c, 8080) // Unnamed global default

	// Verify named versions isolate and resolve correctly
	assert.Equal(t, 80, Resolve[int](c, "http-port"))
	assert.Equal(t, 443, Resolve[int](c, "https-port"))
	assert.Equal(t, 8080, Resolve[int](c))
}

func TestContainer_OverwriteBinding(t *testing.T) {
	c := NewServiceContainer()

	Bind[string](c, "initial-value")
	assert.Equal(t, "initial-value", Resolve[string](c))

	// Re-binding should cleanly overwrite the existing value
	Bind[string](c, "updated-value")
	assert.Equal(t, "updated-value", Resolve[string](c))
}

func TestContainer_InterfaceBindings(t *testing.T) {
	c := NewServiceContainer()

	// Bind distinct struct implementations explicitly as a Database interface
	var db1 Database = &mockMySQL{}
	var db2 Database = &mockPostgres{}

	Bind[Database](c, db1, "mysql")
	Bind[Database](c, db2, "postgres")

	// Resolve back through the generic interface signature
	res1, found1 := TryResolve[Database](c, "mysql")
	require.True(t, found1)
	assert.Equal(t, "mysql connected", res1.Connect())

	res2 := Resolve[Database](c, "postgres")
	assert.Equal(t, "postgres connected", res2.Connect())
}

func TestContainer_ResolvePanicOnMissing(t *testing.T) {
	c := NewServiceContainer()

	// Use assert.PanicsWithError to verify the formatted error message for an unnamed type
	assert.PanicsWithError(
		t,
		"container: float64 (name=\"\") not bound",
		func() { Resolve[float64](c) },
		"Should panic with specific message because float64 was never bound",
	)

	// Use assert.PanicsWithError to verify the formatted error message for a named type
	assert.PanicsWithError(
		t,
		"container: string (name=\"missing-key\") not bound",
		func() { Resolve[string](c, "missing-key") },
		"Should panic with specific message identifying the target name constraint",
	)
}

func TestContainer_TryResolveMissingReturnsZeroValue(t *testing.T) {
	c := NewServiceContainer()

	val, found := TryResolve[int](c, "non-existent")
	assert.False(t, found, "Should report false for non-bound configurations")
	assert.Equal(t, 0, val, "Missing bindings should return their types zero value")

	type dummyStruct struct{ Name string }
	structVal, structFound := TryResolve[dummyStruct](c)
	assert.False(t, structFound)
	assert.Empty(t, structVal.Name)
}

func TestContainer_ConcurrencySafety(t *testing.T) {
	c := NewServiceContainer()
	var wg sync.WaitGroup

	workers := 50
	iterations := 100

	// Spin up dynamic parallel readers and writers to stress-test RWMutex behavior
	for i := range workers {
		wg.Add(2)

		// Concurrent writers
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				Bind[int](c, workerID, "shared-worker-metric")
			}
		}(i)

		// Concurrent readers
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = TryResolve[int](c, "shared-worker-metric")
			}
		}()
	}

	wg.Wait()
}
