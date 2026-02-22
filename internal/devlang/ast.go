package devlang

// Position represents a source location (1-based line and column).
type Position struct {
	Line int
	Col  int
}

type Decl interface {
	Pos() Position
	declNode()
}

// File is the root node for a .devops source file.
type File struct {
	Path  string
	Decls []Decl
}

// TargetDecl represents `target "name" { ... }`.
type TargetDecl struct {
	Name    string
	Address *StringLiteral
	PosInfo Position
}

func (d *TargetDecl) Pos() Position { return d.PosInfo }
func (d *TargetDecl) declNode()     {}

// NodeDecl represents `node "name" { ... }`.
type NodeDecl struct {
	Name          string
	Type          *Ident
	Targets       []*Ident
	DependsOn     []*StringLiteral
	FailurePolicy *Ident
	Inputs        map[string]Expr
	PosInfo       Position
}

func (d *NodeDecl) Pos() Position { return d.PosInfo }
func (d *NodeDecl) declNode()     {}

// LetDecl is parsed but rejected in v0.1.
type LetDecl struct {
	Name   string
	Value  Expr
	PosInfo Position
}

func (d *LetDecl) Pos() Position { return d.PosInfo }
func (d *LetDecl) declNode()     {}

// ForDecl is parsed but rejected in v0.1.
type ForDecl struct {
	VarName string
	Range   Expr
	Body    []Decl
	PosInfo Position
}

func (d *ForDecl) Pos() Position { return d.PosInfo }
func (d *ForDecl) declNode()     {}

// StepDecl is parsed but rejected in v0.1.
type StepDecl struct {
	Name    string
	Body    *NodeDecl
	PosInfo Position
}

func (d *StepDecl) Pos() Position { return d.PosInfo }
func (d *StepDecl) declNode()     {}

// ModuleDecl is parsed but rejected in v0.1.
type ModuleDecl struct {
	Name    string
	Decls   []Decl
	PosInfo Position
}

func (d *ModuleDecl) Pos() Position { return d.PosInfo }
func (d *ModuleDecl) declNode()     {}

// Expr is the interface for all expression nodes.
type Expr interface {
	Pos() Position
	exprNode()
}

// Ident represents an identifier like local or file.sync.
type Ident struct {
	Name    string
	PosInfo Position
}

func (e *Ident) Pos() Position { return e.PosInfo }
func (e *Ident) exprNode()     {}

// StringLiteral represents a string literal.
type StringLiteral struct {
	Value   string
	PosInfo Position
}

func (e *StringLiteral) Pos() Position { return e.PosInfo }
func (e *StringLiteral) exprNode()     {}

// BoolLiteral represents a boolean literal.
type BoolLiteral struct {
	Value   bool
	PosInfo Position
}

func (e *BoolLiteral) Pos() Position { return e.PosInfo }
func (e *BoolLiteral) exprNode()     {}

// ListLiteral represents a list value, e.g. ["a", "b"].
type ListLiteral struct {
	Elems   []Expr
	PosInfo Position
}

func (e *ListLiteral) Pos() Position { return e.PosInfo }
func (e *ListLiteral) exprNode()     {}

// BinaryOp represents a binary operator.
type BinaryOp int

const (
	OpAdd BinaryOp = iota // +
	OpAnd                 // &&
	OpOr                  // ||
	OpEq                  // ==
	OpNeq                 // !=
)

// BinaryExpr represents binary operations: +, &&, ||, ==, !=
type BinaryExpr struct {
	Left    Expr
	Op      BinaryOp
	Right   Expr
	PosInfo Position
}

func (e *BinaryExpr) Pos() Position { return e.PosInfo }
func (e *BinaryExpr) exprNode()     {}

// TernaryExpr represents conditional: cond ? true_expr : false_expr
type TernaryExpr struct {
	Cond      Expr
	TrueExpr  Expr
	FalseExpr Expr
	PosInfo   Position
}

func (e *TernaryExpr) Pos() Position { return e.PosInfo }
func (e *TernaryExpr) exprNode()     {}
