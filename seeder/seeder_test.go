package seeder_test

import (
	"testing"

	"github.com/go-hypercube/go-hypercube/namespaced"
	"github.com/go-hypercube/go-hypercube/seeder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSeeder is a minimal seeder.Seeder for testing NamespacedSlice
// behavior without needing a real *seeder.App.
type fakeSeeder struct {
	name string
}

func (s *fakeSeeder) Name() string            { return s.name }
func (s *fakeSeeder) Run(_ *seeder.App) error { return nil }

func TestNamespacedSlice_GetNamespace(t *testing.T) {
	slice := namespaced.NamespacedSlice[seeder.Seeder]{
		seeder.NewNamespaced("auth", &fakeSeeder{name: "b"}),
		seeder.NewNamespaced("auth", &fakeSeeder{name: "a"}),
		seeder.NewNamespaced("billing", &fakeSeeder{name: "x"}),
	}

	t.Run("existing namespace, sorted by name", func(t *testing.T) {
		got := slice.GetNamespace("auth")
		require.Len(t, got, 2)
		assert.Equal(t, "a", got[0].Name())
		assert.Equal(t, "b", got[1].Name())
	})

	t.Run("non-existing namespace", func(t *testing.T) {
		got := slice.GetNamespace("unknown")
		assert.Empty(t, got)
	})
}

func TestNamespacedSlice_GroupByNamespace(t *testing.T) {
	slice := namespaced.NamespacedSlice[seeder.Seeder]{
		seeder.NewNamespaced("auth", &fakeSeeder{name: "z"}),
		seeder.NewNamespaced("auth", &fakeSeeder{name: "a"}),
		seeder.NewNamespaced("billing", &fakeSeeder{name: "m"}),
	}

	groups := slice.GroupByNamespace()

	require.Contains(t, groups, "auth")
	require.Contains(t, groups, "billing")
	require.Len(t, groups["auth"], 2)
	assert.Equal(t, "a", groups["auth"][0].Name())
	assert.Equal(t, "z", groups["auth"][1].Name())
	require.Len(t, groups["billing"], 1)
	assert.Equal(t, "m", groups["billing"][0].Name())
}

func TestNamespacedSlice_Sort(t *testing.T) {
	slice := namespaced.NamespacedSlice[seeder.Seeder]{
		seeder.NewNamespaced("b", &fakeSeeder{name: "b"}),
		seeder.NewNamespaced("a", &fakeSeeder{name: "z"}),
		seeder.NewNamespaced("a", &fakeSeeder{name: "a"}),
	}

	slice.Sort()

	require.Len(t, slice, 3)
	assert.Equal(t, "a", slice[0].Namespace)
	assert.Equal(t, "a", slice[0].Item.Name())
	assert.Equal(t, "a", slice[1].Namespace)
	assert.Equal(t, "z", slice[1].Item.Name())
	assert.Equal(t, "b", slice[2].Namespace)
	assert.Equal(t, "b", slice[2].Item.Name())
}

func TestNamespacedSlice_Namespaces(t *testing.T) {
	slice := namespaced.NamespacedSlice[seeder.Seeder]{
		seeder.NewNamespaced("billing", &fakeSeeder{name: "x"}),
		seeder.NewNamespaced("auth", &fakeSeeder{name: "a"}),
		seeder.NewNamespaced("auth", &fakeSeeder{name: "b"}),
		seeder.NewNamespaced("admin", &fakeSeeder{name: "c"}),
	}

	got := slice.Namespaces()
	assert.Equal(t, []string{"admin", "auth", "billing"}, got)
}

func TestNamespacedSlice_Contains(t *testing.T) {
	slice := namespaced.NamespacedSlice[seeder.Seeder]{
		seeder.NewNamespaced("auth", &fakeSeeder{name: "init"}),
		seeder.NewNamespaced("billing", &fakeSeeder{name: "create_invoices"}),
	}

	t.Run("found", func(t *testing.T) {
		assert.True(t, slice.Contains("auth", "init"))
		assert.True(t, slice.Contains("billing", "create_invoices"))
	})

	t.Run("not found", func(t *testing.T) {
		assert.False(t, slice.Contains("auth", "unknown"))
		assert.False(t, slice.Contains("unknown", "init"))
		assert.False(t, slice.Contains("auth", ""))
	})
}

func TestNamespacedSlice_GetSeeder(t *testing.T) {
	target := &fakeSeeder{name: "init"}
	slice := namespaced.NamespacedSlice[seeder.Seeder]{
		seeder.NewNamespaced("auth", target),
		seeder.NewNamespaced("billing", &fakeSeeder{name: "init"}), // same name, different namespace
	}

	t.Run("found returns matching namespaced seeder", func(t *testing.T) {
		got, ok := slice.Get("auth", "init")
		require.True(t, ok)
		assert.Equal(t, "init", got.Name())
	})

	t.Run("not found returns nil", func(t *testing.T) {
		_, ok := slice.Get("auth", "does-not-exist")
		assert.False(t, ok)
		_, ok = slice.Get("unknown", "init")
		assert.False(t, ok)
	})

	t.Run("namespace disambiguates same-named seeders", func(t *testing.T) {
		gotAuth, ok := slice.Get("auth", "init")
		require.True(t, ok)
		gotBilling, ok := slice.Get("billing", "init")
		require.True(t, ok)
		assert.NotSame(t, gotAuth, gotBilling)
	})
}

func TestNamespacedSlice_GetNamespaces(t *testing.T) {
	slice := namespaced.NamespacedSlice[seeder.Seeder]{
		seeder.NewNamespaced("auth", &fakeSeeder{name: "b"}),
		seeder.NewNamespaced("auth", &fakeSeeder{name: "a"}),
		seeder.NewNamespaced("billing", &fakeSeeder{name: "x"}),
		seeder.NewNamespaced("admin", &fakeSeeder{name: "init"}),
	}

	t.Run("multiple namespaces, sorted by namespace then name", func(t *testing.T) {
		got := slice.GetNamespaces("auth", "billing")
		require.Len(t, got, 3)
		assert.Equal(t, "auth", got[0].Namespace)
		assert.Equal(t, "a", got[0].Item.Name())
		assert.Equal(t, "auth", got[1].Namespace)
		assert.Equal(t, "b", got[1].Item.Name())
		assert.Equal(t, "billing", got[2].Namespace)
		assert.Equal(t, "x", got[2].Item.Name())
	})

	t.Run("single namespace", func(t *testing.T) {
		got := slice.GetNamespaces("auth")
		require.Len(t, got, 2)
		assert.Equal(t, "a", got[0].Item.Name())
		assert.Equal(t, "b", got[1].Item.Name())
	})

	t.Run("no arguments returns nil", func(t *testing.T) {
		assert.Nil(t, slice.GetNamespaces())
	})

	t.Run("non-existing namespace returns empty", func(t *testing.T) {
		got := slice.GetNamespaces("unknown")
		assert.Empty(t, got)
	})
}
