package devlang

import "fmt"

// evaluateExpr evaluates an expression to a literal at compile time.
// Returns a literal expression (StringLiteral, BoolLiteral, or ListLiteral).
func evaluateExpr(e Expr, lets LetEnv, path string) (Expr, error) {
	switch v := e.(type) {
	case *StringLiteral:
		return v, nil

	case *BoolLiteral:
		return v, nil

	case *NumberLiteral:
		return v, nil

	case *ListLiteral:
		// Evaluate all elements
		evaluatedElems := make([]Expr, len(v.Elems))
		for i, elem := range v.Elems {
			evalElem, err := evaluateExpr(elem, lets, path)
			if err != nil {
				return nil, err
			}
			evaluatedElems[i] = evalElem
		}
		return &ListLiteral{
			Elems:   evaluatedElems,
			PosInfo: v.PosInfo,
		}, nil

	case *Ident:
		// Look up in lets and evaluate recursively
		if lets == nil {
			return nil, &SemanticError{
				Path: path,
				Pos:  v.Pos(),
				Msg:  fmt.Sprintf("unresolved identifier %q in expression", v.Name),
			}
		}
		letExpr, ok := lets[v.Name]
		if !ok {
			return nil, &SemanticError{
				Path: path,
				Pos:  v.Pos(),
				Msg:  fmt.Sprintf("unresolved identifier %q in expression", v.Name),
			}
		}
		return evaluateExpr(letExpr, lets, path)

	case *BinaryExpr:
		// Evaluate operands
		leftVal, err := evaluateExpr(v.Left, lets, path)
		if err != nil {
			return nil, err
		}
		rightVal, err := evaluateExpr(v.Right, lets, path)
		if err != nil {
			return nil, err
		}

		switch v.Op {
		case OpAdd:
			// String concatenation
			leftStr, ok1 := leftVal.(*StringLiteral)
			rightStr, ok2 := rightVal.(*StringLiteral)
			if !ok1 || !ok2 {
				return nil, &SemanticError{
					Path: path,
					Pos:  v.Pos(),
					Msg:  "internal error: '+' operands are not strings after type checking",
				}
			}
			return &StringLiteral{
				Value:   leftStr.Value + rightStr.Value,
				PosInfo: v.PosInfo,
			}, nil

		case OpAnd:
			// Logical AND
			leftBool, ok1 := leftVal.(*BoolLiteral)
			rightBool, ok2 := rightVal.(*BoolLiteral)
			if !ok1 || !ok2 {
				return nil, &SemanticError{
					Path: path,
					Pos:  v.Pos(),
					Msg:  "internal error: '&&' operands are not bools after type checking",
				}
			}
			return &BoolLiteral{
				Value:   leftBool.Value && rightBool.Value,
				PosInfo: v.PosInfo,
			}, nil

		case OpOr:
			// Logical OR
			leftBool, ok1 := leftVal.(*BoolLiteral)
			rightBool, ok2 := rightVal.(*BoolLiteral)
			if !ok1 || !ok2 {
				return nil, &SemanticError{
					Path: path,
					Pos:  v.Pos(),
					Msg:  "internal error: '||' operands are not bools after type checking",
				}
			}
			return &BoolLiteral{
				Value:   leftBool.Value || rightBool.Value,
				PosInfo: v.PosInfo,
			}, nil

		case OpEq:
			// Equality
			result := false
			if leftStr, ok1 := leftVal.(*StringLiteral); ok1 {
				if rightStr, ok2 := rightVal.(*StringLiteral); ok2 {
					result = leftStr.Value == rightStr.Value
				}
			} else if leftBool, ok1 := leftVal.(*BoolLiteral); ok1 {
				if rightBool, ok2 := rightVal.(*BoolLiteral); ok2 {
					result = leftBool.Value == rightBool.Value
				}
			}
			return &BoolLiteral{
				Value:   result,
				PosInfo: v.PosInfo,
			}, nil

		case OpNeq:
			// Inequality
			result := false
			if leftStr, ok1 := leftVal.(*StringLiteral); ok1 {
				if rightStr, ok2 := rightVal.(*StringLiteral); ok2 {
					result = leftStr.Value != rightStr.Value
				}
			} else if leftBool, ok1 := leftVal.(*BoolLiteral); ok1 {
				if rightBool, ok2 := rightVal.(*BoolLiteral); ok2 {
					result = leftBool.Value != rightBool.Value
				}
			}
			return &BoolLiteral{
				Value:   result,
				PosInfo: v.PosInfo,
			}, nil

		default:
			return nil, &SemanticError{
				Path: path,
				Pos:  v.Pos(),
				Msg:  "unknown binary operator in evaluation",
			}
		}

	case *TernaryExpr:
		// Evaluate condition
		condVal, err := evaluateExpr(v.Cond, lets, path)
		if err != nil {
			return nil, err
		}

		condBool, ok := condVal.(*BoolLiteral)
		if !ok {
			return nil, &SemanticError{
				Path: path,
				Pos:  v.Cond.Pos(),
				Msg:  "internal error: ternary condition is not bool after type checking",
			}
		}

		// Evaluate the appropriate branch
		if condBool.Value {
			return evaluateExpr(v.TrueExpr, lets, path)
		} else {
			return evaluateExpr(v.FalseExpr, lets, path)
		}

	default:
		return nil, &SemanticError{
			Path: path,
			Pos:  e.Pos(),
			Msg:  "unsupported expression type in evaluation",
		}
	}
}
