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

// LowerToPlanV0_5 converts a validated AST with steps and for-loops into a plan.Plan IR.
// Steps are recursively expanded and for-loops are unrolled at compile time.
func LowerToPlanV0_5(file *File, lets LetEnv, steps map[string]*StepDecl, forLoops []*ForDecl) (*plan.Plan, error) {
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

	// Step expansion cache for memoization
	primitiveTypes := map[string]bool{
		"file.sync":    true,
		"process.exec": true,
	}

	// Collect and expand regular nodes (not in for-loops).
	for _, decl := range file.Decls {
		nodeDecl, ok := decl.(*NodeDecl)
		if !ok {
			continue
		}

		if nodeDecl.Type == nil {
			return nil, fmt.Errorf("%s:%d:%d: node %q missing type", file.Path, nodeDecl.Pos().Line, nodeDecl.Pos().Col, nodeDecl.Name)
		}

		effectiveNode, err := expandNodeWithSteps(nodeDecl, steps, primitiveTypes, lets)
		if err != nil {
			return nil, err
		}

		n, err := lowerNodeToPlan(effectiveNode, lets)
		if err != nil {
			return nil, err
		}

		p.Nodes = append(p.Nodes, n)
	}

	// Unroll for-loops and expand nodes.
	for _, forDecl := range forLoops {
		// Resolve range to list literal (already validated)
		rangeExpr := forDecl.Range
		if ident, ok := rangeExpr.(*Ident); ok {
			if letVal, exists := lets[ident.Name]; exists {
				rangeExpr = letVal
			}
		}

		listLit, ok := rangeExpr.(*ListLiteral)
		if !ok {
			return nil, fmt.Errorf("internal error: for-loop range is not a list literal")
		}

		// Unroll loop: for each element, expand all nodes in body
		for _, elem := range listLit.Elems {
			strLit, ok := elem.(*StringLiteral)
			if !ok {
				continue
			}

			loopVarValue := strLit.Value

			// Process each node in for-loop body
			for _, bodyDecl := range forDecl.Body {
				nodeDecl, ok := bodyDecl.(*NodeDecl)
				if !ok {
					continue
				}

				// Deep clone node to prevent aliasing
				clonedNode := deepCloneNode(nodeDecl)

				// Substitute ${varName} with loop variable value
				substituteLoopVariable(clonedNode, forDecl.VarName, loopVarValue)

				// Expand with steps if needed
				effectiveNode, err := expandNodeWithSteps(clonedNode, steps, primitiveTypes, lets)
				if err != nil {
					return nil, err
				}

				n, err := lowerNodeToPlan(effectiveNode, lets)
				if err != nil {
					return nil, err
				}

				p.Nodes = append(p.Nodes, n)
			}
		}
	}

	return p, nil
}

// expandNodeWithSteps recursively expands a node that may reference steps.
func expandNodeWithSteps(nodeDecl *NodeDecl, steps map[string]*StepDecl, primitiveTypes map[string]bool, lets LetEnv) (*NodeDecl, error) {
	if nodeDecl.Type == nil {
		return nil, fmt.Errorf("node missing type")
	}

	typeName := nodeDecl.Type.Name

	// Check if this node references a step
	stepDecl, isStep := steps[typeName]

	if !isStep {
		// Regular primitive node
		return nodeDecl, nil
	}

	// Recursively expand the step
	expandedStep, err := expandStepRecursive(stepDecl, steps, primitiveTypes, make(map[string]*NodeDecl))
	if err != nil {
		return nil, err
	}

	// Merge node with expanded step
	effectiveNode := deepCloneNode(expandedStep)
	effectiveNode.Name = nodeDecl.Name            // Use node's ID
	effectiveNode.Targets = nodeDecl.Targets      // From node
	effectiveNode.DependsOn = nodeDecl.DependsOn  // From node

	// Merge inputs: node overrides step
	for key, expr := range nodeDecl.Inputs {
		effectiveNode.Inputs[key] = expr
	}

	// Node can override failure_policy
	if nodeDecl.FailurePolicy != nil {
		effectiveNode.FailurePolicy = nodeDecl.FailurePolicy
	}

	return effectiveNode, nil
}

// expandStepRecursive recursively expands a step to its primitive form.
func expandStepRecursive(stepDecl *StepDecl, steps map[string]*StepDecl, primitiveTypes map[string]bool, cache map[string]*NodeDecl) (*NodeDecl, error) {
	// Check cache
	if cached, ok := cache[stepDecl.Name]; ok {
		return deepCloneNode(cached), nil
	}

	if stepDecl.Body.Type == nil {
		return nil, fmt.Errorf("step %q missing type", stepDecl.Name)
	}

	typeName := stepDecl.Body.Type.Name

	var base *NodeDecl

	if primitiveTypes[typeName] {
		// Base case: primitive
		base = deepCloneNode(stepDecl.Body)
	} else {
		// Recursive case: expand parent step
		parentStep, ok := steps[typeName]
		if !ok {
			return nil, fmt.Errorf("step %q references unknown step %q", stepDecl.Name, typeName)
		}

		parent, err := expandStepRecursive(parentStep, steps, primitiveTypes, cache)
		if err != nil {
			return nil, err
		}
		base = deepCloneNode(parent)
	}

	// Merge step inputs into base
	for key, expr := range stepDecl.Body.Inputs {
		base.Inputs[key] = expr // Step overrides parent
	}

	// Handle failure_policy
	if stepDecl.Body.FailurePolicy != nil {
		base.FailurePolicy = stepDecl.Body.FailurePolicy
	}

	cache[stepDecl.Name] = deepCloneNode(base)
	return base, nil
}

// deepCloneNode creates a deep copy of a NodeDecl to prevent aliasing.
func deepCloneNode(node *NodeDecl) *NodeDecl {
	if node == nil {
		return nil
	}

	clone := &NodeDecl{
		Name:          node.Name,
		Type:          node.Type, // Type is *Ident, but we don't modify it
		Targets:       make([]*Ident, len(node.Targets)),
		DependsOn:     make([]*StringLiteral, len(node.DependsOn)),
		FailurePolicy: node.FailurePolicy,
		Inputs:        make(map[string]Expr, len(node.Inputs)),
		PosInfo:       node.PosInfo,
	}

	copy(clone.Targets, node.Targets)
	copy(clone.DependsOn, node.DependsOn)

	for key, expr := range node.Inputs {
		clone.Inputs[key] = expr // Shallow copy is OK for immutable Expr
	}

	return clone
}

// substituteLoopVariable substitutes ${varName} in node name and string inputs.
func substituteLoopVariable(node *NodeDecl, varName, value string) {
	// Substitute in node name
	node.Name = substituteInString(node.Name, varName, value)

	// Substitute in string literal inputs
	for key, expr := range node.Inputs {
		node.Inputs[key] = substituteInExpr(expr, varName, value)
	}
}

// substituteInString replaces ${varName} with value in a string.
func substituteInString(s, varName, value string) string {
	placeholder := "${" + varName + "}"
	result := ""
	for {
		idx := indexOf(s, placeholder)
		if idx == -1 {
			result += s
			break
		}
		result += s[:idx] + value
		s = s[idx+len(placeholder):]
	}
	return result
}

// indexOf returns the index of substr in s, or -1 if not found.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// substituteInExpr substitutes ${varName} in string literals within an expression.
func substituteInExpr(expr Expr, varName, value string) Expr {
	switch e := expr.(type) {
	case *StringLiteral:
		return &StringLiteral{
			Value:   substituteInString(e.Value, varName, value),
			PosInfo: e.PosInfo,
		}
	case *ListLiteral:
		newElems := make([]Expr, len(e.Elems))
		for i, elem := range e.Elems {
			newElems[i] = substituteInExpr(elem, varName, value)
		}
		return &ListLiteral{
			Elems:   newElems,
			PosInfo: e.PosInfo,
		}
	default:
		// Other types (BoolLiteral, Ident, etc.) are not substituted
		return expr
	}
}

// lowerNodeToPlan converts a fully expanded NodeDecl to plan.Node.
func lowerNodeToPlan(node *NodeDecl, lets LetEnv) (plan.Node, error) {
	n := plan.Node{
		ID:            node.Name,
		Type:          node.Type.Name,
		Targets:       nil,
		DependsOn:     nil,
		FailurePolicy: "",
		Inputs:        map[string]any{},
	}

	for _, t := range node.Targets {
		n.Targets = append(n.Targets, t.Name)
	}
	for _, dep := range node.DependsOn {
		n.DependsOn = append(n.DependsOn, dep.Value)
	}
	if node.FailurePolicy != nil {
		n.FailurePolicy = node.FailurePolicy.Name
	}

	for key, expr := range node.Inputs {
		v, err := lowerExprV0_2(expr, lets)
		if err != nil {
			return n, err
		}
		n.Inputs[key] = v
	}

	return n, nil
}
