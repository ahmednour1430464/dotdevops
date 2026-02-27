package devlang

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

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
	case *NumberLiteral:
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
	case *NumberLiteral:
		return v.Value, nil
	case *ListLiteral:
		out := make([]any, 0, len(v.Elems))
		for _, elem := range v.Elems {
			val, err := lowerExprV0_2(elem, lets)
			if err != nil {
				return nil, err
			}
			out = append(out, val)
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
	case *BinaryExpr:
		left, err := lowerExprV0_2(v.Left, lets)
		if err != nil {
			return nil, err
		}
		right, err := lowerExprV0_2(v.Right, lets)
		if err != nil {
			return nil, err
		}
		switch v.Op {
		case OpAdd:
			return fmt.Sprintf("%v%v", left, right), nil
		default:
			return nil, fmt.Errorf("unsupported operator %v in lowering", v.Op)
		}
	case *SecretRef:
		// Emits the sentinel JSON object {"__secret__": "KEY"}
		return map[string]interface{}{"__secret__": v.Key}, nil
	default:
		return nil, fmt.Errorf("internal error: unsupported expression node %T in lowering", e)
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
// SecretRef inputs are emitted as "[SECRET:KEY]" sentinel strings in Inputs,
// and the secret key is collected into RequiresSecrets.
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

	secretSet := map[string]bool{}
	for key, expr := range node.Inputs {
		// Special-case SecretRef: emit sentinel placeholder, record the key.
		if sr, ok := expr.(*SecretRef); ok {
			n.Inputs[key] = "[SECRET:" + sr.Key + "]"
			secretSet[sr.Key] = true
			continue
		}
		v, err := lowerExprV0_2(expr, lets)
		if err != nil {
			return n, err
		}
		// If the value came back as the __secret__ map (from lowerExprV0_2 handling SecretRef
		// embedded inside another expr), convert to sentinel string.
		if m, ok := v.(map[string]interface{}); ok {
			if secretKey, ok := m["__secret__"].(string); ok {
				n.Inputs[key] = "[SECRET:" + secretKey + "]"
				secretSet[secretKey] = true
				continue
			}
		}
		n.Inputs[key] = v
	}

	// Collect sorted secret keys for deterministic output.
	if len(secretSet) > 0 {
		keys := make([]string, 0, len(secretSet))
		for k := range secretSet {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		n.RequiresSecrets = keys
	}

	return n, nil
}


// LowerToPlanV0_6 converts a validated AST with steps, parameters, and for-loops into a plan.Plan IR.
// Parameters are substituted during step expansion (v0.6 new feature).
func LowerToPlanV0_6(file *File, lets LetEnv, steps map[string]*StepDecl, forLoops []*ForDecl) (*plan.Plan, error) {
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

		effectiveNode, err := expandNodeWithStepsV0_6(nodeDecl, steps, primitiveTypes, lets)
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
				effectiveNode, err := expandNodeWithStepsV0_6(clonedNode, steps, primitiveTypes, lets)
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

// expandNodeWithStepsV0_6 recursively expands a node that may reference steps (with parameter substitution).
func expandNodeWithStepsV0_6(nodeDecl *NodeDecl, steps map[string]*StepDecl, primitiveTypes map[string]bool, lets LetEnv) (*NodeDecl, error) {
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

	// v0.6: Build parameter environment from node inputs and step defaults
	paramEnv := make(map[string]Expr)
	for _, param := range stepDecl.Params {
		// Check if node provides this parameter
		if providedValue, ok := nodeDecl.Inputs[param.Name]; ok {
			paramEnv[param.Name] = providedValue
		} else if param.Default != nil {
			// Use default value
			paramEnv[param.Name] = param.Default
		}
		// If neither provided nor default, validation should have caught this (required param missing)
	}

	// Recursively expand the step with parameter environment
	expandedStep, err := expandStepRecursiveV0_6(stepDecl, steps, primitiveTypes, paramEnv, lets, make(map[string]*NodeDecl))
	if err != nil {
		return nil, err
	}

	// Merge node with expanded step
	effectiveNode := deepCloneNode(expandedStep)
	effectiveNode.Name = nodeDecl.Name            // Use node's ID
	effectiveNode.Targets = nodeDecl.Targets      // From node
	effectiveNode.DependsOn = nodeDecl.DependsOn  // From node

	// Merge inputs: node overrides step (non-parameter inputs)
	for key, expr := range nodeDecl.Inputs {
		// Skip parameters (already handled in paramEnv)
		isParam := false
		for _, param := range stepDecl.Params {
			if key == param.Name {
				isParam = true
				break
			}
		}
		if !isParam {
			effectiveNode.Inputs[key] = expr
		}
	}

	// Node can override failure_policy
	if nodeDecl.FailurePolicy != nil {
		effectiveNode.FailurePolicy = nodeDecl.FailurePolicy
	}

	return effectiveNode, nil
}

// expandStepRecursiveV0_6 recursively expands a step to its primitive form (with parameter substitution).
func expandStepRecursiveV0_6(stepDecl *StepDecl, steps map[string]*StepDecl, primitiveTypes map[string]bool, paramEnv map[string]Expr, lets LetEnv, cache map[string]*NodeDecl) (*NodeDecl, error) {
	// Note: Caching with parameters is complex (would need to include paramEnv in cache key).
	// For simplicity in v0.6, we'll skip memoization across different parameter bindings.
	// This is still deterministic and correct, just potentially less efficient.

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

		// Build parameter environment for parent step
		parentParamEnv := make(map[string]Expr)
		for _, param := range parentStep.Params {
			// Check if current step body provides this parameter
			if providedValue, ok := stepDecl.Body.Inputs[param.Name]; ok {
				// Substitute current step's parameters in provided value
				substituted := substituteParamsInExpr(providedValue, paramEnv)
				parentParamEnv[param.Name] = substituted
			} else if param.Default != nil {
				// Use default value
				parentParamEnv[param.Name] = param.Default
			}
		}

		parent, err := expandStepRecursiveV0_6(parentStep, steps, primitiveTypes, parentParamEnv, lets, cache)
		if err != nil {
			return nil, err
		}
		base = deepCloneNode(parent)
	}

	// Merge step inputs into base (with parameter substitution)
	for key, expr := range stepDecl.Body.Inputs {
		// Substitute parameters in expression
		substituted := substituteParamsInExpr(expr, paramEnv)
		base.Inputs[key] = substituted
	}

	// Handle failure_policy
	if stepDecl.Body.FailurePolicy != nil {
		base.FailurePolicy = stepDecl.Body.FailurePolicy
	}

	return base, nil
}

// substituteParamsInExpr substitutes parameter references in an expression with their values from paramEnv.
// Identifier resolution order: paramEnv first, then expression as-is (will be resolved by lets later).
func substituteParamsInExpr(expr Expr, paramEnv map[string]Expr) Expr {
	switch e := expr.(type) {
	case *Ident:
		if paramVal, ok := paramEnv[e.Name]; ok {
			return paramVal
		}
		return e
	case *BinaryExpr:
		return &BinaryExpr{
			Left:    substituteParamsInExpr(e.Left, paramEnv),
			Op:      e.Op,
			Right:   substituteParamsInExpr(e.Right, paramEnv),
			PosInfo: e.PosInfo,
		}
	case *TernaryExpr:
		return &TernaryExpr{
			Cond:      substituteParamsInExpr(e.Cond, paramEnv),
			TrueExpr:  substituteParamsInExpr(e.TrueExpr, paramEnv),
			FalseExpr: substituteParamsInExpr(e.FalseExpr, paramEnv),
			PosInfo:   e.PosInfo,
		}
	case *ListLiteral:
		newElems := make([]Expr, len(e.Elems))
		for i, elem := range e.Elems {
			newElems[i] = substituteParamsInExpr(elem, paramEnv)
		}
		return &ListLiteral{
			Elems:   newElems,
			PosInfo: e.PosInfo,
		}
	case *SecretRef:
		// Secret keys shouldn't be parameterized usually, but if they are it's not supported yet
		return expr
	default:
		// Literals (StringLiteral, BoolLiteral) are not substituted
		return expr
	}
}

// LowerToPlanV0_8 converts a validated AST (v0.8) into a plan.Plan IR.
func LowerToPlanV0_8(file *File, lets LetEnv, steps map[string]*StepDecl, forLoops []*ForDecl, fleets map[string]*FleetDecl) (*plan.Plan, error) {
	// First run v0.6 lower logic which handles lets, steps, and for loops
	p, err := LowerToPlanV0_6(file, lets, steps, forLoops)
	if err != nil {
		return nil, err
	}
	
	p.Version = "1.0"

	// Gather Targets with v0.8 Labels support.
	// Since LowerToPlanV0_6 already creates p.Targets, we just need to update it.
	targetLabels := map[string]map[string]string{}
	for _, decl := range file.Decls {
		if t, ok := decl.(*TargetDecl); ok {
			targetLabels[t.Name] = t.Labels
		}
	}
	
	// Update target labels in plan
	for i := range p.Targets {
		p.Targets[i].Labels = targetLabels[p.Targets[i].ID]
	}

	// Gather original Nodes to map contract fields
	nodeMap := map[string]*NodeDecl{}
	for _, decl := range file.Decls {
		if n, ok := decl.(*NodeDecl); ok {
			nodeMap[n.Name] = n
		} else if s, ok := decl.(*StepDecl); ok {
			// Step bodies (single NodeDecl in v0.6+)
			if s.Body != nil {
				nodeMap[s.Body.Name] = s.Body
			}
		}
	}

	// Re-map nodes for fleets and contracts
	var finalNodes []plan.Node
	for _, n := range p.Nodes {
		// 1. Resolve Fleets
		resolvedSet := map[string]bool{}
		for _, tRef := range n.Targets {
			if f, ok := fleets[tRef]; ok {
				// Expand fleet
				for _, t := range p.Targets {
					isMatch := true
					for k, v := range f.Match {
						if t.Labels[k] != v {
							isMatch = false
							break
						}
					}
					if isMatch {
						resolvedSet[t.ID] = true
					}
				}
			} else {
				resolvedSet[tRef] = true
			}
		}
		n.Targets = []string{}
		for t := range resolvedSet {
			n.Targets = append(n.Targets, t)
		}
		sort.Strings(n.Targets)

		// 2. Map Contracts (if we can find the original decl)
		// Try exact match (top-level nodes)
		decl := nodeMap[n.ID]
		if decl == nil {
			// Try step prefix match (e.g. "step_name.node_name")
			for k, v := range nodeMap {
				if len(n.ID) > len(k)+1 && n.ID[len(n.ID)-len(k):] == k && n.ID[len(n.ID)-len(k)-1] == '.' {
					decl = v
					break
				}
			}
		}

		if decl != nil {
			if decl.Idempotent != nil {
				n.Idempotent = decl.Idempotent.Value
			}
			if decl.SideEffects != nil {
				n.SideEffects = decl.SideEffects.Value
			}
			if decl.Retry != nil && decl.Retry.Attempts > 0 {
				n.Retry = &plan.RetryConfig{
					Attempts: decl.Retry.Attempts,
					Delay:    decl.Retry.Delay,
				}
			}
			if decl.RollbackCmd != nil {
				for _, e := range decl.RollbackCmd.Elems {
					if s, ok := e.(*StringLiteral); ok {
						n.RollbackCmd = append(n.RollbackCmd, s.Value)
					}
				}
			}
		}

		finalNodes = append(finalNodes, n)
	}
	
	p.Nodes = finalNodes

	return p, nil
}

// LowerToPlanV1_2 converts a validated AST (v1.2) into a plan.Plan IR, handling primitive expansion.
func LowerToPlanV1_2(file *File, lets LetEnv, steps map[string]*StepDecl, forLoops []*ForDecl, fleets map[string]*FleetDecl, primitives map[string]*PrimitiveDecl) (*plan.Plan, error) {
	// 1. First run v0.8 logic to get a baseline plan (un-expanded primitives will just be nodes of that type)
	p, err := LowerToPlanV0_8(file, lets, steps, forLoops, fleets)
	if err != nil {
		return nil, err
	}

	// 2. Expand custom primitives
	finalNodes := []plan.Node{}
	for _, n := range p.Nodes {
		if prim, ok := primitives[n.Type]; ok {
			expanded, err := expandCustomPrimitive(n, prim, primitives, lets)
			if err != nil {
				return nil, err
			}
			finalNodes = append(finalNodes, expanded...)
		} else {
			finalNodes = append(finalNodes, n)
		}
	}
	p.Nodes = finalNodes

	return p, nil
}

// expandCustomPrimitive recursively expands a custom primitive into one or more built-in nodes.
func expandCustomPrimitive(inst plan.Node, prim *PrimitiveDecl, primitives map[string]*PrimitiveDecl, lets LetEnv) ([]plan.Node, error) {
	// Build mapping from primitive input names to provided values in the instance
	inputEnv := make(map[string]Expr)
	for _, inDecl := range prim.Inputs {
		val, ok := inst.Inputs[inDecl.Name]
		if !ok {
			// This should be caught by validation
			return nil, fmt.Errorf("missing required input %q for primitive %q", inDecl.Name, prim.Name)
		}
		// Convert the already lowered 'any' value back into a literal Expr for substitution
		inputEnv[inDecl.Name] = literalToExpr(val)
	}

	// Evaluate prepare block if present (v1.4+)
	prepareEnv := make(map[string]any)
	if prim.Prepare != nil {
		var err error
		prepareEnv, err = evaluatePrepareBlock(prim.Prepare, inputEnv)
		if err != nil {
			return nil, fmt.Errorf("evaluating prepare block: %w", err)
		}
	}

	var result []plan.Node
	for _, decl := range prim.Body {
		// Handle ForeachBodyDecl (v1.4+)
		if foreachDecl, ok := decl.(*ForeachBodyDecl); ok {
			expanded, err := expandForeachBody(foreachDecl, inst, prim, primitives, lets, inputEnv, prepareEnv)
			if err != nil {
				return nil, err
			}
			result = append(result, expanded...)
			continue
		}

		nodeDecl, ok := decl.(*NodeDecl)
		if !ok {
			continue
		}

		expandedNodes, err := expandPrimitiveNode(nodeDecl, inst, prim, primitives, lets, inputEnv)
		if err != nil {
			return nil, err
		}
		result = append(result, expandedNodes...)
	}

	return result, nil
}

// expandPrimitiveNode expands a single node declaration from a primitive body.
func expandPrimitiveNode(nodeDecl *NodeDecl, inst plan.Node, prim *PrimitiveDecl, primitives map[string]*PrimitiveDecl, lets LetEnv, inputEnv map[string]Expr) ([]plan.Node, error) {
	// Substitute inputs in nodeDecl
	expandedNodeDecl := deepCloneNode(nodeDecl)
	for key, expr := range expandedNodeDecl.Inputs {
		expandedNodeDecl.Inputs[key] = substituteInputsInExpr(expr, inputEnv)
	}
	
	// Map back to plan.Node
	n, err := lowerNodeToPlan(expandedNodeDecl, lets)
	if err != nil {
		return nil, err
	}

	// Fix identifiers: n.ID should be unique. Prefix with parent ID.
	n.ID = inst.ID + "." + n.ID
	
	// Fix depends_on: also prefix internal dependencies
	// Collect all internal node names for this primitive
	internalNodes := make(map[string]bool)
	for _, d := range prim.Body {
		if nd, ok := d.(*NodeDecl); ok {
			internalNodes[nd.Name] = true
		}
	}

	newDependsOn := make([]string, 0, len(n.DependsOn))
	for _, dep := range n.DependsOn {
		if internalNodes[dep] {
			newDependsOn = append(newDependsOn, inst.ID+"."+dep)
		} else {
			newDependsOn = append(newDependsOn, dep)
		}
	}
	n.DependsOn = newDependsOn
	
	// Inherit targets if the expanded node doesn't specify them
	if len(n.Targets) == 0 {
		n.Targets = inst.Targets
	}
	
	// Add metadata about source
	if n.Inputs == nil {
		n.Inputs = make(map[string]any)
	}
	n.Inputs["_source"] = fmt.Sprintf("primitive:%s", prim.Name)

	// Propagate contract from primitive to expanded node (v1.2+)
	if prim.Contract != nil {
		if prim.Contract.Idempotent != nil {
			n.Idempotent = *prim.Contract.Idempotent
		}
		if prim.Contract.SideEffects != nil {
			n.SideEffects = *prim.Contract.SideEffects
		}
		if prim.Contract.Retry != nil {
			n.Retry = &plan.RetryConfig{
				Attempts: *prim.Contract.Retry,
				Delay:    "1s", // Default delay
			}
		}
	}

	// Propagate probe and desired from primitive (v1.3+)
	if prim.Probe != nil {
		n.Probe = serializeProbeDecl(prim.Probe, inputEnv)
	}
	if prim.Desired != nil {
		n.Desired = serializeDesiredDecl(prim.Desired, inputEnv)
	}

	// Recursive expansion if this node is itself a custom primitive
	if subPrim, ok := primitives[n.Type]; ok {
		subExpanded, err := expandCustomPrimitive(n, subPrim, primitives, lets)
		if err != nil {
			return nil, err
		}
		return subExpanded, nil
	}
	return []plan.Node{n}, nil
}

// expandForeachBody expands a foreach block into multiple nodes.
func expandForeachBody(foreachDecl *ForeachBodyDecl, inst plan.Node, prim *PrimitiveDecl, primitives map[string]*PrimitiveDecl, lets LetEnv, inputEnv map[string]Expr, prepareEnv map[string]any) ([]plan.Node, error) {
	// Resolve the range expression
	rangeVal, err := resolveRangeExpr(foreachDecl.Range, inputEnv, prepareEnv)
	if err != nil {
		return nil, fmt.Errorf("resolving foreach range: %w", err)
	}

	// rangeVal must be a list
	list, ok := rangeVal.([]any)
	if !ok {
		return nil, fmt.Errorf("foreach range must be a list, got %T", rangeVal)
	}

	var result []plan.Node
	for _, elem := range list {
		// Build loop variable environment
		loopEnv := make(map[string]Expr)
		for k, v := range inputEnv {
			loopEnv[k] = v
		}
		// Add loop variable
		loopEnv[foreachDecl.VarName] = anyToExpr(elem)

		// Expand each body declaration
		for _, bodyDecl := range foreachDecl.Body {
			if nodeDecl, ok := bodyDecl.(*NodeDecl); ok {
				// Clone and substitute
				cloned := deepCloneNode(nodeDecl)
				for key, expr := range cloned.Inputs {
					cloned.Inputs[key] = substituteInputsInExpr(expr, loopEnv)
				}

				// Also substitute in node name for uniqueness
				cloned.Name = substituteVarInString(cloned.Name, foreachDecl.VarName, elem)

				expandedNodes, err := expandPrimitiveNode(cloned, inst, prim, primitives, lets, loopEnv)
				if err != nil {
					return nil, err
				}
				result = append(result, expandedNodes...)
			}
			// Handle nested foreach
			if nestedForeach, ok := bodyDecl.(*ForeachBodyDecl); ok {
				nestedNodes, err := expandForeachBody(nestedForeach, inst, prim, primitives, lets, loopEnv, prepareEnv)
				if err != nil {
					return nil, err
				}
				result = append(result, nestedNodes...)
			}
		}
	}

	return result, nil
}

// resolveRangeExpr resolves a foreach range expression to a concrete value.
func resolveRangeExpr(expr Expr, inputEnv map[string]Expr, prepareEnv map[string]any) (any, error) {
	switch e := expr.(type) {
	case *Ident:
		// Check for "prepare.X" pattern
		if len(e.Name) > 8 && e.Name[:8] == "prepare." {
			varName := e.Name[8:]
			if val, ok := prepareEnv[varName]; ok {
				return val, nil
			}
			return nil, fmt.Errorf("undefined prepare variable %q", varName)
		}
		// Check inputs
		if val, ok := inputEnv[e.Name]; ok {
			return exprToAny(val)
		}
		return nil, fmt.Errorf("undefined variable %q in foreach range", e.Name)
	case *ListLiteral:
		// Static list
		result := make([]any, len(e.Elems))
		for i, elem := range e.Elems {
			val, err := exprToAny(elem)
			if err != nil {
				return nil, err
			}
			result[i] = val
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported foreach range expression type %T", expr)
	}
}

// evaluatePrepareBlock evaluates a prepare block and returns the bindings.
func evaluatePrepareBlock(prepare *PrepareDecl, inputEnv map[string]Expr) (map[string]any, error) {
	result := make(map[string]any)
	for _, binding := range prepare.Bindings {
		val, err := evaluateCtrlExpr(binding.Expr, inputEnv)
		if err != nil {
			return nil, fmt.Errorf("evaluating %s: %w", binding.Name, err)
		}
		result[binding.Name] = val
	}
	return result, nil
}

// evaluateCtrlExpr evaluates a controller-side expression.
func evaluateCtrlExpr(expr Expr, inputEnv map[string]Expr) (any, error) {
	switch e := expr.(type) {
	case *FunctionCall:
		return evaluateCtrlFunction(e, inputEnv)
	case *StringLiteral:
		return e.Value, nil
	case *BoolLiteral:
		return e.Value, nil
	case *NumberLiteral:
		return e.Value, nil
	case *Ident:
		if val, ok := inputEnv[e.Name]; ok {
			return exprToAny(val)
		}
		// Check for "inputs.X" pattern
		if len(e.Name) > 7 && e.Name[:7] == "inputs." {
			inputName := e.Name[7:]
			if val, ok := inputEnv[inputName]; ok {
				return exprToAny(val)
			}
		}
		return nil, fmt.Errorf("undefined variable %q", e.Name)
	default:
		return nil, fmt.Errorf("unsupported controller expression type %T", expr)
	}
}

// evaluateCtrlFunction evaluates a _ctrl.* function call.
func evaluateCtrlFunction(fc *FunctionCall, inputEnv map[string]Expr) (any, error) {
	switch fc.Name {
	case "_ctrl.readdir":
		if len(fc.Args) != 1 {
			return nil, fmt.Errorf("_ctrl.readdir requires exactly 1 argument")
		}
		pathVal, err := evaluateCtrlExpr(fc.Args[0], inputEnv)
		if err != nil {
			return nil, err
		}
		pathStr, ok := pathVal.(string)
		if !ok {
			return nil, fmt.Errorf("_ctrl.readdir argument must be a string, got %T", pathVal)
		}
		return ctrlReaddir(pathStr)

	case "_ctrl.read":
		if len(fc.Args) != 1 {
			return nil, fmt.Errorf("_ctrl.read requires exactly 1 argument")
		}
		pathVal, err := evaluateCtrlExpr(fc.Args[0], inputEnv)
		if err != nil {
			return nil, err
		}
		pathStr, ok := pathVal.(string)
		if !ok {
			return nil, fmt.Errorf("_ctrl.read argument must be a string, got %T", pathVal)
		}
		return ctrlRead(pathStr)

	case "_ctrl.sha256":
		if len(fc.Args) != 1 {
			return nil, fmt.Errorf("_ctrl.sha256 requires exactly 1 argument")
		}
		pathVal, err := evaluateCtrlExpr(fc.Args[0], inputEnv)
		if err != nil {
			return nil, err
		}
		pathStr, ok := pathVal.(string)
		if !ok {
			return nil, fmt.Errorf("_ctrl.sha256 argument must be a string, got %T", pathVal)
		}
		return ctrlSha256(pathStr)

	default:
		return nil, fmt.Errorf("unknown controller function %q", fc.Name)
	}
}

// exprToAny converts an Expr to its runtime value.
func exprToAny(expr Expr) (any, error) {
	switch e := expr.(type) {
	case *StringLiteral:
		return e.Value, nil
	case *BoolLiteral:
		return e.Value, nil
	case *NumberLiteral:
		return e.Value, nil
	case *ListLiteral:
		result := make([]any, len(e.Elems))
		for i, elem := range e.Elems {
			val, err := exprToAny(elem)
			if err != nil {
				return nil, err
			}
			result[i] = val
		}
		return result, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to runtime value", expr)
	}
}

// anyToExpr converts a runtime value to an Expr.
func anyToExpr(v any) Expr {
	switch val := v.(type) {
	case string:
		return &StringLiteral{Value: val}
	case bool:
		return &BoolLiteral{Value: val}
	case int:
		return &NumberLiteral{Value: val}
	case map[string]any:
		// For file entries from _ctrl.readdir, we treat the whole map as available
		// via dot access. For simplicity, we'll create a special identifier.
		return &MapLiteral{Value: val}
	case []any:
		elems := make([]Expr, len(val))
		for i, ev := range val {
			elems[i] = anyToExpr(ev)
		}
		return &ListLiteral{Elems: elems}
	default:
		return &StringLiteral{Value: fmt.Sprintf("%v", v)}
	}
}

// substituteVarInString substitutes ${varName} and direct var references in a string.
func substituteVarInString(s string, varName string, value any) string {
	// Handle ${varName} syntax
	result := s
	placeholder := "${" + varName + "}"
	if valStr, ok := value.(string); ok {
		for {
			idx := indexOf(result, placeholder)
			if idx == -1 {
				break
			}
			result = result[:idx] + valStr + result[idx+len(placeholder):]
		}
	}
	// Handle ${varName.field} syntax for maps
	if m, ok := value.(map[string]any); ok {
		for fieldName, fieldVal := range m {
			fieldPlaceholder := "${" + varName + "." + fieldName + "}"
			if fieldStr, ok := fieldVal.(string); ok {
				for {
					idx := indexOf(result, fieldPlaceholder)
					if idx == -1 {
						break
					}
					result = result[:idx] + fieldStr + result[idx+len(fieldPlaceholder):]
				}
			}
		}
	}
	return result
}

func substituteInputsInExpr(expr Expr, inputEnv map[string]Expr) Expr {
	switch e := expr.(type) {
	case *Ident:
		// Check for "inputs.X" pattern
		if len(e.Name) > 7 && e.Name[:7] == "inputs." {
			inputName := e.Name[7:]
			if val, ok := inputEnv[inputName]; ok {
				return substituteInputsInExpr(val, inputEnv)
			}
		}
		// Check for "varname.fieldname" pattern where varname is a MapLiteral
		if dotIdx := indexOfRune(e.Name, '.'); dotIdx > 0 {
			varName := e.Name[:dotIdx]
			fieldName := e.Name[dotIdx+1:]
			if val, ok := inputEnv[varName]; ok {
				if mapLit, ok := val.(*MapLiteral); ok {
					if fieldVal, ok := mapLit.Value[fieldName]; ok {
						return anyToExpr(fieldVal)
					}
				}
			}
		}
		// Direct input reference
		if val, ok := inputEnv[e.Name]; ok {
			return val
		}
		return e
	case *MapLiteral:
		// MapLiterals are passed through as-is
		return e
	case *BinaryExpr:
		return &BinaryExpr{
			Left:    substituteInputsInExpr(e.Left, inputEnv),
			Op:      e.Op,
			Right:   substituteInputsInExpr(e.Right, inputEnv),
			PosInfo: e.PosInfo,
		}
	case *TernaryExpr:
		return &TernaryExpr{
			Cond:      substituteInputsInExpr(e.Cond, inputEnv),
			TrueExpr:  substituteInputsInExpr(e.TrueExpr, inputEnv),
			FalseExpr: substituteInputsInExpr(e.FalseExpr, inputEnv),
			PosInfo:   e.PosInfo,
		}
	case *ListLiteral:
		newElems := make([]Expr, len(e.Elems))
		for i, elem := range e.Elems {
			newElems[i] = substituteInputsInExpr(elem, inputEnv)
		}
		return &ListLiteral{
			Elems:   newElems,
			PosInfo: e.PosInfo,
		}
	case *FunctionCall:
		// Substitute arguments in function calls
		newArgs := make([]Expr, len(e.Args))
		for i, arg := range e.Args {
			newArgs[i] = substituteInputsInExpr(arg, inputEnv)
		}
		// If this is a _ctrl.* function, evaluate it now
		if len(e.Name) > 6 && e.Name[:6] == "_ctrl." {
			newCall := &FunctionCall{
				Name:    e.Name,
				Args:    newArgs,
				PosInfo: e.PosInfo,
			}
			result, err := evaluateCtrlFunction(newCall, inputEnv)
			if err == nil {
				return anyToExpr(result)
			}
			// If evaluation fails, fall through to return the FunctionCall
		}
		return &FunctionCall{
			Name:    e.Name,
			Args:    newArgs,
			PosInfo: e.PosInfo,
		}
	default:
		return expr
	}
}

// indexOfRune returns the index of the first occurrence of rune r in s, or -1.
func indexOfRune(s string, r rune) int {
	for i, c := range s {
		if c == r {
			return i
		}
	}
	return -1
}

func literalToExpr(v any) Expr {
	switch val := v.(type) {
	case string:
		return &StringLiteral{Value: val}
	case bool:
		return &BoolLiteral{Value: val}
	case int:
		return &NumberLiteral{Value: val}
	case []any:
		elems := make([]Expr, len(val))
		for i, ev := range val {
			elems[i] = literalToExpr(ev)
		}
		return &ListLiteral{Elems: elems}
	default:
		return nil
	}
}

// serializeProbeDecl converts a ProbeDecl to a map suitable for JSON serialization.
// It handles FunctionCall expressions and substitutes input references.
func serializeProbeDecl(probe *ProbeDecl, inputEnv map[string]Expr) map[string]any {
	result := make(map[string]any)
	for _, field := range probe.Fields {
		result[field.Name] = serializeProbeExpr(field.Expr, inputEnv)
	}
	return result
}

// serializeDesiredDecl converts a DesiredDecl to a map suitable for JSON serialization.
// It handles literal values and input references.
func serializeDesiredDecl(desired *DesiredDecl, inputEnv map[string]Expr) map[string]any {
	result := make(map[string]any)
	for _, field := range desired.Fields {
		result[field.Name] = serializeDesiredExpr(field.Expr, inputEnv)
	}
	return result
}

// serializeProbeExpr converts a probe expression to a serializable form.
// Function calls are serialized as {"func": "name", "args": [...]}.
// Input references like "inputs.path" are resolved from inputEnv.
func serializeProbeExpr(expr Expr, inputEnv map[string]Expr) any {
	switch e := expr.(type) {
	case *FunctionCall:
		// Serialize function call as {"func": "name", "args": [...]}
		args := make([]any, len(e.Args))
		for i, arg := range e.Args {
			args[i] = serializeProbeExpr(arg, inputEnv)
		}
		return map[string]any{
			"func": e.Name,
			"args": args,
		}
	case *Ident:
		// Check for "inputs.X" pattern
		if len(e.Name) > 7 && e.Name[:7] == "inputs." {
			inputName := e.Name[7:]
			if val, ok := inputEnv[inputName]; ok {
				return serializeProbeExpr(val, inputEnv)
			}
		}
		// Direct input reference
		if val, ok := inputEnv[e.Name]; ok {
			return serializeProbeExpr(val, inputEnv)
		}
		// Return as identifier reference for late binding
		return map[string]any{"ref": e.Name}
	case *StringLiteral:
		return e.Value
	case *BoolLiteral:
		return e.Value
	case *NumberLiteral:
		return e.Value
	case *ListLiteral:
		elems := make([]any, len(e.Elems))
		for i, elem := range e.Elems {
			elems[i] = serializeProbeExpr(elem, inputEnv)
		}
		return elems
	default:
		return nil
	}
}

// serializeDesiredExpr converts a desired expression to a serializable form.
// Input references like "inputs.path" are resolved from inputEnv.
func serializeDesiredExpr(expr Expr, inputEnv map[string]Expr) any {
	switch e := expr.(type) {
	case *FunctionCall:
		// Desired can also have function calls (e.g., sha256_file)
		args := make([]any, len(e.Args))
		for i, arg := range e.Args {
			args[i] = serializeDesiredExpr(arg, inputEnv)
		}
		return map[string]any{
			"func": e.Name,
			"args": args,
		}
	case *Ident:
		// Check for "inputs.X" pattern
		if len(e.Name) > 7 && e.Name[:7] == "inputs." {
			inputName := e.Name[7:]
			if val, ok := inputEnv[inputName]; ok {
				return serializeDesiredExpr(val, inputEnv)
			}
		}
		// Direct input reference
		if val, ok := inputEnv[e.Name]; ok {
			return serializeDesiredExpr(val, inputEnv)
		}
		// Return as identifier reference
		return map[string]any{"ref": e.Name}
	case *StringLiteral:
		return e.Value
	case *BoolLiteral:
		return e.Value
	case *NumberLiteral:
		return e.Value
	case *ListLiteral:
		elems := make([]any, len(e.Elems))
		for i, elem := range e.Elems {
			elems[i] = serializeDesiredExpr(elem, inputEnv)
		}
		return elems
	default:
		return nil
	}
}

// ctrlReaddir lists files in a directory on the controller.
// Returns a list of maps with file metadata: name, relative_path, absolute_path, is_dir, size, mode.
func ctrlReaddir(path string) ([]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", path)
	}

	var result []any
	err = filepath.WalkDir(path, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip the root directory itself
		if filePath == path {
			return nil
		}
		// Skip directories (we only sync files)
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(path, filePath)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		fileEntry := map[string]any{
			"name":          d.Name(),
			"relative_path": relPath,
			"absolute_path": filePath,
			"is_dir":        d.IsDir(),
			"size":          info.Size(),
			"mode":          fmt.Sprintf("%04o", info.Mode().Perm()),
		}
		result = append(result, fileEntry)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", path, err)
	}

	return result, nil
}

// ctrlRead reads a file from the controller filesystem.
// Returns the file content as a string.
func ctrlRead(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return string(data), nil
}

// ctrlSha256 computes the SHA256 hash of a file on the controller.
func ctrlSha256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// LowerToPlanV2_0 extends v1.2 lowering with user-defined function support.
func LowerToPlanV2_0(file *File, lets LetEnv, steps map[string]*StepDecl, forLoops []*ForDecl, fleets map[string]*FleetDecl, primitives map[string]*PrimitiveDecl, funcs map[string]*FnDecl) (*plan.Plan, error) {
	// First, expand any function calls in let bindings
	expandedLets := make(LetEnv)
	for name, expr := range lets {
		expandedLets[name] = expandFunctionCalls(expr, funcs)
	}

	// Expand function calls in node inputs
	for _, decl := range file.Decls {
		if nodeDecl, ok := decl.(*NodeDecl); ok {
			for key, expr := range nodeDecl.Inputs {
				nodeDecl.Inputs[key] = expandFunctionCalls(expr, funcs)
			}
		}
	}

	// Now proceed with v1.2 lowering
	return LowerToPlanV1_2(file, expandedLets, steps, forLoops, fleets, primitives)
}

// expandFunctionCalls recursively expands user-defined function calls in an expression.
func expandFunctionCalls(expr Expr, funcs map[string]*FnDecl) Expr {
	switch e := expr.(type) {
	case *FunctionCall:
		// Check if this is a user-defined function
		if fn, ok := funcs[e.Name]; ok {
			// Build parameter substitution map
			paramEnv := make(map[string]Expr)
			for i, paramName := range fn.Params {
				if i < len(e.Args) {
					paramEnv[paramName] = expandFunctionCalls(e.Args[i], funcs)
				}
			}
			// Substitute parameters in function body
			return substituteFnParamsInExpr(fn.Body, paramEnv, funcs)
		}
		// Not a user-defined function, just expand the arguments
		newArgs := make([]Expr, len(e.Args))
		for i, arg := range e.Args {
			newArgs[i] = expandFunctionCalls(arg, funcs)
		}
		return &FunctionCall{
			Name:    e.Name,
			Args:    newArgs,
			PosInfo: e.PosInfo,
		}

	case *BinaryExpr:
		return &BinaryExpr{
			Left:    expandFunctionCalls(e.Left, funcs),
			Op:      e.Op,
			Right:   expandFunctionCalls(e.Right, funcs),
			PosInfo: e.PosInfo,
		}

	case *TernaryExpr:
		return &TernaryExpr{
			Cond:      expandFunctionCalls(e.Cond, funcs),
			TrueExpr:  expandFunctionCalls(e.TrueExpr, funcs),
			FalseExpr: expandFunctionCalls(e.FalseExpr, funcs),
			PosInfo:   e.PosInfo,
		}

	case *ListLiteral:
		newElems := make([]Expr, len(e.Elems))
		for i, elem := range e.Elems {
			newElems[i] = expandFunctionCalls(elem, funcs)
		}
		return &ListLiteral{
			Elems:   newElems,
			PosInfo: e.PosInfo,
		}

	case *Ident:
		// Check if this is a function call without parentheses (e.g., just the function name)
		if fn, ok := funcs[e.Name]; ok && len(fn.Params) == 0 {
			// Zero-argument function can be called just by name
			return expandFunctionCalls(fn.Body, funcs)
		}
		return e

	default:
		return expr
	}
}

// substituteFnParamsInExpr substitutes parameter references in a function body expression.
func substituteFnParamsInExpr(expr Expr, paramEnv map[string]Expr, funcs map[string]*FnDecl) Expr {
	switch e := expr.(type) {
	case *Ident:
		// Check if this identifier is a parameter
		if val, ok := paramEnv[e.Name]; ok {
			return val
		}
		return e

	case *FunctionCall:
		// First substitute in arguments
		newArgs := make([]Expr, len(e.Args))
		for i, arg := range e.Args {
			newArgs[i] = substituteFnParamsInExpr(arg, paramEnv, funcs)
		}
		// Then expand if it's a user-defined function
		newCall := &FunctionCall{
			Name:    e.Name,
			Args:    newArgs,
			PosInfo: e.PosInfo,
		}
		return expandFunctionCalls(newCall, funcs)

	case *BinaryExpr:
		return &BinaryExpr{
			Left:    substituteFnParamsInExpr(e.Left, paramEnv, funcs),
			Op:      e.Op,
			Right:   substituteFnParamsInExpr(e.Right, paramEnv, funcs),
			PosInfo: e.PosInfo,
		}

	case *TernaryExpr:
		return &TernaryExpr{
			Cond:      substituteFnParamsInExpr(e.Cond, paramEnv, funcs),
			TrueExpr:  substituteFnParamsInExpr(e.TrueExpr, paramEnv, funcs),
			FalseExpr: substituteFnParamsInExpr(e.FalseExpr, paramEnv, funcs),
			PosInfo:   e.PosInfo,
		}

	case *ListLiteral:
		newElems := make([]Expr, len(e.Elems))
		for i, elem := range e.Elems {
			newElems[i] = substituteFnParamsInExpr(elem, paramEnv, funcs)
		}
		return &ListLiteral{
			Elems:   newElems,
			PosInfo: e.PosInfo,
		}

	default:
		return expr
	}
}
