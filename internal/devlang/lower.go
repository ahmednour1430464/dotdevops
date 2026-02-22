package devlang

import (
	"fmt"

	"devopsctl/internal/plan"
)

// LowerToPlan converts a validated AST into a plan.Plan IR.
func LowerToPlan(file *File) (*plan.Plan, error) {
	p := &plan.Plan{
		Version: "1.0",
		Targets: make([]plan.Target, 0),
		Nodes:   make([]plan.Node, 0),
	}

	// Collect targets and nodes.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *TargetDecl:
			if d.Address == nil {
				return nil, fmt.Errorf("%s:%d:%d: target %q missing address", file.Path, d.Pos().Line, d.Pos().Col, d.Name)
			}
			p.Targets = append(p.Targets, plan.Target{
				ID:      d.Name,
				Address: d.Address.Value,
			})
		case *NodeDecl:
			n := plan.Node{
				ID:            d.Name,
				Type:          "",
				Targets:       nil,
				DependsOn:     nil,
				FailurePolicy: "",
				Inputs:        map[string]any{},
			}

			if d.Type != nil {
				n.Type = d.Type.Name
			}

			for _, t := range d.Targets {
				n.Targets = append(n.Targets, t.Name)
			}
			for _, dep := range d.DependsOn {
				n.DependsOn = append(n.DependsOn, dep.Value)
			}
			if d.FailurePolicy != nil {
				n.FailurePolicy = d.FailurePolicy.Name
			}

			for key, expr := range d.Inputs {
				v, err := lowerExpr(expr)
				if err != nil {
					return nil, err
				}
				n.Inputs[key] = v
			}

			p.Nodes = append(p.Nodes, n)
		}
	}

	return p, nil
}

func lowerExpr(e Expr) (any, error) {
	switch v := e.(type) {
	case *StringLiteral:
		return v.Value, nil
	case *BoolLiteral:
		return v.Value, nil
	case *ListLiteral:
		out := make([]any, 0, len(v.Elems))
		for _, elem := range v.Elems {
			// For v0.1 we only expect string literals in lists we lower.
			if s, ok := elem.(*StringLiteral); ok {
				out = append(out, s.Value)
				continue
			}
			return nil, fmt.Errorf("internal error: list literal contains non-string element at %d:%d", elem.Pos().Line, elem.Pos().Col)
		}
		return out, nil
	case *Ident:
		// Ident should not be lowered as a value in v0.1.
		return nil, fmt.Errorf("internal error: cannot lower identifier %q as a value at %d:%d", v.Name, v.Pos().Line, v.Pos().Col)
	default:
		return nil, fmt.Errorf("internal error: unsupported expression node in lowering")
	}
}

// LowerToPlanV0_2 converts a validated AST into a plan.Plan IR using a let environment for value substitution.
func LowerToPlanV0_2(file *File, lets LetEnv) (*plan.Plan, error) {
	p := &plan.Plan{
		Version: "1.0",
		Targets: make([]plan.Target, 0),
		Nodes:   make([]plan.Node, 0),
	}

	// Collect targets and nodes.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *TargetDecl:
			if d.Address == nil {
				return nil, fmt.Errorf("%s:%d:%d: target %q missing address", file.Path, d.Pos().Line, d.Pos().Col, d.Name)
			}
			p.Targets = append(p.Targets, plan.Target{
				ID:      d.Name,
				Address: d.Address.Value,
			})
		case *NodeDecl:
			n := plan.Node{
				ID:            d.Name,
				Type:          "",
				Targets:       nil,
				DependsOn:     nil,
				FailurePolicy: "",
				Inputs:        map[string]any{},
			}

			if d.Type != nil {
				n.Type = d.Type.Name
			}

			for _, t := range d.Targets {
				n.Targets = append(n.Targets, t.Name)
			}
			for _, dep := range d.DependsOn {
				n.DependsOn = append(n.DependsOn, dep.Value)
			}
			if d.FailurePolicy != nil {
				n.FailurePolicy = d.FailurePolicy.Name
			}

			for key, expr := range d.Inputs {
				v, err := lowerExprV0_2(expr, lets)
				if err != nil {
					return nil, err
				}
				n.Inputs[key] = v
			}

			p.Nodes = append(p.Nodes, n)
		}
	}

	return p, nil
}

func lowerExprV0_2(e Expr, lets LetEnv) (any, error) {
	switch v := e.(type) {
	case *StringLiteral:
		return v.Value, nil
	case *BoolLiteral:
		return v.Value, nil
	case *ListLiteral:
		out := make([]any, 0, len(v.Elems))
		for _, elem := range v.Elems {
			if s, ok := elem.(*StringLiteral); ok {
				out = append(out, s.Value)
				continue
			}
			return nil, fmt.Errorf("internal error: list literal contains non-string element at %d:%d", elem.Pos().Line, elem.Pos().Col)
		}
		return out, nil
	case *Ident:
		if lets == nil {
			return nil, fmt.Errorf("internal error: cannot lower identifier %q as a value at %d:%d", v.Name, v.Pos().Line, v.Pos().Col)
		}
		letExpr, ok := lets[v.Name]
		if !ok {
			return nil, fmt.Errorf("internal error: cannot lower identifier %q as a value at %d:%d", v.Name, v.Pos().Line, v.Pos().Col)
		}
		return lowerExprV0_2(letExpr, lets)
	default:
		return nil, fmt.Errorf("internal error: unsupported expression node in lowering")
	}
}

// LowerToPlanV0_4 converts a validated AST with steps into a plan.Plan IR.
// Steps are expanded to regular nodes at compile time (macro expansion).
func LowerToPlanV0_4(file *File, lets LetEnv, steps map[string]*StepDecl) (*plan.Plan, error) {
	p := &plan.Plan{
		Version: "1.0",
		Targets: make([]plan.Target, 0),
		Nodes:   make([]plan.Node, 0),
	}

	// Collect targets.
	for _, decl := range file.Decls {
		targetDecl, ok := decl.(*TargetDecl)
		if !ok {
			continue
		}
		if targetDecl.Address == nil {
			return nil, fmt.Errorf("%s:%d:%d: target %q missing address", file.Path, targetDecl.Pos().Line, targetDecl.Pos().Col, targetDecl.Name)
		}
		p.Targets = append(p.Targets, plan.Target{
			ID:      targetDecl.Name,
			Address: targetDecl.Address.Value,
		})
	}

	// Collect and expand nodes.
	for _, decl := range file.Decls {
		nodeDecl, ok := decl.(*NodeDecl)
		if !ok {
			continue
		}

		if nodeDecl.Type == nil {
			return nil, fmt.Errorf("%s:%d:%d: node %q missing type", file.Path, nodeDecl.Pos().Line, nodeDecl.Pos().Col, nodeDecl.Name)
		}

		typeName := nodeDecl.Type.Name

		// Check if this node references a step
		stepDecl, isStep := steps[typeName]

		var effectiveNode *NodeDecl
		if isStep {
			// Clone step body as base
			effectiveNode = &NodeDecl{
				Name:          nodeDecl.Name, // Use node's ID, not step's
				Type:          stepDecl.Body.Type,
				Targets:       nodeDecl.Targets,       // From node
				DependsOn:     nodeDecl.DependsOn,     // From node
				FailurePolicy: stepDecl.Body.FailurePolicy, // From step (can be overridden)
				Inputs:        make(map[string]Expr),
				PosInfo:       nodeDecl.PosInfo,
			}

			// Merge inputs: step defaults + node overrides
			for key, expr := range stepDecl.Body.Inputs {
				effectiveNode.Inputs[key] = expr
			}
			for key, expr := range nodeDecl.Inputs {
				effectiveNode.Inputs[key] = expr // Node overrides step
			}

			// Node can override failure_policy
			if nodeDecl.FailurePolicy != nil {
				effectiveNode.FailurePolicy = nodeDecl.FailurePolicy
			}
		} else {
			// Regular primitive node
			effectiveNode = nodeDecl
		}

		// Lower the effective node to plan.Node
		n := plan.Node{
			ID:            effectiveNode.Name,
			Type:          effectiveNode.Type.Name,
			Targets:       nil,
			DependsOn:     nil,
			FailurePolicy: "",
			Inputs:        map[string]any{},
		}

		for _, t := range effectiveNode.Targets {
			n.Targets = append(n.Targets, t.Name)
		}
		for _, dep := range effectiveNode.DependsOn {
			n.DependsOn = append(n.DependsOn, dep.Value)
		}
		if effectiveNode.FailurePolicy != nil {
			n.FailurePolicy = effectiveNode.FailurePolicy.Name
		}

		for key, expr := range effectiveNode.Inputs {
			v, err := lowerExprV0_2(expr, lets)
			if err != nil {
				return nil, err
			}
			n.Inputs[key] = v
		}

		p.Nodes = append(p.Nodes, n)
	}

	return p, nil
}
