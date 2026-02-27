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

// VersionDecl represents `version = "v0.7"` — the self-declared language version (v0.7+).
type VersionDecl struct {
	Version string
	PosInfo Position
}

func (d *VersionDecl) Pos() Position { return d.PosInfo }
func (d *VersionDecl) declNode()     {}

// TargetDecl represents `target "name" { ... }`.
type TargetDecl struct {
	Name    string
	Address *StringLiteral
	Labels  map[string]string // v0.8: metadata labels for fleet selection
	PosInfo Position
}

// FleetDecl represents `fleet "name" { match = { key = "val" } }` — a named group of targets by label selector (v0.8+).
type FleetDecl struct {
	Name    string
	Match   map[string]string // all key/value pairs must match target labels
	PosInfo Position
}

func (d *FleetDecl) Pos() Position { return d.PosInfo }
func (d *FleetDecl) declNode()     {}

func (d *TargetDecl) Pos() Position { return d.PosInfo }
func (d *TargetDecl) declNode()     {}

// RetryConfig holds retry parameters for a node (v0.8+).
type RetryConfig struct {
	Attempts int
	Delay    string // e.g. "5s"
}

// NodeDecl represents `node "name" { ... }`.
type NodeDecl struct {
	Name          string
	Type          *Ident
	Targets       []*Ident
	DependsOn     []*StringLiteral
	FailurePolicy *Ident
	Inputs        map[string]Expr
	PosInfo       Position
	// v0.8 contract fields (all optional)
	Idempotent  *BoolLiteral   // true = safe to retry automatically
	SideEffects *StringLiteral // "none" | "local" | "external"
	Retry       *RetryConfig   // automatic retry configuration
	RollbackCmd *ListLiteral   // process.exec inverse command for rollback
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

// ParamDecl represents a parameter declaration in a step (v0.6+).
type ParamDecl struct {
	Name    string
	Default Expr     // nil if required parameter
	PosInfo Position
}

// StepDecl is parsed but rejected in v0.1.
type StepDecl struct {
	Name    string
	Params  []*ParamDecl // v0.6+: parameter declarations
	Body    *NodeDecl
	PosInfo Position
}

func (d *StepDecl) Pos() Position { return d.PosInfo }
func (d *StepDecl) declNode()     {}

// PrimitiveInputDecl represents an input declaration in a primitive (v1.2+).
type PrimitiveInputDecl struct {
	Name    string
	Type    *Ident   // identifier representing the type (e.g. string, bool, list)
	PosInfo Position
}

// ContractDecl represents a contract block in a primitive (v1.2+).
// Declares behavioral guarantees like idempotency, side effects, and retry policy.
type ContractDecl struct {
	Idempotent  *bool   // nil means unspecified
	SideEffects *string // "none", "local", "external", nil means unspecified
	Retry       *int    // nil means unspecified
	PosInfo     Position
}

func (d *ContractDecl) Pos() Position { return d.PosInfo }

// ProbeField represents a single observation in a probe block (v1.3+).
// Example: exists = _fs.exists(inputs.path)
type ProbeField struct {
	Name    string
	Expr    Expr // The expression to evaluate (restricted to read-only built-ins)
	PosInfo Position
}

func (p *ProbeField) Pos() Position { return p.PosInfo }

// ProbeDecl represents a probe block in a primitive (v1.3+).
// Contains a list of observations that describe the current state.
type ProbeDecl struct {
	Fields  []*ProbeField
	PosInfo Position
}

func (d *ProbeDecl) Pos() Position { return d.PosInfo }

// DesiredField represents a single field in a desired state block (v1.3+).
// Example: exists = true
type DesiredField struct {
	Name    string
	Expr    Expr // The expected value expression
	PosInfo Position
}

func (d *DesiredField) Pos() Position { return d.PosInfo }

// DesiredDecl represents a desired block in a primitive (v1.3+).
// Contains the expected values that probe results should match.
type DesiredDecl struct {
	Fields  []*DesiredField
	PosInfo Position
}

func (d *DesiredDecl) Pos() Position { return d.PosInfo }

// PrepareBinding represents a single variable binding in a prepare block (v1.4+).
// Example: files = _ctrl.readdir(inputs.src)
type PrepareBinding struct {
	Name    string // variable name
	Expr    Expr   // the expression to evaluate on controller
	PosInfo Position
}

func (b *PrepareBinding) Pos() Position { return b.PosInfo }

// PrepareDecl represents a prepare { ... } block in a primitive (v1.4+).
// Contains controller-side computations that run before body expansion.
type PrepareDecl struct {
	Bindings []*PrepareBinding
	PosInfo  Position
}

func (d *PrepareDecl) Pos() Position { return d.PosInfo }

// ForeachBodyDecl represents a foreach x in y { ... } block in a primitive body (v1.4+).
// Expands at apply-time, generating nodes for each element in the list.
type ForeachBodyDecl struct {
	VarName string // loop variable name (e.g., "file")
	Range   Expr   // list expression to iterate (e.g., prepare.files)
	Body    []Decl // declarations to expand for each element
	PosInfo Position
}

func (d *ForeachBodyDecl) Pos() Position { return d.PosInfo }
func (d *ForeachBodyDecl) declNode()     {}

// PrimitiveDecl represents `primitive "name" { ... }` (v1.2+).
type PrimitiveDecl struct {
	Name     string
	Inputs   []*PrimitiveInputDecl
	Prepare  *PrepareDecl  // optional prepare block (v1.4+)
	Body     []Decl
	Contract *ContractDecl // optional contract block
	Probe    *ProbeDecl    // optional probe block (v1.3+)
	Desired  *DesiredDecl  // optional desired state block (v1.3+)
	PosInfo  Position
}

func (d *PrimitiveDecl) Pos() Position { return d.PosInfo }
func (d *PrimitiveDecl) declNode()     {}

// ModuleDecl is parsed but rejected in v0.1.
type ModuleDecl struct {
	Name    string
	Decls   []Decl
	PosInfo Position
}

func (d *ModuleDecl) Pos() Position { return d.PosInfo }
func (d *ModuleDecl) declNode()     {}

// ImportDecl represents `import "path"` (v2.0+).
// Imports all declarations from another .devops file into the current scope.
type ImportDecl struct {
	Path    string   // relative or absolute path to the imported file
	PosInfo Position
}

func (d *ImportDecl) Pos() Position { return d.PosInfo }
func (d *ImportDecl) declNode()     {}

// FnDecl represents a user-defined function `fn name(params) { body }` (v2.0+).
// Functions are expanded at compile-time like macros.
type FnDecl struct {
	Name    string       // function name
	Params  []string     // parameter names (positional, untyped)
	Body    Expr         // function body expression
	PosInfo Position
}

func (d *FnDecl) Pos() Position { return d.PosInfo }
func (d *FnDecl) declNode()     {}

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

// NumberLiteral represents a numeric literal.
type NumberLiteral struct {
	Value   int
	PosInfo Position
}

func (e *NumberLiteral) Pos() Position { return e.PosInfo }
func (e *NumberLiteral) exprNode()     {}

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

// SecretRef represents `secret("KEY")` — a v0.9 typed reference to an external secret value.
// Secrets are never embedded in the compiled plan; a sentinel placeholder is emitted instead.
type SecretRef struct {
	Key     string // the secret key name to resolve at apply time
	PosInfo Position
}

func (e *SecretRef) Pos() Position { return e.PosInfo }
func (e *SecretRef) exprNode()     {}

// FunctionCall represents a function call expression like `_fs.exists(path)` (v1.3+).
type FunctionCall struct {
	Name    string // function name (may include dots like _fs.exists)
	Args    []Expr // function arguments
	PosInfo Position
}

func (e *FunctionCall) Pos() Position { return e.PosInfo }
func (e *FunctionCall) exprNode()     {}

// MapLiteral represents a map value (used internally for _ctrl.readdir results).
type MapLiteral struct {
	Value   map[string]any
	PosInfo Position
}

func (e *MapLiteral) Pos() Position { return e.PosInfo }
func (e *MapLiteral) exprNode()     {}

