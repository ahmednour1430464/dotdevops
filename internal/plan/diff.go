package plan

import (
	"reflect"
)

// DiffResult represents the semantic diff between two plans.
type DiffResult struct {
	Added   []Node
	Removed []Node
	Changed []NodeDiff
}

// NodeDiff describes a node that is present in both plans but was modified.
type NodeDiff struct {
	Old Node
	New Node
}

// HasChanges returns true if there are any added, removed, or changed nodes.
func (r DiffResult) HasChanges() bool {
	return len(r.Added) > 0 || len(r.Removed) > 0 || len(r.Changed) > 0
}

// Diff compares an old plan with a new plan and returns the semantic differences.
func Diff(oldPlan, newPlan *Plan) DiffResult {
	var res DiffResult

	oldMap := make(map[string]Node)
	for _, n := range oldPlan.Nodes {
		oldMap[n.ID] = n
	}

	newMap := make(map[string]Node)
	for _, n := range newPlan.Nodes {
		newMap[n.ID] = n
	}

	// Find removed and changed
	for _, oldNode := range oldPlan.Nodes { // Preserve order
		if newNode, exists := newMap[oldNode.ID]; !exists {
			res.Removed = append(res.Removed, oldNode)
		} else if !reflect.DeepEqual(oldNode, newNode) {
			res.Changed = append(res.Changed, NodeDiff{Old: oldNode, New: newNode})
		}
	}

	// Find added
	for _, newNode := range newPlan.Nodes { // Preserve order
		if _, exists := oldMap[newNode.ID]; !exists {
			res.Added = append(res.Added, newNode)
		}
	}

	return res
}
