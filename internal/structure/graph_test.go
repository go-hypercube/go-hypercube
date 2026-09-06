package structure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTopologicalOrCycle_ValidDAG(t *testing.T) {
	// A -> B -> C -> D
	//      \---> D
	g := NewGraph[string]()
	g.AddVertex("A")
	g.AddVertex("B")
	g.AddVertex("C")
	g.AddVertex("D")

	g.AddEdge("A", "B")
	g.AddEdge("B", "C")
	g.AddEdge("B", "D")
	g.AddEdge("C", "D")

	order, cycle := g.GetTopologicalOrCycle()

	// Use assert for cycle because it should be nil
	assert.Nil(t, cycle, "Valid DAG should not have a cycle")

	// Use require here: if order is nil, trying to parse indices below will panic the test.
	require.NotNil(t, order, "Valid DAG must return a topological order")
	require.Len(t, order, 4, "Order should contain all 4 vertices")

	// Verify topological constraints (dependencies must come before their children)
	assert.Less(t, indexOf(order, "A"), indexOf(order, "B"), "A must come before B")
	assert.Less(t, indexOf(order, "B"), indexOf(order, "C"), "B must come before C")
	assert.Less(t, indexOf(order, "B"), indexOf(order, "D"), "B must come before D")
	assert.Less(t, indexOf(order, "C"), indexOf(order, "D"), "C must come before D")
}

func TestGetTopologicalOrCycle_SimpleCycle(t *testing.T) {
	// A -> B -> C -> A
	g := NewGraph[string]()
	g.AddVertex("A")
	g.AddVertex("B")
	g.AddVertex("C")

	g.AddEdge("A", "B")
	g.AddEdge("B", "C")
	g.AddEdge("C", "A")

	order, cycle := g.GetTopologicalOrCycle()

	assert.Nil(t, order, "Graph with a cycle cannot have a topological order")

	// Use require here: if cycle is nil, the slice evaluation below fails horribly
	require.NotNil(t, cycle, "Cycle path should be detected")

	expectedCycles := []string{"A", "B", "C", "A"}
	assert.Equal(t, expectedCycles, cycle, "Should output a valid, closed cycle loop path")
}

func TestGetTopologicalOrCycle_DeepCycle(t *testing.T) {
	// A -> B -> C -> D -> B (Cycle is safely isolated to B-C-D-B)
	//                \--> E
	g := NewGraph[string]()
	g.AddVertex("A")
	g.AddVertex("B")
	g.AddVertex("C")
	g.AddVertex("D")
	g.AddVertex("E")

	g.AddEdge("A", "B")
	g.AddEdge("B", "C")
	g.AddEdge("C", "D")
	g.AddEdge("D", "B")
	g.AddEdge("D", "E")

	order, cycle := g.GetTopologicalOrCycle()

	assert.Nil(t, order)
	require.NotNil(t, cycle)
	assert.Equal(t, []string{"B", "C", "D", "B"}, cycle, "Should isolate only nodes participating in the cycle")
}

func TestGetTopologicalOrCycle_DisconnectedGraphWithCycle(t *testing.T) {
	// Component 1 (Safe): X -> Y
	// Component 2 (Has Cycle): M -> N -> M
	g := NewGraph[string]()
	g.AddVertex("X")
	g.AddVertex("Y")
	g.AddVertex("M")
	g.AddVertex("N")

	g.AddEdge("X", "Y")
	g.AddEdge("M", "N")
	g.AddEdge("N", "M")

	order, cycle := g.GetTopologicalOrCycle()

	assert.Nil(t, order, "Global graph topological order fails if any single component has a cycle")
	require.NotNil(t, cycle)

	expectedCycles := []string{"M", "N", "M"}
	assert.Equal(t, expectedCycles, cycle)
}

func TestGetTopologicalOrCycle_EmptyAndSingleNode(t *testing.T) {
	// Case 1: Empty graph
	gEmpty := NewGraph[int]()
	order, cycle := gEmpty.GetTopologicalOrCycle()
	assert.Empty(t, order)
	assert.Nil(t, cycle)

	// Case 2: Single node
	gSingle := NewGraph[int]()
	gSingle.AddVertex(42)
	order, cycle = gSingle.GetTopologicalOrCycle()

	require.NotNil(t, order)
	assert.Equal(t, []int{42}, order)
	assert.Nil(t, cycle)
}

func TestGetReverseTopologicalOrCycle_ValidDAG(t *testing.T) {
	// A -> B -> C -> D
	//      \---> D
	g := NewGraph[string]()
	g.AddVertex("A")
	g.AddVertex("B")
	g.AddVertex("C")
	g.AddVertex("D")

	g.AddEdge("A", "B")
	g.AddEdge("B", "C")
	g.AddEdge("B", "D")
	g.AddEdge("C", "D")

	forward, _ := g.GetTopologicalOrCycle()
	reverse, cycle := g.GetReverseTopologicalOrCycle()

	assert.Nil(t, cycle)
	require.NotNil(t, reverse)
	require.Len(t, reverse, 4)

	// reverse must be the exact reversal of forward
	require.Len(t, forward, 4)
	for i := range forward {
		assert.Equal(t, forward[len(forward)-1-i], reverse[i])
	}

	// and it must satisfy "v before u" for every edge u -> v
	assert.Less(t, indexOf(reverse, "B"), indexOf(reverse, "A"), "B must come before A in reverse order")
	assert.Less(t, indexOf(reverse, "C"), indexOf(reverse, "B"), "C must come before B in reverse order")
	assert.Less(t, indexOf(reverse, "D"), indexOf(reverse, "B"), "D must come before B in reverse order")
	assert.Less(t, indexOf(reverse, "D"), indexOf(reverse, "C"), "D must come before C in reverse order")
}

func TestGetReverseTopologicalOrCycle_EmptyAndSingleNode(t *testing.T) {
	gEmpty := NewGraph[int]()
	order, cycle := gEmpty.GetReverseTopologicalOrCycle()
	assert.Empty(t, order)
	assert.Nil(t, cycle)

	gSingle := NewGraph[int]()
	gSingle.AddVertex(42)
	order, cycle = gSingle.GetReverseTopologicalOrCycle()
	require.NotNil(t, order)
	assert.Equal(t, []int{42}, order)
	assert.Nil(t, cycle)
}

func TestAddEdge_Deduplicates(t *testing.T) {
	t.Run("adding the same edge twice does not duplicate it", func(t *testing.T) {
		g := NewGraph[string]()
		g.AddVertex("A")
		g.AddVertex("B")

		g.AddEdge("A", "B")
		g.AddEdge("A", "B") // duplicate

		assert.Equal(t, []string{"B"}, g.Vertices["A"])
	})

	t.Run("adding different edges from the same vertex both persist", func(t *testing.T) {
		g := NewGraph[string]()
		g.AddVertex("A")
		g.AddVertex("B")
		g.AddVertex("C")

		g.AddEdge("A", "B")
		g.AddEdge("A", "C")
		g.AddEdge("A", "B") // duplicate of the first

		assert.Equal(t, []string{"B", "C"}, g.Vertices["A"])
	})

	t.Run("duplicate edges do not affect topological order or cycle detection", func(t *testing.T) {
		g := NewGraph[string]()
		g.AddVertex("A")
		g.AddVertex("B")
		g.AddEdge("A", "B")
		g.AddEdge("A", "B")
		g.AddEdge("A", "B")

		order, cycle := g.GetTopologicalOrCycle()
		assert.Nil(t, cycle)
		assert.Equal(t, []string{"A", "B"}, order)
	})
}

// Helper function to find index of a slice element
func indexOf[T comparable](slice []T, element T) int {
	for i, v := range slice {
		if v == element {
			return i
		}
	}
	return -1
}
