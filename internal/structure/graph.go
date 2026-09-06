package structure

import "slices"

type Graph[V comparable] struct {
	Vertices map[V][]V // adjacency lists; slice preserves AddEdge order
	order    []V       // preserves AddVertex insertion order for deterministic outer-loop traversal
}

func NewGraph[V comparable]() *Graph[V] {
	return &Graph[V]{Vertices: make(map[V][]V)}
}

func (g *Graph[V]) AddVertex(v V) bool {
	_, exists := g.Vertices[v]
	if !exists {
		g.Vertices[v] = nil
		g.order = append(g.order, v)
	}
	return !exists
}

// AddEdge adds a directed edge from v to e (v -> e), meaning v must
// come before e in GetTopologicalOrCycle's ordering (see that method's
// doc comment for the full convention).
//
// If this exact edge (v -> e) has already been added, AddEdge is a
// no-op — it will not appear twice in v's adjacency list. This keeps
// g.Vertices[v] free of duplicates without needing a separate set,
// preserving deterministic insertion order (unlike the old
// collection.Set-backed adjacency) while still guaranteeing each edge
// is only ever visited once during traversal.
//
// AddEdge does not implicitly add v or e as vertices — callers must
// call AddVertex for both endpoints beforehand
func (g *Graph[V]) AddEdge(v V, e V) {
	if slices.Contains(g.Vertices[v], e) {
		return
	}
	g.Vertices[v] = append(g.Vertices[v], e)
}

// State map: 0 = Unvisited, 1 = Visiting, 2 = Visited
type state int

const (
	unvisited state = iota
	visiting
	visited
)

// GetTopologicalOrCycle computes a topological ordering of the graph's
// vertices using a recursive depth-first search with cycle detection,
// visiting every vertex (including those in disconnected components).
//
// The returned order respects edge direction: for every edge u -> v
// added via AddEdge(u, v), u appears before v in the result. This
// matches the natural reading of AddEdge(u, v) as "u must come before
// v" / "u points to v" — for example, in a build-step or task-ordering
// graph where an edge means "run this, then that." If your graph
// instead models edges as "depends on" (AddEdge(dependent, dependency)),
// use GetReverseTopologicalOrCycle instead to get dependencies ordered
// before their dependents.
//
// If the graph contains no cycle, GetTopologicalOrCycle returns:
//  1. The topological order: every vertex exactly once, ordered so
//     that for every edge u -> v, u precedes v.
//  2. nil for the cycle path.
//
// If the graph contains a cycle, GetTopologicalOrCycle instead returns:
//  1. nil for the order, since no valid topological order exists.
//  2. The cycle path: a closed loop of vertices, starting and ending
//     with the same vertex (e.g. [A, B, C, A]), showing one concrete
//     cycle found in the graph. If the graph has multiple disconnected
//     components and more than one contains a cycle, only one such
//     cycle is reported — the one belonging to whichever component is
//     reached first — not every cycle in the graph.
//
// Both the returned order and, when applicable, the returned cycle path
// (including which vertex it starts from) are fully deterministic for a
// given sequence of AddVertex/AddEdge calls: vertices are traversed in
// the order they were added via AddVertex, and each vertex's edges are
// visited in the order they were added via AddEdge. Two graphs built
// with the same calls in the same order will always produce identical
// results — this includes vertices with no path between them, which
// retain their relative AddVertex order in the returned slice rather
// than an arbitrary one.
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

		for _, child := range g.Vertices[current] {
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
	for _, vertex := range g.order {
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

// GetReverseTopologicalOrCycle returns the reverse of GetTopologicalOrCycle's
// order: if an edge u -> v means "u must come before v" (GetTopologicalOrCycle's
// convention), the reverse order instead guarantees v comes before u for
// every edge — i.e. it satisfies "dependencies before dependents" when
// edges are built as dependent -> dependency (AddEdge(p, dep)), which is
// the more common way to model a dependency graph.
//
// Returns:
//  1. The reverse topological order (nil if a cycle exists)
//  2. The cycle path (nil if no cycle exists), identical to what
//     GetTopologicalOrCycle would report for the same graph
//
// This does not re-run cycle detection independently — it delegates
// entirely to GetTopologicalOrCycle and reverses the resulting slice
// in place, so the two functions can never disagree about whether a
// cycle exists or where it is, and no extra slice is allocated for the
// reversal.
func (g *Graph[V]) GetReverseTopologicalOrCycle() ([]V, []V) {
	order, cycle := g.GetTopologicalOrCycle()
	if cycle != nil {
		return nil, cycle
	}

	// Reverse topological order slice
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order, nil
}
