package collection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSet_AddAndContains(t *testing.T) {
	var s Set[string]

	// Verify uninitialized set behavior
	assert.False(t, s.Contains("apple"), "Uninitialized set should not contain elements")

	// Add single elements
	s.Add("apple")
	s.Add("banana")
	s.Add("apple") // Duplicate add

	assert.True(t, s.Contains("apple"), "Set should contain 'apple'")
	assert.True(t, s.Contains("banana"), "Set should contain 'banana'")
	assert.False(t, s.Contains("orange"), "Set should not contain 'orange'")

	// Verify uniqueness via length of slice output
	slice := s.Slice()
	assert.Len(t, slice, 2, "Set must only keep unique entries")
}

func TestSet_AddAll(t *testing.T) {
	var s Set[int]

	// Add batch items including a duplicate
	s.AddAll(10, 20, 30, 20)

	assert.True(t, s.Contains(10))
	assert.True(t, s.Contains(20))
	assert.True(t, s.Contains(30))

	slice := s.Slice()
	require.Len(t, slice, 3)
	// ElementsMatch ignores index positions when verifying slices match
	assert.ElementsMatch(t, []int{10, 20, 30}, slice)
}

func TestSet_Remove(t *testing.T) {
	var s Set[string]

	// Test safe removal on an uninitialized map branch
	assert.NotPanics(t, func() {
		s.Remove("ghost")
	}, "Removing from uninitialized set should not panic")

	// Populate and remove active item
	s.AddAll("A", "B", "C")
	s.Remove("B")

	assert.True(t, s.Contains("A"))
	assert.False(t, s.Contains("B"), "B should be removed")
	assert.True(t, s.Contains("C"))

	// Test removal of non-existent item on an initialized map branch
	s.Remove("Z")
	assert.Len(t, s.Slice(), 2)
}

func TestSet_Clear(t *testing.T) {
	var s Set[float64]

	// Clear on uninitialized map is fine (Go's built-in clear handles nil maps safely)
	assert.NotPanics(t, func() {
		s.Clear()
	})

	// Populate and clear elements
	s.AddAll(1.1, 2.2, 3.3)
	require.Len(t, s.Slice(), 3)

	s.Clear()
	assert.Empty(t, s.Slice(), "Set should be completely empty after clear")
	assert.False(t, s.Contains(1.1))
}

func TestSet_GetSliceEmpty(t *testing.T) {
	// Uninitialized set
	var sUninit Set[int]
	assert.Equal(t, []int{}, sUninit.Slice(), "Uninitialized set should return empty slice allocation")

	// Explicitly cleared initialized set
	var sCleared Set[int]
	sCleared.Add(100)
	sCleared.Clear()
	assert.Equal(t, []int{}, sCleared.Slice(), "Cleared set should return empty slice allocation")
}
