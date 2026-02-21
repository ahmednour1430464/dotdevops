package controller

import (
	"fmt"

	"devopsctl/internal/plan"
)

// Graph represents a directed acyclic graph of execution nodes.
type Graph struct {
	Nodes    map[string]plan.Node
	Edges    map[string][]string // Adjacency List: From Node ID -> To Node IDs
	InDegree map[string]int      // Number of incoming edges
}

// BuildGraph constructs a dependency graph from a list of nodes and validates it is acyclic.
func BuildGraph(nodes []plan.Node) (*Graph, error) {
	g := &Graph{
		Nodes:    make(map[string]plan.Node),
		Edges:    make(map[string][]string),
		InDegree: make(map[string]int),
	}

	for _, n := range nodes {
		if _, exists := g.Nodes[n.ID]; exists {
			return nil, fmt.Errorf("duplicate node ID: %s", n.ID)
		}
		g.Nodes[n.ID] = n
		g.InDegree[n.ID] = 0 // Initialize to 0
	}

	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			if _, exists := g.Nodes[dep]; !exists {
				return nil, fmt.Errorf("node %q depends on unknown node %q", n.ID, dep)
			}
			g.Edges[dep] = append(g.Edges[dep], n.ID)
			g.InDegree[n.ID]++
		}
	}

	// Detect cycles
	if err := g.detectCycles(); err != nil {
		return nil, err
	}

	return g, nil
}

// detectCycles performs Kahn's algorithm for topological sorting to ensure there are no cycles.
func (g *Graph) detectCycles() error {
	inDegreeCopy := make(map[string]int)
	for id, deg := range g.InDegree {
		inDegreeCopy[id] = deg
	}

	var queue []string
	for id, deg := range inDegreeCopy {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	count := 0
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		count++

		for _, v := range g.Edges[u] {
			inDegreeCopy[v]--
			if inDegreeCopy[v] == 0 {
				queue = append(queue, v)
			}
		}
	}

	if count != len(g.Nodes) {
		return fmt.Errorf("cycle detected in dependency graph")
	}

	return nil
}
