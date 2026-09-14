package migration_test

import (
	"testing"

	"github.com/go-hypercube/go-hypercube/migration"
	"github.com/go-hypercube/go-hypercube/namespaced"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamespacedSlice_GetNamespace(t *testing.T) {
	slice := namespaced.NamespacedSlice[*migration.Migration]{
		{Namespace: "auth", Item: &migration.Migration{MigrationName: "b"}},
		{Namespace: "auth", Item: &migration.Migration{MigrationName: "a"}},
		{Namespace: "billing", Item: &migration.Migration{MigrationName: "x"}},
	}

	t.Run("existing namespace", func(t *testing.T) {
		got := slice.GetNamespace("auth")
		require.Len(t, got, 2)
		assert.Equal(t, "a", got[0].MigrationName)
		assert.Equal(t, "b", got[1].MigrationName)
	})

	t.Run("non-existing namespace", func(t *testing.T) {
		got := slice.GetNamespace("unknown")
		assert.Empty(t, got)
	})
}

func TestNamespacedSlice_GroupByNamespace(t *testing.T) {
	slice := namespaced.NamespacedSlice[*migration.Migration]{
		{Namespace: "auth", Item: &migration.Migration{MigrationName: "z"}},
		{Namespace: "auth", Item: &migration.Migration{MigrationName: "a"}},
		{Namespace: "billing", Item: &migration.Migration{MigrationName: "m"}},
	}

	groups := slice.GroupByNamespace()

	require.Contains(t, groups, "auth")
	require.Contains(t, groups, "billing")
	assert.Len(t, groups["auth"], 2)
	assert.Equal(t, "a", groups["auth"][0].MigrationName)
	assert.Equal(t, "z", groups["auth"][1].MigrationName)
	assert.Len(t, groups["billing"], 1)
	assert.Equal(t, "m", groups["billing"][0].MigrationName)
}

func TestNamespacedSlice_Sort(t *testing.T) {
	slice := namespaced.NamespacedSlice[*migration.Migration]{
		{Namespace: "b", Item: &migration.Migration{MigrationName: "b"}},
		{Namespace: "a", Item: &migration.Migration{MigrationName: "z"}},
		{Namespace: "a", Item: &migration.Migration{MigrationName: "a"}},
	}

	slice.Sort()

	expected := namespaced.NamespacedSlice[*migration.Migration]{
		{Namespace: "a", Item: &migration.Migration{MigrationName: "a"}},
		{Namespace: "a", Item: &migration.Migration{MigrationName: "z"}},
		{Namespace: "b", Item: &migration.Migration{MigrationName: "b"}},
	}
	assert.Equal(t, expected, slice)
}

func TestNamespacedSlice_Namespaces(t *testing.T) {
	slice := namespaced.NamespacedSlice[*migration.Migration]{
		{Namespace: "billing"},
		{Namespace: "auth"},
		{Namespace: "auth"},
		{Namespace: "admin"},
	}

	got := slice.Namespaces()
	expected := []string{"admin", "auth", "billing"} // sorted
	assert.Equal(t, expected, got)
}

func TestNamespacedSlice_Contains(t *testing.T) {
	slice := namespaced.NamespacedSlice[*migration.Migration]{
		{Namespace: "auth", Item: &migration.Migration{MigrationName: "init"}},
		{Namespace: "billing", Item: &migration.Migration{MigrationName: "create_invoices"}},
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

func TestNamespacedSlice_GetNamespaces(t *testing.T) {
	slice := namespaced.NamespacedSlice[*migration.Migration]{
		{Namespace: "auth", Item: &migration.Migration{MigrationName: "b"}},
		{Namespace: "auth", Item: &migration.Migration{MigrationName: "a"}},
		{Namespace: "billing", Item: &migration.Migration{MigrationName: "x"}},
		{Namespace: "admin", Item: &migration.Migration{MigrationName: "init"}},
	}

	t.Run("multiple namespaces", func(t *testing.T) {
		got := slice.GetNamespaces("auth", "billing")
		require.Len(t, got, 3)
		// Should be sorted: first auth (a, b), then billing (x)
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
		assert.Equal(t, "auth", got[0].Namespace)
		assert.Equal(t, "a", got[0].Item.Name())
		assert.Equal(t, "auth", got[1].Namespace)
		assert.Equal(t, "b", got[1].Item.Name())
	})

	t.Run("no arguments", func(t *testing.T) {
		got := slice.GetNamespaces()
		assert.Nil(t, got)
	})

	t.Run("non-existing namespace", func(t *testing.T) {
		got := slice.GetNamespaces("unknown")
		assert.Empty(t, got)
	})
}
