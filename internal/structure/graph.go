package structure

import "github.com/go-hypercube/go-hypercube/internal/collection"

type Graph[V comparable] struct {
	Vertices map[V]*collection.Set[V]
}

func NewGraph[V comparable]() *Graph[V] {
	return &Graph[V]{Vertices: make(map[V]*collection.Set[V])}
}

func (g *Graph[V]) AddVertex(v V) bool {
	_, exists := g.Vertices[v]
	if !exists {
		g.Vertices[v] = &collection.Set[V]{}
	}
	return !exists
}

func (g *Graph[V]) AddEdge(v V, e V) {
	g.Vertices[v].Add(e)
}

// State map: 0 = Unvisited, 1 = Visiting, 2 = Visited
type state int

const (
	unvisited state = iota
	visiting
	visited
)

// GetTopologicalOrCycle returns:
// 1. The topological order (nil if cycle exists)
// 2. The cycle path (nil if no cycle exists)
func (g *Graph[V]) GetTopologicalOrCycle() ([]V, []V) {
	state := make(map[V]state)
	parent := make(map[V]V)
	var order []V

	var cycleStart V
	var cycleEnd V
	hasCycle := false

	var dfs func(current V)
	dfs = func(current V) {
		if hasCycle {
			return
		}

		state[current] = visiting

		for _, child := range g.Vertices[current].Slice() {
			if state[child] == visiting {
				// Cycle detected!
				hasCycle = true
				cycleStart = child // The node we looped back to
				cycleEnd = current // The node that points back to the start
				return
			}
			if state[child] == unvisited {
				parent[child] = current
				dfs(child)
				if hasCycle {
					return
				}
			}
		}

		state[current] = visited
		order = append(order, current)
	}

	// Check all vertices to catch disconnected graph cycles
	for vertex := range g.Vertices {
		if state[vertex] == unvisited {
			dfs(vertex)
		}
	}

	// If a cycle was found, reconstruct the path
	if hasCycle {
		var cyclePath []V
		curr := cycleEnd

		// Trace back from the end node to the start node of the cycle
		for curr != cycleStart {
			cyclePath = append(cyclePath, curr)
			curr = parent[curr]
		}
		cyclePath = append(cyclePath, cycleStart)

		// Reverse the path to show it in chronological order: Start -> ... -> End -> Start
		for i, j := 0, len(cyclePath)-1; i < j; i, j = i+1, j-1 {
			cyclePath[i], cyclePath[j] = cyclePath[j], cyclePath[i]
		}

		// Complete the loop visually by adding the start node at the end
		cyclePath = append(cyclePath, cycleStart)

		return nil, cyclePath
	}

	// Reverse topological order slice
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order, nil
}
