package plan

import "fmt"

// Validate checks the plan for structural correctness.
// Returns a slice of errors (empty = valid).
func Validate(p *Plan) []error {
	var errs []error

	if p.Version == "" {
		errs = append(errs, fmt.Errorf("plan: missing 'version'"))
	}
	if len(p.Targets) == 0 {
		errs = append(errs, fmt.Errorf("plan: 'targets' must not be empty"))
	}
	if len(p.Nodes) == 0 {
		errs = append(errs, fmt.Errorf("plan: 'nodes' must not be empty"))
	}

	// Index targets for fast lookup
	targetIDs := make(map[string]bool, len(p.Targets))
	for i, t := range p.Targets {
		if t.ID == "" {
			errs = append(errs, fmt.Errorf("targets[%d]: missing 'id'", i))
		}
		if t.Address == "" {
			errs = append(errs, fmt.Errorf("targets[%d]: missing 'address'", i))
		}
		targetIDs[t.ID] = true
	}

	for i, n := range p.Nodes {
		if n.ID == "" {
			errs = append(errs, fmt.Errorf("nodes[%d]: missing 'id'", i))
		}
		if n.Type == "" {
			errs = append(errs, fmt.Errorf("nodes[%d]: missing 'type'", i))
		}
		if len(n.Targets) == 0 {
			errs = append(errs, fmt.Errorf("nodes[%d] (%s): 'targets' must not be empty", i, n.ID))
		}
		for _, tID := range n.Targets {
			if !targetIDs[tID] {
				errs = append(errs, fmt.Errorf("nodes[%d] (%s): unknown target '%s'", i, n.ID, tID))
			}
		}
		switch n.Type {
		case "file.sync":
			src, ok := n.Inputs["src"].(string)
			if !ok || src == "" {
				errs = append(errs, fmt.Errorf("nodes[%d] (%s): file.sync requires string 'src'", i, n.ID))
			}
			dest, ok := n.Inputs["dest"].(string)
			if !ok || dest == "" {
				errs = append(errs, fmt.Errorf("nodes[%d] (%s): file.sync requires string 'dest'", i, n.ID))
			}
		case "process.exec":
			cmdArr, ok := n.Inputs["cmd"].([]any)
			if !ok || len(cmdArr) == 0 {
				errs = append(errs, fmt.Errorf("nodes[%d] (%s): process.exec requires non-empty array 'cmd'", i, n.ID))
			}
			cwd, ok := n.Inputs["cwd"].(string)
			if !ok || cwd == "" {
				errs = append(errs, fmt.Errorf("nodes[%d] (%s): process.exec requires string 'cwd'", i, n.ID))
			}
		default:
			errs = append(errs, fmt.Errorf("nodes[%d] (%s): unknown type '%s'", i, n.ID, n.Type))
		}
	}

	return errs
}
