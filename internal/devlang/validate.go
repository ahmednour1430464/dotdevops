package devlang

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"devopsctl/internal/plan"
)

// SemanticError represents a language-level (v0.1) semantic error.
type SemanticError struct {
	Path string
	Pos  Position
	Msg  string
}

func (e *SemanticError) Error() string {
	return fmt.Sprintf("%s:%d:%d: error: %s", e.Path, e.Pos.Line, e.Pos.Col, e.Msg)
}

type LetEnv map[string]Expr

// ValidateV0_2 enforces the v0.2 language rules on an AST file and returns the collected let environment.
func ValidateV0_2(file *File) ([]error, LetEnv) {
	var errs []error
	lets := LetEnv{}

	// 1. Reject unsupported constructs outright (lets are allowed in v0.2).
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ForDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "for expressions are not supported in language version 0.2",
			})
		case *StepDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "steps are not supported in language version 0.2",
			})
		case *ModuleDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "modules are not supported in language version 0.2",
			})
		}
	}

	if len(errs) > 0 {
		return errs, nil
	}

	// 2. Collect let bindings and build the environment.
	for _, decl := range file.Decls {
		letDecl, ok := decl.(*LetDecl)
		if !ok {
			continue
		}

		if _, exists := lets[letDecl.Name]; exists {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  letDecl.Pos(),
				Msg:  fmt.Sprintf("duplicate let %q", letDecl.Name),
			})
			continue
		}

		switch v := letDecl.Value.(type) {
		case *StringLiteral, *BoolLiteral:
			lets[letDecl.Name] = letDecl.Value
		case *ListLiteral:
			if !allStringLits(v.Elems) {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  letDecl.Value.Pos(),
					Msg:  fmt.Sprintf("let %q value must be a string, bool, or list of string literals", letDecl.Name),
				})
			} else {
				lets[letDecl.Name] = letDecl.Value
			}
		default:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  letDecl.Value.Pos(),
				Msg:  fmt.Sprintf("let %q value must be a string, bool, or list of string literals", letDecl.Name),
			})
		}
	}

	if len(errs) > 0 {
		return errs, nil
	}

	// 3. Build symbol tables for targets and nodes.
	targets := map[string]*TargetDecl{}
	nodes := map[string]*NodeDecl{}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *TargetDecl:
			if _, exists := targets[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate target %q", d.Name),
				})
			} else {
				targets[d.Name] = d
			}
		case *NodeDecl:
			if _, exists := nodes[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate node %q", d.Name),
				})
			} else {
				nodes[d.Name] = d
			}
		}
	}

	// 4. Per-node checks.
	for _, node := range nodes {
		// targets must exist and must not be let bindings
		for _, tIdent := range node.Targets {
			if _, isLet := lets[tIdent.Name]; isLet {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("let binding %q cannot be used in targets; targets must reference target declarations", tIdent.Name),
				})
				continue
			}
			if _, ok := targets[tIdent.Name]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("unknown target %q", tIdent.Name),
				})
			}
		}

		// depends_on by node IDs
		for _, dep := range node.DependsOn {
			if _, ok := nodes[dep.Value]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  dep.Pos(),
					Msg:  fmt.Sprintf("unknown depends_on node %q", dep.Value),
				})
			}
		}

		// primitive type
		switch node.Type.Name {
		case "file.sync", "process.exec", "template.render", "health.check", "service.ensure", "package.install":
			// ok
		default:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.Type.Pos(),
				Msg:  fmt.Sprintf("unknown primitive type %q", node.Type.Name),
			})
		}

		// failure_policy
		if node.FailurePolicy != nil {
			fp := node.FailurePolicy.Name
			if fp != "halt" && fp != "continue" && fp != "rollback" {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  node.FailurePolicy.Pos(),
					Msg:  fmt.Sprintf("invalid failure_policy %q; expected one of: halt, continue, rollback", fp),
				})
			}
		}

		// validate primitive-specific inputs after resolving lets in value position
		resolvedNode := *node
		resolvedNode.Inputs = make(map[string]Expr, len(node.Inputs))
		for key, expr := range node.Inputs {
			resolvedNode.Inputs[key] = resolveLetExpr(expr, lets)
		}

		validatePrimitiveInputsV0_1(file.Path, &resolvedNode, &errs)
	}

	return errs, lets
}

// ValidateV0_1 enforces the v0.1 language rules on an AST file.
func ValidateV0_1(file *File) []error {
	var errs []error

	// 1. Reject unsupported constructs outright.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *LetDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "let bindings are not supported in language version 0.1",
			})
		case *ForDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "for expressions are not supported in language version 0.1",
			})
		case *StepDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "steps are not supported in language version 0.1",
			})
		case *ModuleDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "modules are not supported in language version 0.1",
			})
		}
	}

	if len(errs) > 0 {
		return errs
	}

	// 2. Build symbol tables for targets and nodes.
	targets := map[string]*TargetDecl{}
	nodes := map[string]*NodeDecl{}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *TargetDecl:
			if _, exists := targets[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate target %q", d.Name),
				})
			} else {
				targets[d.Name] = d
			}
		case *NodeDecl:
			if _, exists := nodes[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate node %q", d.Name),
				})
			} else {
				nodes[d.Name] = d
			}
		}
	}

	// 3. Per-node checks.
	for _, node := range nodes {
		// targets must exist
		for _, tIdent := range node.Targets {
			if _, ok := targets[tIdent.Name]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("unknown target %q", tIdent.Name),
				})
			}
		}

		// depends_on by node IDs
		for _, dep := range node.DependsOn {
			if _, ok := nodes[dep.Value]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  dep.Pos(),
					Msg:  fmt.Sprintf("unknown depends_on node %q", dep.Value),
				})
			}
		}

		// primitive type
		switch node.Type.Name {
		case "file.sync", "process.exec", "template.render", "health.check", "service.ensure", "package.install":
			// ok
		default:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.Type.Pos(),
				Msg:  fmt.Sprintf("unknown primitive type %q", node.Type.Name),
			})
		}

		// failure_policy
		if node.FailurePolicy != nil {
			fp := node.FailurePolicy.Name
			if fp != "halt" && fp != "continue" && fp != "rollback" {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  node.FailurePolicy.Pos(),
					Msg:  fmt.Sprintf("invalid failure_policy %q; expected one of: halt, continue, rollback", fp),
				})
			}
		}

		validatePrimitiveInputsV0_1(file.Path, node, &errs)
	}

	return errs
}

func validatePrimitiveInputsV0_1(path string, node *NodeDecl, errs *[]error) {
	switch node.Type.Name {
	case "file.sync":
		// src and dest must be string literals
		srcExpr, ok := node.Inputs["src"]
		if !ok {
			*errs = append(*errs, &SemanticError{
				Path: path,
				Pos:  node.Pos(),
				Msg:  "file.sync requires attribute src",
			})
		} else if _, ok := srcExpr.(*StringLiteral); !ok {
			*errs = append(*errs, &SemanticError{
				Path: path,
				Pos:  srcExpr.Pos(),
				Msg:  "file.sync src must be a string literal",
			})
		}

		destExpr, ok := node.Inputs["dest"]
		if !ok {
			*errs = append(*errs, &SemanticError{
				Path: path,
				Pos:  node.Pos(),
				Msg:  "file.sync requires attribute dest",
			})
		} else if _, ok := destExpr.(*StringLiteral); !ok {
			*errs = append(*errs, &SemanticError{
				Path: path,
				Pos:  destExpr.Pos(),
				Msg:  "file.sync dest must be a string literal",
			})
		}

	case "process.exec":
		cmdExpr, ok := node.Inputs["cmd"]
		if !ok {
			*errs = append(*errs, &SemanticError{
				Path: path,
				Pos:  node.Pos(),
				Msg:  "process.exec requires attribute cmd",
			})
		} else if list, ok := cmdExpr.(*ListLiteral); !ok || !allStringLits(list.Elems) {
			*errs = append(*errs, &SemanticError{
				Path: path,
				Pos:  cmdExpr.Pos(),
				Msg:  "process.exec cmd must be a non-empty list of string literals",
			})
		}

		cwdExpr, ok := node.Inputs["cwd"]
		if !ok {
			*errs = append(*errs, &SemanticError{
				Path: path,
				Pos:  node.Pos(),
				Msg:  "process.exec requires attribute cwd",
			})
		} else if _, ok := cwdExpr.(*StringLiteral); !ok {
			*errs = append(*errs, &SemanticError{
				Path: path,
				Pos:  cwdExpr.Pos(),
				Msg:  "process.exec cwd must be a string literal",
			})
		}
	}
}

func allStringLits(elems []Expr) bool {
	if len(elems) == 0 {
		return false
	}
	for _, e := range elems {
		if _, ok := e.(*StringLiteral); !ok {
			return false
		}
	}
	return true
}

// resolveLetExpr resolves an identifier expression to its let-bound value if present.
// Non-identifier expressions are returned unchanged.
func resolveLetExpr(e Expr, lets LetEnv) Expr {
	if ident, ok := e.(*Ident); ok {
		if lets == nil {
			return e
		}
		if v, ok := lets[ident.Name]; ok {
			return v
		}
	}
	return e
}

// CompileResult is the high-level result of compiling a .devops file.
type CompileResult struct {
	Plan    *plan.Plan
	RawJSON []byte
	Errors  []error
}

// ExtractVersion returns the self-declared language version from the file, or "" if not found.
func ExtractVersion(file *File) string {
	if len(file.Decls) > 0 {
		if v, ok := file.Decls[0].(*VersionDecl); ok {
			return v.Version
		}
	}
	return ""
}

// CompileFileAutoDetect detects the file version and dispatches to the correct compiler.
func CompileFileAutoDetect(path string, src []byte, defaultVersion string) (*CompileResult, error) {
	file, errs := ParseFile(path, src)
	if len(errs) > 0 {
		return &CompileResult{Errors: errs}, nil
	}

	version := ExtractVersion(file)
	if version == "" {
		version = defaultVersion
	}

	switch version {
	case "v0.1":
		return CompileFileV0_1(path, src)
	case "v0.2":
		return CompileFileV0_2(path, src)
	case "v0.3":
		return CompileFileV0_3(path, src)
	case "v0.4":
		return CompileFileV0_4(path, src)
	case "v0.5":
		return CompileFileV0_5(path, src)
	case "v0.6":
		return CompileFileV0_6(path, src)
	// For v0.7 to v0.9 we'll route to the latest compiler (CompileFileV0_8)
	case "v0.7", "v0.8", "v0.9":
		return CompileFileV0_8(path, src)
	// v1.2+ supports custom primitives, probe/desired (v1.3+), prepare/foreach (v1.4+)
	case "v1.2", "v1.3", "v1.4":
		return CompileFileV1_2(path, src)
	// v2.0+ supports imports and user-defined functions
	case "v2.0":
		return CompileFileV2_0(path, src)
	default:
		return &CompileResult{Errors: []error{fmt.Errorf("%s: unknown language version %q", path, version)}}, nil
	}
}

// CompileFileV0_1 runs parse, validate, lower, and IR validation.
func CompileFileV0_1(path string, src []byte) (*CompileResult, error) {
	file, parseErrs := ParseFile(path, src)
	if len(parseErrs) > 0 {
		return &CompileResult{Errors: parseErrs}, nil
	}

	semErrs := ValidateV0_1(file)
	if len(semErrs) > 0 {
		return &CompileResult{Errors: semErrs}, nil
	}

	p, err := LowerToPlan(file)
	if err != nil {
		return nil, err
	}

	// IR-level validation using existing plan.Validate
	if vErrs := plan.Validate(p); len(vErrs) > 0 {
		errs := make([]error, len(vErrs))
		for i, e := range vErrs {
			errs[i] = fmt.Errorf("%s: error: %v", path, e)
		}
		return &CompileResult{Errors: errs}, nil
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}

	return &CompileResult{
		Plan:    p,
		RawJSON: raw,
		Errors:  nil,
	}, nil
}

// CompileFileV0_2 runs parse, v0.2 validate, lower with lets, and IR validation.
func CompileFileV0_2(path string, src []byte) (*CompileResult, error) {
	file, parseErrs := ParseFile(path, src)
	if len(parseErrs) > 0 {
		return &CompileResult{Errors: parseErrs}, nil
	}

	semErrs, lets := ValidateV0_2(file)
	if len(semErrs) > 0 {
		return &CompileResult{Errors: semErrs}, nil
	}

	p, err := LowerToPlanV0_2(file, lets)
	if err != nil {
		return nil, err
	}

	// IR-level validation using existing plan.Validate
	if vErrs := plan.Validate(p); len(vErrs) > 0 {
		errs := make([]error, len(vErrs))
		for i, e := range vErrs {
			errs[i] = fmt.Errorf("%s: error: %v", path, e)
		}
		return &CompileResult{Errors: errs}, nil
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}

	return &CompileResult{
		Plan:    p,
		RawJSON: raw,
		Errors:  nil,
	}, nil
}

// ValidateV0_3 enforces the v0.3 language rules with expression support.
func ValidateV0_3(file *File) ([]error, LetEnv) {
	var errs []error
	lets := LetEnv{}

	// 1. Reject unsupported constructs outright (lets are allowed in v0.3).
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ForDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "for expressions are not supported in language version 0.3",
			})
		case *StepDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "steps are not supported in language version 0.3",
			})
		case *ModuleDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "modules are not supported in language version 0.3",
			})
		}
	}

	if len(errs) > 0 {
		return errs, nil
	}

	// 2. Collect let bindings (with expressions, not yet evaluated).
	for _, decl := range file.Decls {
		letDecl, ok := decl.(*LetDecl)
		if !ok {
			continue
		}

		if _, exists := lets[letDecl.Name]; exists {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  letDecl.Pos(),
				Msg:  fmt.Sprintf("duplicate let %q", letDecl.Name),
			})
			continue
		}

		lets[letDecl.Name] = letDecl.Value
	}

	if len(errs) > 0 {
		return errs, nil
	}

	// 3. Type check all let expressions.
	for name, expr := range lets {
		_, err := typeCheckExpr(expr, lets, file.Path)
		if err != nil {
			errs = append(errs, err)
		}
		// Find the original let decl for better error positioning
		_ = name // unused for now
	}

	if len(errs) > 0 {
		return errs, nil
	}

	// 4. Evaluate all let expressions to literals (constant folding).
	evaluatedLets := LetEnv{}
	for name, expr := range lets {
		evaluated, err := evaluateExpr(expr, lets, file.Path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		evaluatedLets[name] = evaluated
	}

	if len(errs) > 0 {
		return errs, nil
	}

	// Replace lets with evaluated versions
	lets = evaluatedLets

	// 5. Build symbol tables for targets and nodes.
	targets := map[string]*TargetDecl{}
	nodes := map[string]*NodeDecl{}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *TargetDecl:
			if _, exists := targets[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate target %q", d.Name),
				})
			} else {
				targets[d.Name] = d
			}
		case *NodeDecl:
			if _, exists := nodes[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate node %q", d.Name),
				})
			} else {
				nodes[d.Name] = d
			}
		}
	}

	// 6. Per-node checks.
	for _, node := range nodes {
		// targets must exist and must not be let bindings
		for _, tIdent := range node.Targets {
			if _, isLet := lets[tIdent.Name]; isLet {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("let binding %q cannot be used in targets; targets must reference target declarations", tIdent.Name),
				})
				continue
			}
			if _, ok := targets[tIdent.Name]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("unknown target %q", tIdent.Name),
				})
			}
		}

		// depends_on by node IDs
		for _, dep := range node.DependsOn {
			if _, ok := nodes[dep.Value]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  dep.Pos(),
					Msg:  fmt.Sprintf("unknown depends_on node %q", dep.Value),
				})
			}
		}

		// primitive type
		switch node.Type.Name {
		case "file.sync", "process.exec", "template.render", "health.check", "service.ensure", "package.install":
			// ok
		default:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.Type.Pos(),
				Msg:  fmt.Sprintf("unknown primitive type %q", node.Type.Name),
			})
		}

		// failure_policy
		if node.FailurePolicy != nil {
			fp := node.FailurePolicy.Name
			if fp != "halt" && fp != "continue" && fp != "rollback" {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  node.FailurePolicy.Pos(),
					Msg:  fmt.Sprintf("invalid failure_policy %q; expected one of: halt, continue, rollback", fp),
				})
			}
		}

		// validate primitive-specific inputs after resolving lets in value position
		resolvedNode := *node
		resolvedNode.Inputs = make(map[string]Expr, len(node.Inputs))
		for key, expr := range node.Inputs {
			resolvedNode.Inputs[key] = resolveLetExpr(expr, lets)
		}

		validatePrimitiveInputsV0_1(file.Path, &resolvedNode, &errs)
	}

	return errs, lets
}

// CompileFileV0_3 runs parse, v0.3 validate, lower with evaluated lets, and IR validation.
func CompileFileV0_3(path string, src []byte) (*CompileResult, error) {
	file, parseErrs := ParseFile(path, src)
	if len(parseErrs) > 0 {
		return &CompileResult{Errors: parseErrs}, nil
	}

	semErrs, lets := ValidateV0_3(file)
	if len(semErrs) > 0 {
		return &CompileResult{Errors: semErrs}, nil
	}

	p, err := LowerToPlanV0_2(file, lets)
	if err != nil {
		return nil, err
	}

	// IR-level validation using existing plan.Validate
	if vErrs := plan.Validate(p); len(vErrs) > 0 {
		errs := make([]error, len(vErrs))
		for i, e := range vErrs {
			errs[i] = fmt.Errorf("%s: error: %v", path, e)
		}
		return &CompileResult{Errors: errs}, nil
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}

	return &CompileResult{
		Plan:    p,
		RawJSON: raw,
		Errors:  nil,
	}, nil
}

// ValidateV0_4 enforces the v0.4 language rules with reusable step support.
func ValidateV0_4(file *File) ([]error, LetEnv, map[string]*StepDecl) {
	var errs []error
	lets := LetEnv{}
	steps := map[string]*StepDecl{}

	// Known primitive types for collision detection
	primitiveTypes := map[string]bool{
		"file.sync":       true,
		"process.exec":    true,
		"template.render": true,
		"health.check":    true,
		"service.ensure":  true,
	}

	// 1. Reject unsupported constructs (for and module still not supported in v0.4).
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ForDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "for expressions are not supported in language version 0.4",
			})
		case *ModuleDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "modules are not supported in language version 0.4",
			})
		}
	}

	if len(errs) > 0 {
		return errs, nil, nil
	}

	// 2. Collect and validate let bindings (with expressions, not yet evaluated).
	for _, decl := range file.Decls {
		letDecl, ok := decl.(*LetDecl)
		if !ok {
			continue
		}

		if _, exists := lets[letDecl.Name]; exists {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  letDecl.Pos(),
				Msg:  fmt.Sprintf("duplicate let %q", letDecl.Name),
			})
			continue
		}

		lets[letDecl.Name] = letDecl.Value
	}

	if len(errs) > 0 {
		return errs, nil, nil
	}

	// 3. Type check all let expressions.
	for name, expr := range lets {
		_, err := typeCheckExpr(expr, lets, file.Path)
		if err != nil {
			errs = append(errs, err)
		}
		_ = name
	}

	if len(errs) > 0 {
		return errs, nil, nil
	}

	// 4. Evaluate all let expressions to literals (constant folding).
	evaluatedLets := LetEnv{}
	for name, expr := range lets {
		evaluated, err := evaluateExpr(expr, lets, file.Path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		evaluatedLets[name] = evaluated
	}

	if len(errs) > 0 {
		return errs, nil, nil
	}

	lets = evaluatedLets

	// 5. Collect and validate step definitions.
	for _, decl := range file.Decls {
		stepDecl, ok := decl.(*StepDecl)
		if !ok {
			continue
		}

		// Check for duplicate step names
		if _, exists := steps[stepDecl.Name]; exists {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("duplicate step %q", stepDecl.Name),
			})
			continue
		}

		// Check for primitive name collision
		if primitiveTypes[stepDecl.Name] {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step name %q conflicts with built-in primitive", stepDecl.Name),
			})
			continue
		}

		body := stepDecl.Body

		// Steps must NOT specify targets
		if len(body.Targets) > 0 {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step %q must not specify targets (targets belong to node instantiations)", stepDecl.Name),
			})
		}

		// Steps must NOT specify depends_on
		if len(body.DependsOn) > 0 {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step %q must not specify depends_on (graph structure belongs to nodes)", stepDecl.Name),
			})
		}

		// Steps must have a type
		if body.Type == nil {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step %q must specify a type", stepDecl.Name),
			})
			continue
		}

		// Step type must be a known primitive, not another step
		stepType := body.Type.Name
		if !primitiveTypes[stepType] {
			// Check if it references another step (forbidden in v0.4)
			if _, isStep := steps[stepType]; isStep {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  body.Type.Pos(),
					Msg:  fmt.Sprintf("step %q cannot reference step %q (nested steps are not supported in v0.4)", stepDecl.Name, stepType),
				})
			} else {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  body.Type.Pos(),
					Msg:  fmt.Sprintf("step %q has unknown primitive type %q", stepDecl.Name, stepType),
				})
			}
			continue
		}

		// Validate failure_policy if present
		if body.FailurePolicy != nil {
			fp := body.FailurePolicy.Name
			if fp != "halt" && fp != "continue" && fp != "rollback" {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  body.FailurePolicy.Pos(),
					Msg:  fmt.Sprintf("step %q has invalid failure_policy %q; expected one of: halt, continue, rollback", stepDecl.Name, fp),
				})
			}
		}

		steps[stepDecl.Name] = stepDecl
	}

	if len(errs) > 0 {
		return errs, nil, nil
	}

	// 6. Build symbol tables for targets and nodes.
	targets := map[string]*TargetDecl{}
	nodes := map[string]*NodeDecl{}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *TargetDecl:
			if _, exists := targets[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate target %q", d.Name),
				})
			} else {
				targets[d.Name] = d
			}
		case *NodeDecl:
			if _, exists := nodes[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate node %q", d.Name),
				})
			} else {
				nodes[d.Name] = d
			}
		}
	}

	// 7. Validate nodes (resolve whether type is primitive or step).
	for _, node := range nodes {
		if node.Type == nil {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.Pos(),
				Msg:  fmt.Sprintf("node %q must specify a type", node.Name),
			})
			continue
		}

		typeName := node.Type.Name

		// Check if type is a step or primitive
		_, isStep := steps[typeName]
		isPrimitive := primitiveTypes[typeName]

		if !isStep && !isPrimitive {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.Type.Pos(),
				Msg:  fmt.Sprintf("unknown type %q (not a primitive or defined step)", typeName),
			})
			continue
		}

		// Validate targets
		for _, tIdent := range node.Targets {
			if _, isLet := lets[tIdent.Name]; isLet {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("let binding %q cannot be used in targets; targets must reference target declarations", tIdent.Name),
				})
				continue
			}
			if _, ok := targets[tIdent.Name]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("unknown target %q", tIdent.Name),
				})
			}
		}

		// Validate depends_on
		for _, dep := range node.DependsOn {
			if _, ok := nodes[dep.Value]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  dep.Pos(),
					Msg:  fmt.Sprintf("unknown depends_on node %q", dep.Value),
				})
			}
		}

		// Validate failure_policy
		if node.FailurePolicy != nil {
			fp := node.FailurePolicy.Name
			if fp != "halt" && fp != "continue" && fp != "rollback" {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  node.FailurePolicy.Pos(),
					Msg:  fmt.Sprintf("invalid failure_policy %q; expected one of: halt, continue, rollback", fp),
				})
			}
		}

		// Validate primitive-specific inputs after resolving lets
		resolvedNode := *node
		resolvedNode.Inputs = make(map[string]Expr, len(node.Inputs))
		for key, expr := range node.Inputs {
			resolvedNode.Inputs[key] = resolveLetExpr(expr, lets)
		}

		// Only validate inputs if it's a primitive (steps have their own inputs)
		if isPrimitive {
			validatePrimitiveInputsV0_1(file.Path, &resolvedNode, &errs)
		}
	}

	return errs, lets, steps
}

// CompileFileV0_4 runs parse, v0.4 validate, lower with step expansion, and IR validation.
func CompileFileV0_4(path string, src []byte) (*CompileResult, error) {
	file, parseErrs := ParseFile(path, src)
	if len(parseErrs) > 0 {
		return &CompileResult{Errors: parseErrs}, nil
	}

	semErrs, lets, steps := ValidateV0_4(file)
	if len(semErrs) > 0 {
		return &CompileResult{Errors: semErrs}, nil
	}

	p, err := LowerToPlanV0_4(file, lets, steps)
	if err != nil {
		return nil, err
	}

	// IR-level validation using existing plan.Validate
	if vErrs := plan.Validate(p); len(vErrs) > 0 {
		errs := make([]error, len(vErrs))
		for i, e := range vErrs {
			errs[i] = fmt.Errorf("%s: error: %v", path, e)
		}
		return &CompileResult{Errors: errs}, nil
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}

	return &CompileResult{
		Plan:    p,
		RawJSON: raw,
		Errors:  nil,
	}, nil
}

// ValidateV0_5 enforces the v0.5 language rules with for-loops and nested steps support.
func ValidateV0_5(file *File) ([]error, LetEnv, map[string]*StepDecl, []*ForDecl) {
	var errs []error
	lets := LetEnv{}
	steps := map[string]*StepDecl{}
	forLoops := []*ForDecl{}

	// Known primitive types for collision detection
	primitiveTypes := map[string]bool{
		"file.sync":       true,
		"process.exec":    true,
		"template.render": true,
		"health.check":    true,
		"service.ensure":  true,
		"package.install": true,
	}

	// 1. Reject unsupported constructs (module still not supported in v0.5).
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ModuleDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "modules are not supported in language version 0.5",
			})
		}
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 2. Collect and validate let bindings (with expressions, not yet evaluated).
	for _, decl := range file.Decls {
		letDecl, ok := decl.(*LetDecl)
		if !ok {
			continue
		}

		if _, exists := lets[letDecl.Name]; exists {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  letDecl.Pos(),
				Msg:  fmt.Sprintf("duplicate let %q", letDecl.Name),
			})
			continue
		}

		lets[letDecl.Name] = letDecl.Value
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 3. Type check all let expressions.
	for name, expr := range lets {
		_, err := typeCheckExpr(expr, lets, file.Path)
		if err != nil {
			errs = append(errs, err)
		}
		_ = name
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 4. Evaluate all let expressions to literals (constant folding).
	evaluatedLets := LetEnv{}
	for name, expr := range lets {
		evaluated, err := evaluateExpr(expr, lets, file.Path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		evaluatedLets[name] = evaluated
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	lets = evaluatedLets

	// 5. Collect and validate step definitions.
	for _, decl := range file.Decls {
		stepDecl, ok := decl.(*StepDecl)
		if !ok {
			continue
		}

		// Check for duplicate step names
		if _, exists := steps[stepDecl.Name]; exists {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("duplicate step %q", stepDecl.Name),
			})
			continue
		}

		// Check for primitive name collision
		if primitiveTypes[stepDecl.Name] {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step name %q conflicts with built-in primitive", stepDecl.Name),
			})
			continue
		}

		body := stepDecl.Body

		// Steps must NOT specify targets
		if len(body.Targets) > 0 {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step %q must not specify targets (targets belong to node instantiations)", stepDecl.Name),
			})
		}

		// Steps must NOT specify depends_on
		if len(body.DependsOn) > 0 {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step %q must not specify depends_on (graph structure belongs to nodes)", stepDecl.Name),
			})
		}

		// Steps must have a type
		if body.Type == nil {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step %q must specify a type", stepDecl.Name),
			})
			continue
		}

		steps[stepDecl.Name] = stepDecl
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 6. Build step dependency graph and detect cycles (v0.5 allows nested steps).
	stepGraph := buildStepDependencyGraph(steps, primitiveTypes)
	if cycles := detectStepCycles(stepGraph, steps); len(cycles) > 0 {
		for _, cycle := range cycles {
			// Format cycle path as A → B → C → A
			cyclePath := formatCyclePath(cycle)
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  steps[cycle[0]].Pos(),
				Msg:  fmt.Sprintf("circular step dependency detected: %s", cyclePath),
			})
		}
		return errs, nil, nil, nil
	}

	// 7. Validate step types (must eventually resolve to primitives).
	for name, stepDecl := range steps {
		if err := validateStepTypeResolution(name, stepDecl, steps, primitiveTypes, file.Path); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 8. Collect for-loops.
	for _, decl := range file.Decls {
		forDecl, ok := decl.(*ForDecl)
		if !ok {
			continue
		}
		forLoops = append(forLoops, forDecl)
	}

	// 9. Validate for-loops.
	for _, forDecl := range forLoops {
		// Range must evaluate to a list literal
		// First, evaluate the range expression (supports lets)
		rangeExpr := forDecl.Range

		// If it's an identifier, resolve it from lets
		if ident, ok := rangeExpr.(*Ident); ok {
			if letVal, exists := lets[ident.Name]; exists {
				rangeExpr = letVal
			} else {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  forDecl.Range.Pos(),
					Msg:  "for-loop range must evaluate to a literal list of strings",
				})
				continue
			}
		}

		// Now check if it's a list literal
		listLit, ok := rangeExpr.(*ListLiteral)
		if !ok {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  forDecl.Range.Pos(),
				Msg:  "for-loop range must evaluate to a literal list of strings",
			})
			continue
		}

		// Elements must be string literals
		for _, elem := range listLit.Elems {
			if _, ok := elem.(*StringLiteral); !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  elem.Pos(),
					Msg:  "for-loop range must evaluate to a literal list of strings",
				})
			}
		}

		// Body must contain only nodes (no steps, no nested for-loops)
		for _, bodyDecl := range forDecl.Body {
			switch bodyDecl.(type) {
			case *NodeDecl:
				// OK
			case *StepDecl, *ForDecl:
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  bodyDecl.Pos(),
					Msg:  "for-loop body may only contain node declarations",
				})
			default:
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  bodyDecl.Pos(),
					Msg:  "for-loop body may only contain node declarations",
				})
			}
		}
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 10. Build symbol tables for targets and nodes (excluding for-loop generated nodes).
	targets := map[string]*TargetDecl{}
	nodes := map[string]*NodeDecl{}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *TargetDecl:
			if _, exists := targets[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate target %q", d.Name),
				})
			} else {
				targets[d.Name] = d
			}
		case *NodeDecl:
			if _, exists := nodes[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate node %q", d.Name),
				})
			} else {
				nodes[d.Name] = d
			}
		}
	}

	// 11. Validate nodes (resolve whether type is primitive or step).
	for _, node := range nodes {
		if node.Type == nil {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.Pos(),
				Msg:  fmt.Sprintf("node %q must specify a type", node.Name),
			})
			continue
		}

		typeName := node.Type.Name

		// Check if type is a step or primitive
		_, isStep := steps[typeName]
		isPrimitive := primitiveTypes[typeName]

		if !isStep && !isPrimitive {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.Type.Pos(),
				Msg:  fmt.Sprintf("unknown type %q (not a primitive or defined step)", typeName),
			})
			continue
		}

		// Validate targets
		for _, tIdent := range node.Targets {
			if _, isLet := lets[tIdent.Name]; isLet {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("let binding %q cannot be used in targets; targets must reference target declarations", tIdent.Name),
				})
				continue
			}
			if _, ok := targets[tIdent.Name]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("unknown target %q", tIdent.Name),
				})
			}
		}

		// Validate depends_on
		for _, dep := range node.DependsOn {
			if _, ok := nodes[dep.Value]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  dep.Pos(),
					Msg:  fmt.Sprintf("unknown depends_on node %q", dep.Value),
				})
			}
		}

		// Validate failure_policy
		if node.FailurePolicy != nil {
			fp := node.FailurePolicy.Name
			if fp != "halt" && fp != "continue" && fp != "rollback" {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  node.FailurePolicy.Pos(),
					Msg:  fmt.Sprintf("invalid failure_policy %q; expected one of: halt, continue, rollback", fp),
				})
			}
		}

		// Validate primitive-specific inputs after resolving lets
		resolvedNode := *node
		resolvedNode.Inputs = make(map[string]Expr, len(node.Inputs))
		for key, expr := range node.Inputs {
			resolvedNode.Inputs[key] = resolveLetExpr(expr, lets)
		}

		// Only validate inputs if it's a primitive (steps have their own inputs)
		if isPrimitive {
			validatePrimitiveInputsV0_1(file.Path, &resolvedNode, &errs)
		}
	}

	return errs, lets, steps, forLoops
}

// buildStepDependencyGraph builds a map of step -> list of steps it depends on.
func buildStepDependencyGraph(steps map[string]*StepDecl, primitiveTypes map[string]bool) map[string][]string {
	graph := make(map[string][]string)
	for name, stepDecl := range steps {
		if stepDecl.Body.Type == nil {
			continue
		}
		typeName := stepDecl.Body.Type.Name
		if !primitiveTypes[typeName] {
			// This step depends on another step
			graph[name] = append(graph[name], typeName)
		}
	}
	return graph
}

// detectStepCycles detects cycles in the step dependency graph using DFS.
func detectStepCycles(graph map[string][]string, steps map[string]*StepDecl) [][]string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var cycles [][]string

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for _, neighbor := range graph[node] {
			if !visited[neighbor] {
				dfs(neighbor, path)
			} else if recStack[neighbor] {
				// Found a cycle
				cycleStart := -1
				for i, n := range path {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := append([]string{}, path[cycleStart:]...)
					cycle = append(cycle, neighbor)
					cycles = append(cycles, cycle)
				}
			}
		}

		recStack[node] = false
	}

	// Process steps in sorted order for determinism
	stepNames := make([]string, 0, len(steps))
	for name := range steps {
		stepNames = append(stepNames, name)
	}
	sort.Strings(stepNames)

	for _, node := range stepNames {
		if !visited[node] {
			dfs(node, []string{})
		}
	}

	return cycles
}

// formatCyclePath formats a cycle path as "A → B → C → A"
func formatCyclePath(cycle []string) string {
	if len(cycle) == 0 {
		return ""
	}
	result := cycle[0]
	for i := 1; i < len(cycle); i++ {
		result += " → " + cycle[i]
	}
	return result
}

// validateStepTypeResolution ensures that a step's type eventually resolves to a primitive.
func validateStepTypeResolution(stepName string, stepDecl *StepDecl, steps map[string]*StepDecl, primitiveTypes map[string]bool, path string) error {
	if stepDecl.Body.Type == nil {
		return &SemanticError{
			Path: path,
			Pos:  stepDecl.Pos(),
			Msg:  fmt.Sprintf("step %q must specify a type", stepName),
		}
	}

	typeName := stepDecl.Body.Type.Name

	// Check if it's a primitive
	if primitiveTypes[typeName] {
		return nil
	}

	// Check if it's another step
	if _, isStep := steps[typeName]; !isStep {
		return &SemanticError{
			Path: path,
			Pos:  stepDecl.Body.Type.Pos(),
			Msg:  fmt.Sprintf("step %q does not resolve to a primitive type", stepName),
		}
	}

	// Recursively validate (cycles already checked)
	return nil
}

// CompileFileV0_5 runs parse, v0.5 validate, lower with step expansion and for-loop unrolling, and IR validation.
func CompileFileV0_5(path string, src []byte) (*CompileResult, error) {
	file, parseErrs := ParseFile(path, src)
	if len(parseErrs) > 0 {
		return &CompileResult{Errors: parseErrs}, nil
	}

	semErrs, lets, steps, forLoops := ValidateV0_5(file)
	if len(semErrs) > 0 {
		return &CompileResult{Errors: semErrs}, nil
	}

	p, err := LowerToPlanV0_5(file, lets, steps, forLoops)
	if err != nil {
		return nil, err
	}

	// IR-level validation using existing plan.Validate
	if vErrs := plan.Validate(p); len(vErrs) > 0 {
		errs := make([]error, len(vErrs))
		for i, e := range vErrs {
			errs[i] = fmt.Errorf("%s: error: %v", path, e)
		}
		return &CompileResult{Errors: errs}, nil
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}

	return &CompileResult{
		Plan:    p,
		RawJSON: raw,
		Errors:  nil,
	}, nil
}

// ValidateV0_6 enforces the v0.6 language rules with step parameters support.
func ValidateV0_6(file *File) ([]error, LetEnv, map[string]*StepDecl, []*ForDecl) {
	var errs []error
	lets := LetEnv{}
	steps := map[string]*StepDecl{}
	forLoops := []*ForDecl{}

	// Known primitive types for collision detection
	primitiveTypes := map[string]bool{
		"file.sync":       true,
		"process.exec":    true,
		"template.render": true,
		"health.check":    true,
		"service.ensure":  true,
		"package.install": true,
	}

	// 1. Reject unsupported constructs (module still not supported in v0.6).
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ModuleDecl:
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  "modules are not supported in language version 0.6",
			})
		}
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 2. Collect and validate let bindings (with expressions, not yet evaluated).
	for _, decl := range file.Decls {
		letDecl, ok := decl.(*LetDecl)
		if !ok {
			continue
		}

		if _, exists := lets[letDecl.Name]; exists {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  letDecl.Pos(),
				Msg:  fmt.Sprintf("duplicate let %q", letDecl.Name),
			})
			continue
		}

		lets[letDecl.Name] = letDecl.Value
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 3. Type check all let expressions.
	for name, expr := range lets {
		_, err := typeCheckExpr(expr, lets, file.Path)
		if err != nil {
			errs = append(errs, err)
		}
		_ = name
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 4. Evaluate all let expressions to literals (constant folding).
	evaluatedLets := LetEnv{}
	for name, expr := range lets {
		evaluated, err := evaluateExpr(expr, lets, file.Path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		evaluatedLets[name] = evaluated
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	lets = evaluatedLets

	// 5. Collect and validate step definitions (including parameters).
	for _, decl := range file.Decls {
		stepDecl, ok := decl.(*StepDecl)
		if !ok {
			continue
		}

		// Check for duplicate step names
		if _, exists := steps[stepDecl.Name]; exists {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("duplicate step %q", stepDecl.Name),
			})
			continue
		}

		// Check for primitive name collision
		if primitiveTypes[stepDecl.Name] {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step name %q conflicts with built-in primitive", stepDecl.Name),
			})
			continue
		}

		// Validate parameters (v0.6 new feature)
		paramNames := map[string]bool{}
		for _, param := range stepDecl.Params {
			// Check parameter name uniqueness
			if paramNames[param.Name] {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  param.PosInfo,
					Msg:  fmt.Sprintf("duplicate parameter %q in step %q", param.Name, stepDecl.Name),
				})
				continue
			}
			paramNames[param.Name] = true

			// Type-check and evaluate parameter default (if present)
			// Parameter defaults are evaluated once per step definition (compile-time determinism)
			if param.Default != nil {
				// Type-check default expression
				_, err := typeCheckExpr(param.Default, nil, file.Path) // params cannot reference lets
				if err != nil {
					errs = append(errs, &SemanticError{
						Path: file.Path,
						Pos:  param.Default.Pos(),
						Msg:  fmt.Sprintf("parameter %q default in step %q: %v", param.Name, stepDecl.Name, err),
					})
					continue
				}

				// Evaluate default expression (constant folding)
				evaluated, err := evaluateExpr(param.Default, nil, file.Path)
				if err != nil {
					errs = append(errs, &SemanticError{
						Path: file.Path,
						Pos:  param.Default.Pos(),
						Msg:  fmt.Sprintf("parameter %q default in step %q: %v", param.Name, stepDecl.Name, err),
					})
					continue
				}

				// Replace default with evaluated literal
				param.Default = evaluated
			}
		}

		body := stepDecl.Body

		// Steps must NOT specify targets
		if len(body.Targets) > 0 {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step %q must not specify targets (targets belong to node instantiations)", stepDecl.Name),
			})
		}

		// Steps must NOT specify depends_on
		if len(body.DependsOn) > 0 {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step %q must not specify depends_on (graph structure belongs to nodes)", stepDecl.Name),
			})
		}

		// Steps must have a type
		if body.Type == nil {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  stepDecl.Pos(),
				Msg:  fmt.Sprintf("step %q must specify a type", stepDecl.Name),
			})
			continue
		}

		steps[stepDecl.Name] = stepDecl
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 6. Build step dependency graph and detect cycles (v0.6 inherits nested steps from v0.5).
	stepGraph := buildStepDependencyGraph(steps, primitiveTypes)
	if cycles := detectStepCycles(stepGraph, steps); len(cycles) > 0 {
		for _, cycle := range cycles {
			cyclePath := formatCyclePath(cycle)
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  steps[cycle[0]].Pos(),
				Msg:  fmt.Sprintf("circular step dependency detected: %s", cyclePath),
			})
		}
		return errs, nil, nil, nil
	}

	// 7. Validate step types (must eventually resolve to primitives).
	for name, stepDecl := range steps {
		if err := validateStepTypeResolution(name, stepDecl, steps, primitiveTypes, file.Path); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 8. Collect and validate for-loops.
	for _, decl := range file.Decls {
		forDecl, ok := decl.(*ForDecl)
		if !ok {
			continue
		}

		// Resolve range to literal
		rangeExpr := forDecl.Range
		if ident, ok := rangeExpr.(*Ident); ok {
			if letVal, exists := lets[ident.Name]; exists {
				rangeExpr = letVal
			} else {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  forDecl.Range.Pos(),
					Msg:  fmt.Sprintf("for-loop range references undefined let %q", ident.Name),
				})
				continue
			}
		}

		// Range must be a list literal
		listLit, ok := rangeExpr.(*ListLiteral)
		if !ok {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  forDecl.Range.Pos(),
				Msg:  "for-loop range must be a list literal or let-backed list",
			})
			continue
		}

		// Range must contain only string literals
		if !allStringLits(listLit.Elems) {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  forDecl.Range.Pos(),
				Msg:  "for-loop range must be a list of string literals",
			})
			continue
		}

		// Validate for-loop body declarations
		for _, bodyDecl := range forDecl.Body {
			switch bd := bodyDecl.(type) {
			case *NodeDecl:
				// Validate node in for-loop body (basic checks)
				if bd.Type == nil {
					errs = append(errs, &SemanticError{
						Path: file.Path,
						Pos:  bd.Pos(),
						Msg:  fmt.Sprintf("node %q in for-loop must specify a type", bd.Name),
					})
				}
			case *StepDecl:
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  bd.Pos(),
					Msg:  "step definitions cannot appear inside for-loops",
				})
			case *ForDecl:
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  bd.Pos(),
					Msg:  "nested for-loops are not supported",
				})
			}
		}

		forLoops = append(forLoops, forDecl)
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil
	}

	// 9. Build symbol tables for targets and nodes.
	targets := map[string]*TargetDecl{}
	nodes := map[string]*NodeDecl{}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *TargetDecl:
			if _, exists := targets[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate target %q", d.Name),
				})
			} else {
				targets[d.Name] = d
			}
		case *NodeDecl:
			if _, exists := nodes[d.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  d.Pos(),
					Msg:  fmt.Sprintf("duplicate node %q", d.Name),
				})
			} else {
				nodes[d.Name] = d
			}
		}
	}

	// 10. Validate nodes (including parameter provision for step-referencing nodes).
	for _, node := range nodes {
		if node.Type == nil {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.Pos(),
				Msg:  fmt.Sprintf("node %q must specify a type", node.Name),
			})
			continue
		}

		typeName := node.Type.Name

		// Check if type is a step or primitive
		stepDecl, isStep := steps[typeName]
		isPrimitive := primitiveTypes[typeName]

		if !isStep && !isPrimitive {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.Type.Pos(),
				Msg:  fmt.Sprintf("unknown type %q (not a primitive or defined step)", typeName),
			})
			continue
		}

		// v0.6: If node references a step, validate required parameters are provided
		if isStep {
			providedParams := map[string]bool{}
			for key := range node.Inputs {
				providedParams[key] = true
			}

			// Check all required parameters (those without defaults)
			for _, param := range stepDecl.Params {
				if param.Default == nil && !providedParams[param.Name] {
					errs = append(errs, &SemanticError{
						Path: file.Path,
						Pos:  node.Pos(),
						Msg:  fmt.Sprintf("node %q must provide required parameter %q for step %q", node.Name, param.Name, typeName),
					})
				}
			}
		}

		// Validate targets
		for _, tIdent := range node.Targets {
			if _, isLet := lets[tIdent.Name]; isLet {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("let binding %q cannot be used in targets; targets must reference target declarations", tIdent.Name),
				})
				continue
			}
			if _, ok := targets[tIdent.Name]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  tIdent.Pos(),
					Msg:  fmt.Sprintf("unknown target %q", tIdent.Name),
				})
			}
		}

		// Validate depends_on
		for _, dep := range node.DependsOn {
			if _, ok := nodes[dep.Value]; !ok {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  dep.Pos(),
					Msg:  fmt.Sprintf("unknown depends_on node %q", dep.Value),
				})
			}
		}

		// Validate failure_policy
		if node.FailurePolicy != nil {
			fp := node.FailurePolicy.Name
			if fp != "halt" && fp != "continue" && fp != "rollback" {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  node.FailurePolicy.Pos(),
					Msg:  fmt.Sprintf("invalid failure_policy %q; expected one of: halt, continue, rollback", fp),
				})
			}
		}

		// Validate primitive-specific inputs (skip parameter validation for now, handled in lowering)
	}

	return errs, lets, steps, forLoops
}

// CompileFileV0_6 runs parse, v0.6 validate, lower with parameter substitution, and IR validation.
func CompileFileV0_6(path string, src []byte) (*CompileResult, error) {
	file, parseErrs := ParseFile(path, src)
	if len(parseErrs) > 0 {
		return &CompileResult{Errors: parseErrs}, nil
	}

	semErrs, lets, steps, forLoops := ValidateV0_6(file)
	if len(semErrs) > 0 {
		return &CompileResult{Errors: semErrs}, nil
	}

	p, err := LowerToPlanV0_6(file, lets, steps, forLoops)
	if err != nil {
		return nil, err
	}

	// IR-level validation using existing plan.Validate
	if vErrs := plan.Validate(p); len(vErrs) > 0 {
		errs := make([]error, len(vErrs))
		for i, e := range vErrs {
			errs[i] = fmt.Errorf("%s: error: %v", path, e)
		}
		return &CompileResult{Errors: errs}, nil
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}

	return &CompileResult{
		Plan:    p,
		RawJSON: raw,
		Errors:  nil,
	}, nil
}

// ValidateV0_8 enforces v0.8 rules (target fleets, labels, contracts).
func ValidateV0_8(file *File) ([]error, LetEnv, map[string]*StepDecl, []*ForDecl, map[string]*FleetDecl) {
	// First run v0.6 validations (which covers lets, steps, for loops, etc)
	errs, lets, steps, forLoops := ValidateV0_6(file)

	targets := map[string]*TargetDecl{}
	fleets := map[string]*FleetDecl{}
	nodes := map[string]*NodeDecl{}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *TargetDecl:
			if _, exists := targets[d.Name]; exists {
				errs = append(errs, &SemanticError{Path: file.Path, Pos: d.Pos(), Msg: fmt.Sprintf("duplicate target %q", d.Name)})
			}
			if _, exists := fleets[d.Name]; exists {
				errs = append(errs, &SemanticError{Path: file.Path, Pos: d.Pos(), Msg: fmt.Sprintf("target name %q collides with fleet name", d.Name)})
			}
			targets[d.Name] = d
		case *FleetDecl:
			if _, exists := fleets[d.Name]; exists {
				errs = append(errs, &SemanticError{Path: file.Path, Pos: d.Pos(), Msg: fmt.Sprintf("duplicate fleet %q", d.Name)})
			}
			if _, exists := targets[d.Name]; exists {
				errs = append(errs, &SemanticError{Path: file.Path, Pos: d.Pos(), Msg: fmt.Sprintf("fleet name %q collides with target name", d.Name)})
			}
			fleets[d.Name] = d
		case *NodeDecl:
			if _, exists := nodes[d.Name]; exists {
				errs = append(errs, &SemanticError{Path: file.Path, Pos: d.Pos(), Msg: fmt.Sprintf("duplicate node %q", d.Name)})
			}
			nodes[d.Name] = d
		}
	}

	for _, d := range fleets {
		// Fleet must resolve to at least one target
		matched := false
		for _, t := range targets {
			isMatch := true
			for k, v := range d.Match {
				if t.Labels[k] != v {
					isMatch = false
					break
				}
			}
			if isMatch {
				matched = true
				break
			}
		}
		if !matched {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  d.Pos(),
				Msg:  fmt.Sprintf("fleet %q match selectors do not match any defined targets", d.Name),
			})
		}
	}

	for _, node := range nodes {
		// Validate side_effects
		if node.SideEffects != nil {
			se := node.SideEffects.Value
			if se != "none" && se != "local" && se != "external" {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  node.SideEffects.Pos(),
					Msg:  fmt.Sprintf("invalid side_effects %q; expected one of: none, local, external", se),
				})
			}
		}

		// Validate retry attempts
		if node.Retry != nil && node.Retry.Attempts <= 0 {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.Pos(), // we don't have pos info directly on Retry struct, use node
				Msg:  "retry attempts must be strictly positive",
			})
		}

		// Validate rollback_cmd type match
		if node.RollbackCmd != nil && node.Type.Name != "process.exec" {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  node.RollbackCmd.Pos(),
				Msg:  "rollback_cmd is only supported on process.exec nodes",
			})
		}

		// In v0.8 targets can be fleet names or explicit targets
		var resolvedTargets []string
		for _, tIdent := range node.Targets {
			isTarget := false
			isFleet := false
			if _, ok := targets[tIdent.Name]; ok {
				isTarget = true
				resolvedTargets = append(resolvedTargets, tIdent.Name)
			}
			if f, ok := fleets[tIdent.Name]; ok {
				isFleet = true
				// resolve fleet to target list
				for _, t := range targets {
					isMatch := true
					for k, v := range f.Match {
						if t.Labels[k] != v {
							isMatch = false
							break
						}
					}
					if isMatch {
						resolvedTargets = append(resolvedTargets, t.Name)
					}
				}
			}

			// If it's a let binding, we already emit error in v0.6 logic
			if _, isLet := lets[tIdent.Name]; !isLet && !isTarget && !isFleet {
				// Don't duplicate the "unknown target" error from ValidateV0_6 unless it's strictly a v0.8 thing
				// actually v0.6 will emit "unknown target" because it doesn't know about fleets!
				// We need to filter out the v0.6 unknown target error if it was a valid fleet.
			}
		}
	}

	// Filter out "unknown target" errors from v0.6 if they are valid fleet names
	var filteredErrs []error
	for _, err := range errs {
		if semErr, ok := err.(*SemanticError); ok {
			var tName string
			if n, _ := fmt.Sscanf(semErr.Msg, "unknown target %q", &tName); n == 1 {
				if _, ok := fleets[tName]; ok {
					continue // it's a fleet, suppress error
				}
			}
		}
		filteredErrs = append(filteredErrs, err)
	}

	return filteredErrs, lets, steps, forLoops, fleets
}

// CompileFileV0_8 runs parse, v0.8 validate, lower with parameter substitution/fleets, and IR validation.
func CompileFileV0_8(path string, src []byte) (*CompileResult, error) {
	file, parseErrs := ParseFile(path, src)
	if len(parseErrs) > 0 {
		return &CompileResult{Errors: parseErrs}, nil
	}

	semErrs, lets, steps, forLoops, fleets := ValidateV0_8(file)
	if len(semErrs) > 0 {
		return &CompileResult{Errors: semErrs}, nil
	}

	p, err := LowerToPlanV0_8(file, lets, steps, forLoops, fleets)
	if err != nil {
		return nil, err
	}

	if vErrs := plan.Validate(p); len(vErrs) > 0 {
		errs := make([]error, len(vErrs))
		for i, e := range vErrs {
			errs[i] = fmt.Errorf("%s: error: %v", path, e)
		}
		return &CompileResult{Errors: errs}, nil
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}

	return &CompileResult{
		Plan:    p,
		RawJSON: raw,
		Errors:  nil,
	}, nil
}

// ValidateV1_2 enforces v1.2 rules (custom primitives).
func ValidateV1_2(file *File) ([]error, LetEnv, map[string]*StepDecl, []*ForDecl, map[string]*FleetDecl, map[string]*PrimitiveDecl) {
	// 1. Run v0.8 validations
	errs, lets, steps, forLoops, fleets := ValidateV0_8(file)

	primitives := map[string]*PrimitiveDecl{}
	primitiveTypes := map[string]bool{
		"file.sync":       true, // existing built-ins
		"process.exec":    true,
		"template.render": true,
		"health.check":    true, // existing built-ins
		"service.ensure":  true,
		"package.install": true,
		"_fs.write":       true,
		"_fs.read":        true,
		"_fs.mkdir":       true,
		"_fs.delete":      true,
		"_fs.chmod":       true,
		"_fs.chown":       true,
		"_fs.exists":      true,
		"_fs.stat":        true,
		"_net.fetch":      true,
		"_exec":           true,
	}

	// Load stdlib primitives and add them to known types
	stdlibPrims, _ := LoadStdlib()
	for name := range stdlibPrims {
		primitiveTypes[name] = true
	}

	// 2. Collect primitives
	for _, decl := range file.Decls {
		p, ok := decl.(*PrimitiveDecl)
		if !ok {
			continue
		}

		if _, exists := primitives[p.Name]; exists {
			errs = append(errs, &SemanticError{Path: file.Path, Pos: p.Pos(), Msg: fmt.Sprintf("duplicate primitive %q", p.Name)})
			continue
		}
		if primitiveTypes[p.Name] {
			errs = append(errs, &SemanticError{Path: file.Path, Pos: p.Pos(), Msg: fmt.Sprintf("primitive name %q conflicts with built-in", p.Name)})
			continue
		}
		if _, exists := steps[p.Name]; exists {
			errs = append(errs, &SemanticError{Path: file.Path, Pos: p.Pos(), Msg: fmt.Sprintf("primitive name %q conflicts with step name", p.Name)})
			continue
		}

		primitives[p.Name] = p
	}

	if len(errs) > 0 {
		// Filter out "unknown type" errors from v0.6 logic if the type matches a custom primitive
		var filtered []error
		for _, e := range errs {
			if se, ok := e.(*SemanticError); ok {
				if strings.HasPrefix(se.Msg, "unknown type ") {
					// Use Sscanf to just extract the first quoted string
					var tName string
					fmt.Sscanf(se.Msg, "unknown type %q", &tName)
					if tName != "" && (primitives[tName] != nil || primitiveTypes[tName]) {
						continue // It's a valid custom or built-in primitive
					}
				}
			}
			filtered = append(filtered, e)
		}
		errs = filtered
	}

	if len(errs) > 0 {
		return errs, nil, nil, nil, nil, nil
	}

	// 3. Cycle Detection
	// Build primitive dependency graph (primitive calls another primitive in its body)
	adj := map[string][]string{}
	for name, p := range primitives {
		for _, bodyDecl := range p.Body {
			if node, ok := bodyDecl.(*NodeDecl); ok && node.Type != nil {
				if _, ok := primitives[node.Type.Name]; ok {
					adj[name] = appendUniqStr(adj[name], node.Type.Name)
				}
			}
		}
	}

	if cycles := findPrimitiveCycles(adj); len(cycles) > 0 {
		for _, cycle := range cycles {
			errs = append(errs, &SemanticError{
				Path: file.Path,
				Pos:  primitives[cycle[0]].Pos(),
				Msg:  fmt.Sprintf("circular primitive dependency detected: %s", formatCyclePath(cycle)),
			})
		}
	}

	return errs, lets, steps, forLoops, fleets, primitives
}

func findPrimitiveCycles(adj map[string][]string) [][]string {
	var cycles [][]string
	visited := map[string]bool{}
	recStack := map[string]bool{}

	var dfs func(u string, path []string)
	dfs = func(u string, path []string) {
		visited[u] = true
		recStack[u] = true
		currPath := append(path, u)

		for _, v := range adj[u] {
			if !visited[v] {
				dfs(v, currPath)
			} else if recStack[v] {
				// Cycle found: extract cycle from currPath
				for i, node := range currPath {
					if node == v {
						cycle := make([]string, len(currPath)-i)
						copy(cycle, currPath[i:])
						cycles = append(cycles, append(cycle, v))
						break
					}
				}
			}
		}
		recStack[u] = false
	}

	// Process in sorted order for determinism
	var keys []string
	for k := range adj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !visited[k] {
			dfs(k, []string{})
		}
	}
	return cycles
}

func appendUniqStr(slice []string, s string) []string {
	for _, x := range slice {
		if x == s {
			return slice
		}
	}
	return append(slice, s)
}

// CompileFileV1_2 runs parse, v1.2 validate, lower with primitives, and IR validation.
func CompileFileV1_2(path string, src []byte) (*CompileResult, error) {
	file, parseErrs := ParseFile(path, src)
	if len(parseErrs) > 0 {
		return &CompileResult{Errors: parseErrs}, nil
	}

	semErrs, lets, steps, forLoops, fleets, primitives := ValidateV1_2(file)
	if len(semErrs) > 0 {
		return &CompileResult{Errors: semErrs}, nil
	}

	// Merge stdlib primitives with user-defined primitives
	mergedPrimitives, err := MergeStdlibPrimitives(primitives)
	if err != nil {
		return nil, fmt.Errorf("loading stdlib: %w", err)
	}

	p, err := LowerToPlanV1_2(file, lets, steps, forLoops, fleets, mergedPrimitives)
	if err != nil {
		return nil, err
	}

	if vErrs := plan.Validate(p); len(vErrs) > 0 {
		errs := make([]error, len(vErrs))
		for i, e := range vErrs {
			errs[i] = fmt.Errorf("%s: error: %v", path, e)
		}
		return &CompileResult{Errors: errs}, nil
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}

	return &CompileResult{
		Plan:    p,
		RawJSON: raw,
		Errors:  nil,
	}, nil
}

// CompileFileV2_0 runs parse, import resolution, v2.0 validate, lower with functions, and IR validation.
func CompileFileV2_0(path string, src []byte) (*CompileResult, error) {
	file, parseErrs := ParseFile(path, src)
	if len(parseErrs) > 0 {
		return &CompileResult{Errors: parseErrs}, nil
	}

	// Resolve imports
	resolver := NewImportResolver("")
	absPath, _ := filepath.Abs(path)
	resolver.Loaded[absPath] = file // Pre-cache the main file with absolute path
	file.Path = absPath             // Ensure file.Path is absolute for consistent tracking
	resolvedDecls, importErrs := resolver.ResolveImports(file)
	if len(importErrs) > 0 {
		return &CompileResult{Errors: importErrs}, nil
	}

	// Create a new file with resolved declarations
	resolvedFile := &File{
		Path:  file.Path,
		Decls: resolvedDecls,
	}

	// Validate with v2.0 rules (extends v1.2)
	semErrs, lets, steps, forLoops, fleets, primitives, funcs := ValidateV2_0(resolvedFile)
	if len(semErrs) > 0 {
		return &CompileResult{Errors: semErrs}, nil
	}

	// Merge stdlib primitives with user-defined primitives
	mergedPrimitives, err := MergeStdlibPrimitives(primitives)
	if err != nil {
		return nil, fmt.Errorf("loading stdlib: %w", err)
	}

	p, err := LowerToPlanV2_0(resolvedFile, lets, steps, forLoops, fleets, mergedPrimitives, funcs)
	if err != nil {
		return nil, err
	}

	if vErrs := plan.Validate(p); len(vErrs) > 0 {
		errs := make([]error, len(vErrs))
		for i, e := range vErrs {
			errs[i] = fmt.Errorf("%s: error: %v", path, e)
		}
		return &CompileResult{Errors: errs}, nil
	}

	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}

	return &CompileResult{
		Plan:    p,
		RawJSON: raw,
		Errors:  nil,
	}, nil
}

// FuncEnv maps function names to their declarations.
type FuncEnv map[string]*FnDecl

// ValidateV2_0 enforces v2.0 language rules, adding imports and user-defined functions.
func ValidateV2_0(file *File) ([]error, LetEnv, map[string]*StepDecl, []*ForDecl, map[string]*FleetDecl, map[string]*PrimitiveDecl, FuncEnv) {
	var errs []error
	funcs := FuncEnv{}

	// Collect function declarations
	for _, decl := range file.Decls {
		if fnDecl, ok := decl.(*FnDecl); ok {
			if _, exists := funcs[fnDecl.Name]; exists {
				errs = append(errs, &SemanticError{
					Path: file.Path,
					Pos:  fnDecl.Pos(),
					Msg:  fmt.Sprintf("duplicate function %q", fnDecl.Name),
				})
				continue
			}
			funcs[fnDecl.Name] = fnDecl
		}
	}

	// Run v1.2 validation for the rest
	v1_2Errs, lets, steps, forLoops, fleets, primitives := ValidateV1_2(file)

	// Merge errors
	errs = append(errs, v1_2Errs...)

	return errs, lets, steps, forLoops, fleets, primitives, funcs
}
