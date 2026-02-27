package devlang

import (
	"fmt"
	"strconv"
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
	case KW_PRIMITIVE:
		return p.parsePrimitiveDecl()
	case KW_VERSION:
		return p.parseVersionDecl()
	case KW_FLEET:
		return p.parseFleetDecl()
	case KW_IMPORT:
		return p.parseImportDecl()
	case KW_FN:
		return p.parseFnDecl()
	default:
		p.addError("expected declaration (version, target, node, let, for, step, fleet, module, import, fn)", p.cur.Pos)
		return nil
	}
}

func (p *Parser) synchronize() {
	// Simple error recovery: skip until a likely decl start or EOF.
	for p.cur.Type != EOF {
		switch p.cur.Type {
		case KW_TARGET, KW_NODE, KW_LET, KW_FOR, KW_STEP, KW_MODULE, KW_VERSION, KW_FLEET, KW_PRIMITIVE, KW_IMPORT, KW_FN:
			return
		}
		p.nextToken()
	}
}

// version = "v0.7" — self-declared language version directive (v0.7+).
func (p *Parser) parseVersionDecl() Decl {
	startPos := p.cur.Pos
	// expect '='
	p.nextToken()
	if p.cur.Type != EQUAL {
		p.addError("expected '=' after 'version'", p.cur.Pos)
		return &VersionDecl{PosInfo: startPos}
	}
	// expect string version
	p.nextToken()
	if p.cur.Type != STRING {
		p.addError("expected version string (e.g. \"v0.7\")", p.cur.Pos)
		return &VersionDecl{PosInfo: startPos}
	}
	return &VersionDecl{
		Version: p.cur.Lexeme,
		PosInfo: startPos,
	}
}

// fleet "name" { match = { key = "val" } }
func (p *Parser) parseFleetDecl() Decl {
	startPos := p.cur.Pos
	p.nextToken()
	if p.cur.Type != STRING {
		p.addError("expected fleet name string", p.cur.Pos)
		return &FleetDecl{PosInfo: startPos}
	}
	nameTok := p.cur

	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after fleet name", p.cur.Pos)
	}

	match := map[string]string{}
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type != IDENT {
			p.addError("expected identifier in fleet body", p.cur.Pos)
			p.synchronize()
			break
		}
		key := p.cur.Lexeme
		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' in fleet body", p.cur.Pos)
			p.synchronize()
			break
		}
		p.nextToken()
		if key == "match" {
			// parse inline map: { key = "val" ... }
			if p.cur.Type != LBRACE {
				p.addError("expected '{' for match map", p.cur.Pos)
			} else {
				match = p.parseStringMap()
			}
		} else {
			// skip unknown fields
			p.parseExpr()
		}
	}

	return &FleetDecl{
		Name:    nameTok.Lexeme,
		Match:   match,
		PosInfo: startPos,
	}
}

// parseStringMap parses { key = "val" key2 = "val2" } → map[string]string.
// Current token must be '{' on entry.
func (p *Parser) parseStringMap() map[string]string {
	result := map[string]string{}
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type == COMMA {
			continue
		}
		if p.cur.Type != IDENT {
			p.addError("expected identifier in map", p.cur.Pos)
			break
		}
		key := p.cur.Lexeme
		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' in map", p.cur.Pos)
			break
		}
		p.nextToken()
		if p.cur.Type != STRING {
			p.addError("expected string value in map", p.cur.Pos)
			break
		}
		result[key] = p.cur.Lexeme
	}
	return result
}

// import "path" — imports declarations from another file (v2.0+).
func (p *Parser) parseImportDecl() Decl {
	startPos := p.cur.Pos
	// expect string path
	p.nextToken()
	if p.cur.Type != STRING {
		p.addError("expected import path string", p.cur.Pos)
		return &ImportDecl{PosInfo: startPos}
	}
	return &ImportDecl{
		Path:    p.cur.Lexeme,
		PosInfo: startPos,
	}
}

// fn name(params) { body } — user-defined function (v2.0+).
func (p *Parser) parseFnDecl() Decl {
	startPos := p.cur.Pos
	// expect identifier (function name)
	p.nextToken()
	if p.cur.Type != IDENT {
		p.addError("expected function name", p.cur.Pos)
		return &FnDecl{PosInfo: startPos}
	}
	name := p.cur.Lexeme

	// expect '('
	p.nextToken()
	if p.cur.Type != LPAREN {
		p.addError("expected '(' after function name", p.cur.Pos)
		return &FnDecl{Name: name, PosInfo: startPos}
	}

	// parse parameters
	var params []string
	for {
		p.nextToken()
		if p.cur.Type == RPAREN || p.cur.Type == EOF {
			break
		}
		if p.cur.Type == COMMA {
			continue
		}
		if p.cur.Type != IDENT {
			p.addError("expected parameter name", p.cur.Pos)
			break
		}
		params = append(params, p.cur.Lexeme)
	}

	// expect '{'
	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after function parameters", p.cur.Pos)
		return &FnDecl{Name: name, Params: params, PosInfo: startPos}
	}

	// parse body expression
	p.nextToken()
	body := p.parseExpr()

	// expect '}'
	p.nextToken()
	if p.cur.Type != RBRACE {
		p.addError("expected '}' after function body", p.cur.Pos)
	}

	return &FnDecl{
		Name:    name,
		Params:  params,
		Body:    body,
		PosInfo: startPos,
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
	labels := map[string]string{}
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
		switch key.Lexeme {
		case "address":
			expr := p.parseExpr()
			if s, ok := expr.(*StringLiteral); ok {
				addr = s
			} else {
				p.addError("address must be a string literal", expr.Pos())
			}
		case "labels":
			// v0.8: labels = { role = "web" env = "prod" }
			if p.cur.Type != LBRACE {
				p.addError("expected '{' for labels map", p.cur.Pos)
			} else {
				labels = p.parseStringMap()
			}
		default:
			// ignore unknown fields gracefully
			p.parseExpr()
		}
	}

	return &TargetDecl{
		Name:    nameTok.Lexeme,
		Address: addr,
		Labels:  labels,
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
		switch key {
		case "type":
			expr := p.parseExpr()
			if id, ok := expr.(*Ident); ok {
				node.Type = id
			} else {
				p.addError("type must be an identifier", expr.Pos())
			}
		case "targets":
			expr := p.parseExpr()
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
			expr := p.parseExpr()
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
			expr := p.parseExpr()
			if id, ok := expr.(*Ident); ok {
				node.FailurePolicy = id
			} else {
				p.addError("failure_policy must be an identifier", expr.Pos())
			}
		case "idempotent":
			expr := p.parseExpr()
			if b, ok := expr.(*BoolLiteral); ok {
				node.Idempotent = b
			} else {
				p.addError("idempotent must be a boolean literal (true or false)", expr.Pos())
			}
		case "side_effects":
			expr := p.parseExpr()
			if s, ok := expr.(*StringLiteral); ok {
				node.SideEffects = s
			} else {
				p.addError("side_effects must be a string literal", expr.Pos())
			}
		case "rollback_cmd":
			expr := p.parseExpr()
			if list, ok := expr.(*ListLiteral); ok {
				node.RollbackCmd = list
			} else {
				p.addError("rollback_cmd must be a list of string literals", expr.Pos())
			}
		case "retry":
			if p.cur.Type != LBRACE {
				p.addError("expected '{' after retry", p.cur.Pos)
			} else {
				node.Retry = p.parseRetryConfig()
			}
		default:
			// Primitive-specific inputs
			node.Inputs[key] = p.parseExpr()
		}
	}

	return node
}

func (p *Parser) parseRetryConfig() *RetryConfig {
	res := &RetryConfig{}
	
	// current is '{'
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type != IDENT {
			p.addError("expected identifier in retry block", p.cur.Pos)
			p.synchronize()
			break
		}
		key := p.cur.Lexeme
		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' after identifier in retry block", p.cur.Pos)
			p.synchronize()
			break
		}
		p.nextToken()
		expr := p.parseExpr()
		switch key {
		case "attempts":
			if n, ok := expr.(*NumberLiteral); ok {
				res.Attempts = n.Value
			} else {
				p.addError("attempts must be a number literal", expr.Pos())
			}
		case "delay":
			if s, ok := expr.(*StringLiteral); ok {
				res.Delay = s.Value
			} else {
				p.addError("delay must be a string literal", expr.Pos())
			}
		default:
			p.addError(fmt.Sprintf("unknown retry field %q", key), p.cur.Pos)
		}
	}
	
	return res
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
	p.nextToken()
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

// step "name" { param* node-body }
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

	// Phase 1: Parse param declarations (must come first)
	var params []*ParamDecl
	seenBodyField := false
	needNextToken := true
	for {
		if needNextToken {
			p.nextToken()
		}
		needNextToken = true  // Reset for next iteration
		
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			// Empty step body
			return &StepDecl{
				Name:    nameTok.Lexeme,
				Params:  params,
				Body:    &NodeDecl{Name: nameTok.Lexeme, Inputs: make(map[string]Expr), PosInfo: startPos},
				PosInfo: startPos,
			}
		}
		
		// Check if this is a param declaration
		if p.cur.Type == KW_PARAM {
			if seenBodyField {
				p.addError("param declarations must appear before step body fields", p.cur.Pos)
				p.synchronize()
				break
			}
			
			paramPos := p.cur.Pos
			p.nextToken()
			if p.cur.Type != IDENT {
				p.addError("expected parameter name identifier", p.cur.Pos)
				p.synchronize()
				continue
			}
			
			paramName := p.cur.Lexeme
			p.nextToken()
			
			var defaultExpr Expr
			if p.cur.Type == EQUAL {
				// Optional parameter with default
				p.nextToken()
				defaultExpr = p.parseExpr()
				p.nextToken()  // Move past expression
				needNextToken = false  // We've already advanced
			} else {
				// No default - p.cur is already at the next token
				needNextToken = false  // Don't advance again at loop start
			}
			
			params = append(params, &ParamDecl{
				Name:    paramName,
				Default: defaultExpr,
				PosInfo: paramPos,
			})
			continue
		}
		
		// Not a param, so we've entered the step body section
		seenBodyField = true
		break
	}

	// Phase 2: Parse step body (reuse node-body parsing)
	node := &NodeDecl{
		Name:    nameTok.Lexeme,
		Inputs:  make(map[string]Expr),
		PosInfo: startPos,
	}

	// p.cur is now positioned at the first body field identifier
	for {
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
		switch key {
		case "type":
			expr := p.parseExpr()
			if id, ok := expr.(*Ident); ok {
				node.Type = id
			} else {
				p.addError("type must be an identifier", expr.Pos())
			}
		case "targets":
			expr := p.parseExpr()
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
			expr := p.parseExpr()
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
			expr := p.parseExpr()
			if id, ok := expr.(*Ident); ok {
				node.FailurePolicy = id
			} else {
				p.addError("failure_policy must be an identifier", expr.Pos())
			}
		case "idempotent":
			expr := p.parseExpr()
			if b, ok := expr.(*BoolLiteral); ok {
				node.Idempotent = b
			} else {
				p.addError("idempotent must be a boolean literal (true or false)", expr.Pos())
			}
		case "side_effects":
			expr := p.parseExpr()
			if s, ok := expr.(*StringLiteral); ok {
				node.SideEffects = s
			} else {
				p.addError("side_effects must be a string literal", expr.Pos())
			}
		case "rollback_cmd":
			expr := p.parseExpr()
			if list, ok := expr.(*ListLiteral); ok {
				node.RollbackCmd = list
			} else {
				p.addError("rollback_cmd must be a list of string literals", expr.Pos())
			}
		case "retry":
			if p.cur.Type != LBRACE {
				p.addError("expected '{' after retry", p.cur.Pos)
			} else {
				node.Retry = p.parseRetryConfig()
			}
		default:
			node.Inputs[key] = p.parseExpr()
		}
		
		p.nextToken()
	}

	return &StepDecl{
		Name:    nameTok.Lexeme,
		Params:  params,
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

// primitive "name" { inputs { ... } body { ... } }
func (p *Parser) parsePrimitiveDecl() Decl {
	startPos := p.cur.Pos
	p.nextToken()
	if p.cur.Type != STRING {
		p.addError("expected primitive name string", p.cur.Pos)
		return &PrimitiveDecl{PosInfo: startPos}
	}
	name := p.cur.Lexeme

	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after primitive name", p.cur.Pos)
	}

	decl := &PrimitiveDecl{
		Name:    name,
		PosInfo: startPos,
	}

	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		
		switch p.cur.Type {
		case KW_INPUTS:
			decl.Inputs = p.parsePrimitiveInputs()
		case KW_PREPARE:
			decl.Prepare = p.parsePrimitivePrepare()
		case KW_BODY:
			decl.Body = p.parsePrimitiveBody()
		case KW_CONTRACT:
			decl.Contract = p.parsePrimitiveContract()
		case KW_PROBE:
			decl.Probe = p.parsePrimitiveProbe()
		case KW_DESIRED:
			decl.Desired = p.parsePrimitiveDesired()
		default:
			// Unknown field
			if p.cur.Type == IDENT {
				p.addError(fmt.Sprintf("unsupported primitive field %q", p.cur.Lexeme), p.cur.Pos)
				p.synchronize()
			} else {
				p.addError("expected 'inputs', 'prepare', 'body', 'contract', 'probe', or 'desired' in primitive", p.cur.Pos)
				p.synchronize()
			}
		}
	}

	return decl
}

func (p *Parser) parsePrimitiveInputs() []*PrimitiveInputDecl {
	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after 'inputs'", p.cur.Pos)
		return nil
	}
	
	var inputs []*PrimitiveInputDecl
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type == COMMA {
			continue
		}
		if p.cur.Type != IDENT {
			p.addError("expected input name identifier", p.cur.Pos)
			// don't synchronize here to avoid skipping the whole inputs block
			continue
		}
		name := p.cur.Lexeme
		pos := p.cur.Pos
		
		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' after input name", p.cur.Pos)
			continue
		}
		
		p.nextToken()
		if p.cur.Type != IDENT {
			p.addError("expected type identifier (e.g. string, bool, list)", p.cur.Pos)
			continue
		}
		typ := &Ident{Name: p.cur.Lexeme, PosInfo: p.cur.Pos}
		
		inputs = append(inputs, &PrimitiveInputDecl{
			Name:    name,
			Type:    typ,
			PosInfo: pos,
		})
	}
	return inputs
}

func (p *Parser) parsePrimitiveBody() []Decl {
	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after 'body'", p.cur.Pos)
		return nil
	}
	
	var body []Decl
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		// Handle foreach inside primitive body
		if p.cur.Type == KW_FOREACH {
			foreachDecl := p.parseForeachBodyDecl()
			if foreachDecl != nil {
				body = append(body, foreachDecl)
			}
			continue
		}
		decl := p.parseDecl()
		if decl != nil {
			body = append(body, decl)
		} else {
			// If parseDecl failed, synchronize but stay within the body block if possible
			// Actually parseDecl already synchronizes.
		}
	}
	return body
}

// parsePrimitiveContract parses the contract { ... } block in a primitive.
// Supports: idempotent (bool), side_effects (string), retry (int)
func (p *Parser) parsePrimitiveContract() *ContractDecl {
	startPos := p.cur.Pos
	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after 'contract'", p.cur.Pos)
		return nil
	}
	
	contract := &ContractDecl{PosInfo: startPos}
	
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type == COMMA {
			continue
		}
		if p.cur.Type != IDENT {
			p.addError("expected contract field name", p.cur.Pos)
			continue
		}
		fieldName := p.cur.Lexeme
		
		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' after contract field name", p.cur.Pos)
			continue
		}
		
		p.nextToken()
		switch fieldName {
		case "idempotent":
			if p.cur.Type == BOOL {
				val := p.cur.Lexeme == "true"
				contract.Idempotent = &val
			} else {
				p.addError("idempotent must be true or false", p.cur.Pos)
			}
		case "side_effects":
			if p.cur.Type == STRING {
				val := p.cur.Lexeme
				if val != "none" && val != "local" && val != "external" {
					p.addError("side_effects must be \"none\", \"local\", or \"external\"", p.cur.Pos)
				} else {
					contract.SideEffects = &val
				}
			} else {
				p.addError("side_effects must be a string", p.cur.Pos)
			}
		case "retry":
			if p.cur.Type == NUMBER {
				val, _ := strconv.Atoi(p.cur.Lexeme)
				contract.Retry = &val
			} else {
				p.addError("retry must be a number", p.cur.Pos)
			}
		default:
			p.addError(fmt.Sprintf("unknown contract field %q", fieldName), p.cur.Pos)
		}
	}
	
	return contract
}

// parsePrimitiveProbe parses the probe { ... } block in a primitive.
// Example: probe { exists = _fs.exists(inputs.path) }
func (p *Parser) parsePrimitiveProbe() *ProbeDecl {
	startPos := p.cur.Pos
	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after 'probe'", p.cur.Pos)
		return nil
	}
	
	probe := &ProbeDecl{PosInfo: startPos}
	
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type == COMMA {
			continue
		}
		if p.cur.Type != IDENT {
			p.addError("expected probe field name", p.cur.Pos)
			continue
		}
		fieldName := p.cur.Lexeme
		fieldPos := p.cur.Pos
		
		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' after probe field name", p.cur.Pos)
			continue
		}
		
		p.nextToken()
		expr := p.parseExpr()
		
		probe.Fields = append(probe.Fields, &ProbeField{
			Name:    fieldName,
			Expr:    expr,
			PosInfo: fieldPos,
		})
	}
	
	return probe
}

// parsePrimitiveDesired parses the desired { ... } block in a primitive.
// Example: desired { exists = true }
func (p *Parser) parsePrimitiveDesired() *DesiredDecl {
	startPos := p.cur.Pos
	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after 'desired'", p.cur.Pos)
		return nil
	}
	
	desired := &DesiredDecl{PosInfo: startPos}
	
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type == COMMA {
			continue
		}
		if p.cur.Type != IDENT {
			p.addError("expected desired field name", p.cur.Pos)
			continue
		}
		fieldName := p.cur.Lexeme
		fieldPos := p.cur.Pos
		
		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' after desired field name", p.cur.Pos)
			continue
		}
		
		p.nextToken()
		expr := p.parseExpr()
		
		desired.Fields = append(desired.Fields, &DesiredField{
			Name:    fieldName,
			Expr:    expr,
			PosInfo: fieldPos,
		})
	}
	
	return desired
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

// parsePrimary parses primary expressions: literals, identifiers, lists, secret() calls
func (p *Parser) parsePrimary() Expr {
	tok := p.cur
	switch tok.Type {
	case STRING:
		return &StringLiteral{Value: tok.Lexeme, PosInfo: tok.Pos}
	case BOOL:
		return &BoolLiteral{Value: tok.Lexeme == "true", PosInfo: tok.Pos}
	case NUMBER:
		val, _ := strconv.Atoi(tok.Lexeme)
		return &NumberLiteral{Value: val, PosInfo: tok.Pos}
	case IDENT:
		// v0.9: secret("KEY") — secret reference builtin.
		if tok.Lexeme == "secret" && p.peek.Type == LPAREN {
			return p.parseSecretRef(tok.Pos)
		}
		// v1.3: function call syntax like _fs.exists(path)
		if p.peek.Type == LPAREN {
			return p.parseFunctionCall(tok)
		}
		return &Ident{Name: tok.Lexeme, PosInfo: tok.Pos}
	case LBRACKET:
		return p.parseListLiteral()
	default:
		p.addError("unexpected token in expression", tok.Pos)
		return &StringLiteral{Value: "", PosInfo: tok.Pos}
	}
}

// parseSecretRef parses secret("KEY") after "secret" has been consumed.
// Current token is "secret", peek is LPAREN.
func (p *Parser) parseSecretRef(startPos Position) Expr {
	p.nextToken() // consume '('
	p.nextToken() // move to the key string
	if p.cur.Type != STRING {
		p.addError("expected secret key string in secret(\"KEY\")", p.cur.Pos)
		return &SecretRef{Key: "", PosInfo: startPos}
	}
	key := p.cur.Lexeme
	p.nextToken() // move to ')'
	if p.cur.Type != RPAREN {
		p.addError("expected ')' after secret key", p.cur.Pos)
	}
	return &SecretRef{Key: key, PosInfo: startPos}
}

// parseFunctionCall parses a function call like _fs.exists(path) or _fs.stat(inputs.path).
// Current token is the function name identifier, peek is LPAREN.
func (p *Parser) parseFunctionCall(nameTok Token) Expr {
	startPos := nameTok.Pos
	funcName := nameTok.Lexeme
	
	p.nextToken() // consume '('
	p.nextToken() // move to first argument or ')'
	
	var args []Expr
	for p.cur.Type != RPAREN && p.cur.Type != EOF {
		arg := p.parseExpr()
		args = append(args, arg)
		
		p.nextToken()
		if p.cur.Type == COMMA {
			p.nextToken()
			continue
		}
	}
	
	if p.cur.Type != RPAREN {
		p.addError("expected ')' after function arguments", p.cur.Pos)
	}
	
	return &FunctionCall{
		Name:    funcName,
		Args:    args,
		PosInfo: startPos,
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

// parsePrimitivePrepare parses the prepare { ... } block in a primitive.
// Example: prepare { files = _ctrl.readdir(inputs.src) }
func (p *Parser) parsePrimitivePrepare() *PrepareDecl {
	startPos := p.cur.Pos
	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' after 'prepare'", p.cur.Pos)
		return nil
	}
	
	prepare := &PrepareDecl{PosInfo: startPos}
	
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		if p.cur.Type == COMMA {
			continue
		}
		if p.cur.Type != IDENT {
			p.addError("expected variable name in prepare block", p.cur.Pos)
			continue
		}
		varName := p.cur.Lexeme
		varPos := p.cur.Pos
		
		p.nextToken()
		if p.cur.Type != EQUAL {
			p.addError("expected '=' after variable name in prepare block", p.cur.Pos)
			continue
		}
		
		p.nextToken()
		expr := p.parseExpr()
		
		prepare.Bindings = append(prepare.Bindings, &PrepareBinding{
			Name:    varName,
			Expr:    expr,
			PosInfo: varPos,
		})
	}
	
	return prepare
}

// parseForeachBodyDecl parses a foreach x in y { ... } block inside a primitive body.
// Example: foreach file in prepare.files { node "copy-${file.name}" { ... } }
func (p *Parser) parseForeachBodyDecl() *ForeachBodyDecl {
	startPos := p.cur.Pos
	
	// Current token is 'foreach'
	p.nextToken()
	if p.cur.Type != IDENT {
		p.addError("expected loop variable name after foreach", p.cur.Pos)
		return nil
	}
	varName := p.cur.Lexeme
	
	p.nextToken()
	if p.cur.Type != KW_IN {
		p.addError("expected 'in' after loop variable", p.cur.Pos)
		return nil
	}
	
	p.nextToken()
	rangeExpr := p.parseExpr()
	
	// expect '{'
	p.nextToken()
	if p.cur.Type != LBRACE {
		p.addError("expected '{' to start foreach body", p.cur.Pos)
		return nil
	}
	
	var body []Decl
	for {
		p.nextToken()
		if p.cur.Type == RBRACE || p.cur.Type == EOF {
			break
		}
		// Allow nested foreach
		if p.cur.Type == KW_FOREACH {
			nestedForeach := p.parseForeachBodyDecl()
			if nestedForeach != nil {
				body = append(body, nestedForeach)
			}
			continue
		}
		decl := p.parseDecl()
		if decl != nil {
			body = append(body, decl)
		}
	}
	
	return &ForeachBodyDecl{
		VarName: varName,
		Range:   rangeExpr,
		Body:    body,
		PosInfo: startPos,
	}
}
