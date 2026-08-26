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

	// Map iterations in Go are randomized. The cycle loop can legally
	// start at any node depending on which node DFS hits first.
	expectedCycles := [][]string{
		{"A", "B", "C", "A"},
		{"B", "C", "A", "B"},
		{"C", "A", "B", "C"},
	}
	assert.Contains(t, expectedCycles, cycle, "Should output a valid, closed cycle loop path")
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

	expectedCycles := [][]string{
		{"M", "N", "M"},
		{"N", "M", "N"},
	}
	assert.Contains(t, expectedCycles, cycle)
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

// Helper function to find index of a slice element
func indexOf[T comparable](slice []T, element T) int {
	for i, v := range slice {
		if v == element {
			return i
		}
	}
	return -1
}
