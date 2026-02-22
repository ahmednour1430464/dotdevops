package devlang

import (
	"fmt"
)

// ParseError represents a syntax-level error produced by the parser.
type ParseError struct {
	Path string
	Pos  Position
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Pos.Line, e.Pos.Col, e.Msg)
}

// Parser is a hand-written recursive-descent parser over the token stream.
type Parser struct {
	path string
	lx   *Lexer
	cur  Token
	peek Token
	errs []error
}

// ParseFile parses a .devops file into an AST File.
func ParseFile(path string, src []byte) (*File, []error) {
	p := &Parser{
		path: path,
		lx:   NewLexer(src),
	}
	p.nextToken() // initialize first token and lookahead
	file := p.parseFile()
	if len(p.errs) > 0 {
		return nil, p.errs
	}
	return file, nil
}

func (p *Parser) nextToken() {
	p.cur = p.peek
	p.peek = p.lx.NextToken()
}

func (p *Parser) addError(msg string, pos Position) {
	p.errs = append(p.errs, &ParseError{
		Path: p.path,
		Pos:  pos,
		Msg:  msg,
	})
}

func (p *Parser) expect(tt TokenType, context string) Token {
	tok := p.cur
	if p.cur.Type != tt {
		p.addError(fmt.Sprintf("expected %s, found %q", context, p.cur.Lexeme), p.cur.Pos)
	}
	p.nextToken()
	return tok
}

func (p *Parser) parseFile() *File {
	f := &File{Path: p.path}
	for p.peek.Type != EOF {
		p.nextToken()
		if p.cur.Type == EOF {
			break
		}
		decl := p.parseDecl()
		if decl != nil {
			f.Decls = append(f.Decls, decl)
		} else {
			p.synchronize()
		}
	}
	return f
}

func (p *Parser) parseDecl() Decl {
	switch p.cur.Type {
	case KW_TARGET:
		return p.parseTargetDecl()
	case KW_NODE:
		return p.parseNodeDecl()
	case KW_LET:
		return p.parseLetDecl()
	case KW_FOR:
		return p.parseForDecl()
	case KW_STEP:
		return p.parseStepDecl()
	case KW_MODULE:
		return p.parseModuleDecl()
	default:
		p.addError("expected declaration (target, node, let, for, step, module)", p.cur.Pos)
		return nil
	}
}

func (p *Parser) synchronize() {
	// Simple error recovery: skip until a likely decl start or EOF.
	for p.cur.Type != EOF {
		switch p.cur.Type {
		case KW_TARGET, KW_NODE, KW_LET, KW_FOR, KW_STEP, KW_MODULE:
			return
		}
		p.nextToken()
	}
}

// target "name" { address = "..." }
func (p *Parser) parseTargetDecl() Decl {
	startPos := p.cur.Pos
	// expect next token to be STRING name
	p.nextToken()
	if p.cur.Type != STRING {
		p.addError("expected target name string", p.cur.Pos)
		return &TargetDecl{PosInfo: startPos}
	}
	nameTok := p.cur
	// expect '{'
	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after target name", p.cur.Pos)
	}

	// body
	var addr *StringLiteral
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type != IDENT {
			p.addError("expected identifier in target body", p.cur.Pos)
			p.synchronize()
			break
		}
		key := p.cur
		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' after identifier in target body", p.cur.Pos)
			p.synchronize()
			break
		}
		p.nextToken()
		expr := p.parseExpr()
		if key.Lexeme == "address" {
			if s, ok := expr.(*StringLiteral); ok {
				addr = s
			} else {
				p.addError("address must be a string literal", expr.Pos())
			}
		}
	}

	return &TargetDecl{
		Name:    nameTok.Lexeme,
		Address: addr,
		PosInfo: startPos,
	}
}

// node "name" { ... }
func (p *Parser) parseNodeDecl() Decl {
	startPos := p.cur.Pos
	// expect name string
	p.nextToken()
	if p.cur.Type != STRING {
		p.addError("expected node name string", p.cur.Pos)
		return &NodeDecl{PosInfo: startPos, Inputs: map[string]Expr{}}
	}
	nameTok := p.cur

	// expect '{'
	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after node name", p.cur.Pos)
	}

	node := &NodeDecl{
		Name:    nameTok.Lexeme,
		Inputs:  make(map[string]Expr),
		PosInfo: startPos,
	}

	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type != IDENT {
			p.addError("expected identifier in node body", p.cur.Pos)
			p.synchronize()
			break
		}
		keyTok := p.cur
		key := keyTok.Lexeme

		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' after identifier in node body", p.cur.Pos)
			p.synchronize()
			break
		}

		p.nextToken()
		expr := p.parseExpr()

		switch key {
		case "type":
			if id, ok := expr.(*Ident); ok {
				node.Type = id
			} else {
				p.addError("type must be an identifier", expr.Pos())
			}
		case "targets":
			if list, ok := expr.(*ListLiteral); ok {
				for _, e := range list.Elems {
					if id, ok := e.(*Ident); ok {
						node.Targets = append(node.Targets, id)
					} else {
						p.addError("targets list must contain identifiers", e.Pos())
					}
				}
			} else {
				p.addError("targets must be a list", expr.Pos())
			}
		case "depends_on":
			if list, ok := expr.(*ListLiteral); ok {
				for _, e := range list.Elems {
					if s, ok := e.(*StringLiteral); ok {
						node.DependsOn = append(node.DependsOn, s)
					} else {
						p.addError("depends_on list must contain string literals", e.Pos())
					}
				}
			} else {
				p.addError("depends_on must be a list", expr.Pos())
			}
		case "failure_policy":
			if id, ok := expr.(*Ident); ok {
				node.FailurePolicy = id
			} else {
				p.addError("failure_policy must be an identifier", expr.Pos())
			}
		default:
			// Primitive-specific inputs
			node.Inputs[key] = expr
		}
	}

	return node
}

// let name = expr
func (p *Parser) parseLetDecl() Decl {
	startPos := p.cur.Pos
	p.nextToken()
	if p.cur.Type != IDENT {
		p.addError("expected identifier after let", p.cur.Pos)
		return &LetDecl{PosInfo: startPos}
	}
	nameTok := p.cur
	p.nextToken()
	if p.cur.Type != EQUAL {
		p.addError("expected '=' after let name", p.cur.Pos)
	}
	p.nextToken()
	value := p.parseExpr()
	return &LetDecl{
		Name:   nameTok.Lexeme,
		Value:  value,
		PosInfo: startPos,
	}
}

// for x in expr { decl* }
func (p *Parser) parseForDecl() Decl {
	startPos := p.cur.Pos
	p.nextToken()
	if p.cur.Type != IDENT {
		p.addError("expected loop variable name after for", p.cur.Pos)
		return &ForDecl{PosInfo: startPos}
	}
	varName := p.cur.Lexeme
	p.nextToken()
	if p.cur.Type != KW_IN {
		p.addError("expected 'in' after loop variable", p.cur.Pos)
	}
	p.nextToken()
	rangeExpr := p.parseExpr()

	// expect '{'
	if p.cur.Type != LBRACE {
		p.addError("expected '{' to start for-body", p.cur.Pos)
	}

	body := []Decl{}
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		decl := p.parseDecl()
		if decl != nil {
			body = append(body, decl)
		} else {
			p.synchronize()
		}
	}

	return &ForDecl{
		VarName: varName,
		Range:   rangeExpr,
		Body:    body,
		PosInfo: startPos,
	}
}

// step "name" { node-body }
func (p *Parser) parseStepDecl() Decl {
	startPos := p.cur.Pos
	p.nextToken()
	if p.cur.Type != STRING {
		p.addError("expected step name string", p.cur.Pos)
		return &StepDecl{PosInfo: startPos}
	}
	nameTok := p.cur

	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after step name", p.cur.Pos)
	}

	// Reuse node-body parsing by constructing a NodeDecl manually.
	node := &NodeDecl{
		Name:    nameTok.Lexeme,
		Inputs:  make(map[string]Expr),
		PosInfo: startPos,
	}

	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type != IDENT {
			p.addError("expected identifier in step body", p.cur.Pos)
			p.synchronize()
			break
		}
		keyTok := p.cur
		key := keyTok.Lexeme

		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' after identifier in step body", p.cur.Pos)
			p.synchronize()
			break
		}

		p.nextToken()
		expr := p.parseExpr()

		switch key {
		case "type":
			if id, ok := expr.(*Ident); ok {
				node.Type = id
			} else {
				p.addError("type must be an identifier", expr.Pos())
			}
		case "targets":
			if list, ok := expr.(*ListLiteral); ok {
				for _, e := range list.Elems {
					if id, ok := e.(*Ident); ok {
						node.Targets = append(node.Targets, id)
					} else {
						p.addError("targets list must contain identifiers", e.Pos())
					}
				}
			} else {
				p.addError("targets must be a list", expr.Pos())
			}
		case "depends_on":
			if list, ok := expr.(*ListLiteral); ok {
				for _, e := range list.Elems {
					if s, ok := e.(*StringLiteral); ok {
						node.DependsOn = append(node.DependsOn, s)
					} else {
						p.addError("depends_on list must contain string literals", e.Pos())
					}
				}
			} else {
				p.addError("depends_on must be a list", expr.Pos())
			}
		case "failure_policy":
			if id, ok := expr.(*Ident); ok {
				node.FailurePolicy = id
			} else {
				p.addError("failure_policy must be an identifier", expr.Pos())
			}
		default:
			node.Inputs[key] = expr
		}
	}

	return &StepDecl{
		Name:    nameTok.Lexeme,
		Body:    node,
		PosInfo: startPos,
	}
}

// module name { decl* }
func (p *Parser) parseModuleDecl() Decl {
	startPos := p.cur.Pos
	p.nextToken()
	if p.cur.Type != IDENT {
		p.addError("expected module name identifier", p.cur.Pos)
		return &ModuleDecl{PosInfo: startPos}
	}
	nameTok := p.cur

	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after module name", p.cur.Pos)
	}

	decls := []Decl{}
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		decl := p.parseDecl()
		if decl != nil {
			decls = append(decls, decl)
		} else {
			p.synchronize()
		}
	}

	return &ModuleDecl{
		Name:    nameTok.Lexeme,
		Decls:   decls,
		PosInfo: startPos,
	}
}

// parseExpr parses expressions with precedence climbing.
// Precedence (lowest to highest): ternary (?:), ||, &&, ==, !=, +
func (p *Parser) parseExpr() Expr {
	return p.parseTernary()
}

// parseTernary parses ternary conditional: cond ? true_expr : false_expr
func (p *Parser) parseTernary() Expr {
	startPos := p.cur.Pos
	expr := p.parseLogicalOr()

	if p.peek.Type == QUESTION {
		p.nextToken() // consume '?'
		p.nextToken()
		trueExpr := p.parseExpr()
		if p.peek.Type != COLON {
			p.addError("expected ':' in ternary expression", p.peek.Pos)
			return expr
		}
		p.nextToken() // consume ':'
		p.nextToken()
		falseExpr := p.parseExpr()
		return &TernaryExpr{
			Cond:      expr,
			TrueExpr:  trueExpr,
			FalseExpr: falseExpr,
			PosInfo:   startPos,
		}
	}

	return expr
}

// parseLogicalOr parses logical OR: expr || expr
func (p *Parser) parseLogicalOr() Expr {
	left := p.parseLogicalAnd()

	for p.peek.Type == PIPEPIPE {
		opPos := p.peek.Pos
		p.nextToken() // consume '||'
		p.nextToken()
		right := p.parseLogicalAnd()
		left = &BinaryExpr{
			Left:    left,
			Op:      OpOr,
			Right:   right,
			PosInfo: opPos,
		}
	}

	return left
}

// parseLogicalAnd parses logical AND: expr && expr
func (p *Parser) parseLogicalAnd() Expr {
	left := p.parseEquality()

	for p.peek.Type == AMPAMP {
		opPos := p.peek.Pos
		p.nextToken() // consume '&&'
		p.nextToken()
		right := p.parseEquality()
		left = &BinaryExpr{
			Left:    left,
			Op:      OpAnd,
			Right:   right,
			PosInfo: opPos,
		}
	}

	return left
}

// parseEquality parses equality: expr == expr, expr != expr
func (p *Parser) parseEquality() Expr {
	left := p.parseConcat()

	if p.peek.Type == EQEQ || p.peek.Type == BANGEQ {
		opPos := p.peek.Pos
		opType := p.peek.Type
		p.nextToken() // consume '==' or '!='
		p.nextToken()
		right := p.parseConcat()

		op := OpEq
		if opType == BANGEQ {
			op = OpNeq
		}

		return &BinaryExpr{
			Left:    left,
			Op:      op,
			Right:   right,
			PosInfo: opPos,
		}
	}

	return left
}

// parseConcat parses string concatenation: expr + expr + ...
func (p *Parser) parseConcat() Expr {
	left := p.parsePrimary()

	for p.peek.Type == PLUS {
		opPos := p.peek.Pos
		p.nextToken() // consume '+'
		p.nextToken()
		right := p.parsePrimary()
		left = &BinaryExpr{
			Left:    left,
			Op:      OpAdd,
			Right:   right,
			PosInfo: opPos,
		}
	}

	return left
}

// parsePrimary parses primary expressions: literals, identifiers, lists
func (p *Parser) parsePrimary() Expr {
	tok := p.cur
	switch tok.Type {
	case STRING:
		return &StringLiteral{Value: tok.Lexeme, PosInfo: tok.Pos}
	case BOOL:
		return &BoolLiteral{Value: tok.Lexeme == "true", PosInfo: tok.Pos}
	case IDENT:
		return &Ident{Name: tok.Lexeme, PosInfo: tok.Pos}
	case LBRACKET:
		return p.parseListLiteral()
	default:
		p.addError("unexpected token in expression", tok.Pos)
		return &StringLiteral{Value: "", PosInfo: tok.Pos}
	}
}

func (p *Parser) parseListLiteral() Expr {
	startPos := p.cur.Pos
	// current is '['
	var elems []Expr

	// Move to first element or closing ']'
	p.nextToken()
	for p.cur.Type != RBRACKET && p.cur.Type != EOF {
		// parse element
		elem := p.parseExpr()
		elems = append(elems, elem)

		// move to next token (comma or closing bracket)
		p.nextToken()
		if p.cur.Type == COMMA {
			p.nextToken()
			continue
		}
	}

	if p.cur.Type != RBRACKET {
		p.addError("unterminated list literal", startPos)
	}

	return &ListLiteral{Elems: elems, PosInfo: startPos}
}
