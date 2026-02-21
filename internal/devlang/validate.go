package devlang

import (
	"encoding/json"
	"fmt"

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
		case "file.sync", "process.exec":
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

// CompileResult is the high-level result of compiling a .devops file.
type CompileResult struct {
	Plan    *plan.Plan
	RawJSON []byte
	Errors  []error
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
