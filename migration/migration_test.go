package migration_test

import (
	"testing"

	"github.com/go-hypercube/go-hypercube/migration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamespacedSlice_GetNamespace(t *testing.T) {
	slice := migration.NamespacedSlice{
		{Namespace: "auth", Migration: &migration.Migration{Name: "b"}},
		{Namespace: "auth", Migration: &migration.Migration{Name: "a"}},
		{Namespace: "billing", Migration: &migration.Migration{Name: "x"}},
	}

	t.Run("existing namespace", func(t *testing.T) {
		got := slice.GetNamespace("auth")
		require.Len(t, got, 2)
		assert.Equal(t, "a", got[0].Name)
		assert.Equal(t, "b", got[1].Name)
	})

	t.Run("non-existing namespace", func(t *testing.T) {
		got := slice.GetNamespace("unknown")
		assert.Empty(t, got)
	})
}

func TestNamespacedSlice_GroupByNamespace(t *testing.T) {
	slice := migration.NamespacedSlice{
		{Namespace: "auth", Migration: &migration.Migration{Name: "z"}},
		{Namespace: "auth", Migration: &migration.Migration{Name: "a"}},
		{Namespace: "billing", Migration: &migration.Migration{Name: "m"}},
	}

	groups := slice.GroupByNamespace()

	require.Contains(t, groups, "auth")
	require.Contains(t, groups, "billing")
	assert.Len(t, groups["auth"], 2)
	assert.Equal(t, "a", groups["auth"][0].Name)
	assert.Equal(t, "z", groups["auth"][1].Name)
	assert.Len(t, groups["billing"], 1)
	assert.Equal(t, "m", groups["billing"][0].Name)
}

func TestNamespacedSlice_Sort(t *testing.T) {
	slice := migration.NamespacedSlice{
		{Namespace: "b", Migration: &migration.Migration{Name: "b"}},
		{Namespace: "a", Migration: &migration.Migration{Name: "z"}},
		{Namespace: "a", Migration: &migration.Migration{Name: "a"}},
	}

	slice.Sort()

	expected := migration.NamespacedSlice{
		{Namespace: "a", Migration: &migration.Migration{Name: "a"}},
		{Namespace: "a", Migration: &migration.Migration{Name: "z"}},
		{Namespace: "b", Migration: &migration.Migration{Name: "b"}},
	}
	assert.Equal(t, expected, slice)
}

func TestNamespacedSlice_Namespaces(t *testing.T) {
	slice := migration.NamespacedSlice{
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
	slice := migration.NamespacedSlice{
		{Namespace: "auth", Migration: &migration.Migration{Name: "init"}},
		{Namespace: "billing", Migration: &migration.Migration{Name: "create_invoices"}},
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
	slice := migration.NamespacedSlice{
		{Namespace: "auth", Migration: &migration.Migration{Name: "b"}},
		{Namespace: "auth", Migration: &migration.Migration{Name: "a"}},
		{Namespace: "billing", Migration: &migration.Migration{Name: "x"}},
		{Namespace: "admin", Migration: &migration.Migration{Name: "init"}},
	}

	t.Run("multiple namespaces", func(t *testing.T) {
		got := slice.GetNamespaces("auth", "billing")
		require.Len(t, got, 3)
		// Should be sorted: first auth (a, b), then billing (x)
		assert.Equal(t, "auth", got[0].Namespace)
		assert.Equal(t, "a", got[0].Name)
		assert.Equal(t, "auth", got[1].Namespace)
		assert.Equal(t, "b", got[1].Name)
		assert.Equal(t, "billing", got[2].Namespace)
		assert.Equal(t, "x", got[2].Name)
	})

	t.Run("single namespace", func(t *testing.T) {
		got := slice.GetNamespaces("auth")
		require.Len(t, got, 2)
		assert.Equal(t, "auth", got[0].Namespace)
		assert.Equal(t, "a", got[0].Name)
		assert.Equal(t, "auth", got[1].Namespace)
		assert.Equal(t, "b", got[1].Name)
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
