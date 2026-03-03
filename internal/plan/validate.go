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

	// Index targets and nodes for fast lookup
	targetIDs := make(map[string]bool, len(p.Targets))
	nodeIDs := make(map[string]bool, len(p.Nodes))

	for _, n := range p.Nodes {
		nodeIDs[n.ID] = true
	}
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

		for _, dep := range n.DependsOn {
			if !nodeIDs[dep] {
				errs = append(errs, fmt.Errorf("nodes[%d] (%s): unknown depends_on node '%s'", i, n.ID, dep))
			}
		}

		if n.When != nil {
			if !nodeIDs[n.When.Node] {
				errs = append(errs, fmt.Errorf("nodes[%d] (%s): unknown when.node '%s'", i, n.ID, n.When.Node))
			}
		}

		if n.FailurePolicy != "" && n.FailurePolicy != "halt" && n.FailurePolicy != "continue" && n.FailurePolicy != "rollback" {
			errs = append(errs, fmt.Errorf("nodes[%d] (%s): invalid failure_policy '%s'", i, n.ID, n.FailurePolicy))
		}

		switch n.Type {
		case "file.sync":
			// file.sync: basic structural validation is enough at IR level.
		case "process.exec":
			// Validate process.exec required inputs
			cmd, cmdOk := n.Inputs["cmd"].([]any)
			if !cmdOk || len(cmd) == 0 {
				errs = append(errs, fmt.Errorf("nodes[%d] (%s): process.exec requires non-empty array 'cmd'", i, n.ID))
			}
			if cwd, cwdExists := n.Inputs["cwd"]; cwdExists {
				if _, cwdOk := cwd.(string); !cwdOk {
					errs = append(errs, fmt.Errorf("nodes[%d] (%s): process.exec 'cwd' must be a string", i, n.ID))
				}
			}
		case "_fs.write", "_fs.mkdir", "_fs.delete", "_fs.chmod", "_fs.chown", "_fs.exists", "_fs.stat", "_exec":
			// Atomic built-ins: detailed input validation is handled by the agent context enforcement.
		default:
			// Allow unknown types (user-defined primitives from v1.2+)
			// Validation for custom primitives happens at compile time, not plan time.
		}
	}

	return errs
}
