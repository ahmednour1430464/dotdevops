package devlang

import "fmt"

// Type represents a compile-time type in the v0.3 type system.
type Type int

const (
	TypeString Type = iota
	TypeBool
	TypeStringList
)

func (t Type) String() string {
	switch t {
	case TypeString:
		return "string"
	case TypeBool:
		return "bool"
	case TypeStringList:
		return "string[]"
	default:
		return "unknown"
	}
}

// typeCheckExpr infers the type of an expression using the let environment.
// Returns the type and any type errors encountered.
func typeCheckExpr(e Expr, lets LetEnv, path string) (Type, error) {
	switch v := e.(type) {
	case *StringLiteral:
		return TypeString, nil

	case *BoolLiteral:
		return TypeBool, nil

	case *ListLiteral:
		if len(v.Elems) == 0 {
			return TypeStringList, nil
		}
		// Check all elements are strings
		for _, elem := range v.Elems {
			t, err := typeCheckExpr(elem, lets, path)
			if err != nil {
				return TypeString, err
			}
			if t != TypeString {
				return TypeString, &SemanticError{
					Path: path,
					Pos:  elem.Pos(),
					Msg:  fmt.Sprintf("list element must be string, found %s", t),
				}
			}
		}
		return TypeStringList, nil

	case *Ident:
		// Look up in lets
		if lets == nil {
			return TypeString, &SemanticError{
				Path: path,
				Pos:  v.Pos(),
				Msg:  fmt.Sprintf("unresolved identifier %q", v.Name),
			}
		}
		letExpr, ok := lets[v.Name]
		if !ok {
			return TypeString, &SemanticError{
				Path: path,
				Pos:  v.Pos(),
				Msg:  fmt.Sprintf("unresolved identifier %q", v.Name),
			}
		}
		return typeCheckExpr(letExpr, lets, path)

	case *BinaryExpr:
		leftType, err := typeCheckExpr(v.Left, lets, path)
		if err != nil {
			return TypeString, err
		}
		rightType, err := typeCheckExpr(v.Right, lets, path)
		if err != nil {
			return TypeString, err
		}

		switch v.Op {
		case OpAdd:
			// string + string → string
			if leftType != TypeString || rightType != TypeString {
				return TypeString, &SemanticError{
					Path: path,
					Pos:  v.Pos(),
					Msg:  fmt.Sprintf("type mismatch: cannot apply '+' to %s and %s", leftType, rightType),
				}
			}
			return TypeString, nil

		case OpAnd, OpOr:
			// bool && bool → bool, bool || bool → bool
			if leftType != TypeBool || rightType != TypeBool {
				opStr := "&&"
				if v.Op == OpOr {
					opStr = "||"
				}
				return TypeBool, &SemanticError{
					Path: path,
					Pos:  v.Pos(),
					Msg:  fmt.Sprintf("type mismatch: cannot apply '%s' to %s and %s", opStr, leftType, rightType),
				}
			}
			return TypeBool, nil

		case OpEq, OpNeq:
			// T == T → bool, T != T → bool (types must match)
			if leftType != rightType {
				opStr := "=="
				if v.Op == OpNeq {
					opStr = "!="
				}
				return TypeBool, &SemanticError{
					Path: path,
					Pos:  v.Pos(),
					Msg:  fmt.Sprintf("type mismatch: cannot compare %s %s %s", leftType, opStr, rightType),
				}
			}
			// Lists can't be compared in v0.3
			if leftType == TypeStringList {
				return TypeBool, &SemanticError{
					Path: path,
					Pos:  v.Pos(),
					Msg:  "list comparison not supported",
				}
			}
			return TypeBool, nil

		default:
			return TypeString, &SemanticError{
				Path: path,
				Pos:  v.Pos(),
				Msg:  "unknown binary operator",
			}
		}

	case *TernaryExpr:
		condType, err := typeCheckExpr(v.Cond, lets, path)
		if err != nil {
			return TypeString, err
		}
		if condType != TypeBool {
			return TypeString, &SemanticError{
				Path: path,
				Pos:  v.Cond.Pos(),
				Msg:  fmt.Sprintf("ternary condition must be bool, found %s", condType),
			}
		}

		trueType, err := typeCheckExpr(v.TrueExpr, lets, path)
		if err != nil {
			return TypeString, err
		}
		falseType, err := typeCheckExpr(v.FalseExpr, lets, path)
		if err != nil {
			return TypeString, err
		}

		if trueType != falseType {
			return TypeString, &SemanticError{
				Path: path,
				Pos:  v.Pos(),
				Msg:  fmt.Sprintf("ternary branches must have same type: true branch is %s, false branch is %s", trueType, falseType),
			}
		}

		return trueType, nil

	default:
		return TypeString, &SemanticError{
			Path: path,
			Pos:  e.Pos(),
			Msg:  "unsupported expression type in type checking",
		}
	}
}
