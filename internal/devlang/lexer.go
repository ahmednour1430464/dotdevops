package devlang

// TokenType represents different kinds of tokens in the .devops language.
type TokenType int

const (
	// Special tokens
	EOF TokenType = iota
	ILLEGAL

	// Identifiers & literals
	IDENT  // foo, file.sync
	STRING // "..."
	BOOL   // true, false
	NUMBER // 123

	// Keywords
	KW_TARGET
	KW_NODE
	KW_LET
	KW_MODULE
	KW_STEP
	KW_FOR
	KW_IN
	KW_PARAM
	KW_VERSION   // v0.7: self-declared version directive
	KW_FLEET     // v0.8: named group of targets by label selector
	KW_PRIMITIVE // v1.2: custom primitive definition
	KW_INPUTS
	KW_BODY
	KW_CONTRACT
	KW_PROBE   // v1.3: probe block in primitive
	KW_DESIRED // v1.3: desired state block in primitive
	KW_PREPARE // v1.4: controller-side preparation block
	KW_FOREACH // v1.4: compile-time iteration over lists
	KW_IMPORT  // v2.0: import declarations from other files
	KW_FN      // v2.0: user-defined functions
	KW_AS      // v2.0: namespace alias for imports

	// Operators & punctuation
	EQUAL    // =
	LBRACE   // {
	RBRACE   // }
	LBRACKET // [
	RBRACKET // ]
	LPAREN   // (
	RPAREN   // )
	COMMA    // ,
	PLUS     // +
	AMPAMP   // &&
	PIPEPIPE // ||
	EQEQ     // ==
	BANGEQ   // !=
	QUESTION // ?
	COLON    // :
)

// Token represents a single lexical token.
type Token struct {
	Type   TokenType
	Lexeme string
	Pos    Position
}

// Lexer converts source bytes into a stream of tokens.
type Lexer struct {
	src  []rune
	pos  int
	line int
	col  int
}

// NewLexer creates a new lexer from raw source.
func NewLexer(src []byte) *Lexer {
	return &Lexer{
		src:  []rune(string(src)),
		pos:  0,
		line: 1,
		col:  1,
	}
}

// NextToken scans and returns the next token.
func (l *Lexer) NextToken() Token {
	l.skipWhitespaceAndComments()

	if l.pos >= len(l.src) {
		return Token{Type: EOF, Pos: Position{Line: l.line, Col: l.col}}
	}

	ch := l.peek()
	startPos := Position{Line: l.line, Col: l.col}

	switch ch {
	case '{':
		l.advance()
		return Token{Type: LBRACE, Lexeme: "{", Pos: startPos}
	case '}':
		l.advance()
		return Token{Type: RBRACE, Lexeme: "}", Pos: startPos}
	case '[':
		l.advance()
		return Token{Type: LBRACKET, Lexeme: "[", Pos: startPos}
	case ']':
		l.advance()
		return Token{Type: RBRACKET, Lexeme: "]", Pos: startPos}
	case '(':
		l.advance()
		return Token{Type: LPAREN, Lexeme: "(", Pos: startPos}
	case ')':
		l.advance()
		return Token{Type: RPAREN, Lexeme: ")", Pos: startPos}
	case '=':
		l.advance()
		// Check for ==
		if l.peek() == '=' {
			l.advance()
			return Token{Type: EQEQ, Lexeme: "==", Pos: startPos}
		}
		return Token{Type: EQUAL, Lexeme: "=", Pos: startPos}
	case ',':
		l.advance()
		return Token{Type: COMMA, Lexeme: ",", Pos: startPos}
	case '+':
		l.advance()
		return Token{Type: PLUS, Lexeme: "+", Pos: startPos}
	case '?':
		l.advance()
		return Token{Type: QUESTION, Lexeme: "?", Pos: startPos}
	case ':':
		l.advance()
		return Token{Type: COLON, Lexeme: ":", Pos: startPos}
	case '&':
		l.advance()
		if l.peek() == '&' {
			l.advance()
			return Token{Type: AMPAMP, Lexeme: "&&", Pos: startPos}
		}
		return Token{Type: ILLEGAL, Lexeme: "&", Pos: startPos}
	case '|':
		l.advance()
		if l.peek() == '|' {
			l.advance()
			return Token{Type: PIPEPIPE, Lexeme: "||", Pos: startPos}
		}
		return Token{Type: ILLEGAL, Lexeme: "|", Pos: startPos}
	case '!':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: BANGEQ, Lexeme: "!=", Pos: startPos}
		}
		return Token{Type: ILLEGAL, Lexeme: "!", Pos: startPos}
	case '"':
		return l.readString()
	}

	if isDigit(ch) {
		return l.readNumber()
	}

	if isLetter(ch) || ch == '_' {
		return l.readIdentOrKeyword()
	}

	// Unknown character
	l.advance()
	return Token{Type: ILLEGAL, Lexeme: string(ch), Pos: startPos}
}

func (l *Lexer) readNumber() Token {
	startPos := Position{Line: l.line, Col: l.col}
	start := l.pos
	for isDigit(l.peek()) {
		l.advance()
	}
	return Token{Type: NUMBER, Lexeme: string(l.src[start:l.pos]), Pos: startPos}
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) advance() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) skipWhitespaceAndComments() {
	for {
		if l.pos >= len(l.src) {
			return
		}
		ch := l.peek()
		// Whitespace
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			l.advance()
			continue
		}
		// Line comments: // ...
		if ch == '/' && l.peekNext() == '/' {
			// consume '//' and then until end of line
			l.advance()
			l.advance()
			for {
				if l.pos >= len(l.src) {
					return
				}
				if l.peek() == '\n' {
					l.advance()
					break
				}
				l.advance()
			}
			continue
		}
		return
	}
}

func (l *Lexer) peekNext() rune {
	if l.pos+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+1]
}

func (l *Lexer) readString() Token {
	startPos := Position{Line: l.line, Col: l.col}
	// Consume opening quote
	l.advance()

	var runes []rune
	for {
		if l.pos >= len(l.src) {
			return Token{Type: ILLEGAL, Lexeme: "unterminated string", Pos: startPos}
		}
		ch := l.advance()
		if ch == '"' {
			break
		}
		if ch == '\\' {
			if l.pos >= len(l.src) {
				return Token{Type: ILLEGAL, Lexeme: "unterminated escape sequence", Pos: startPos}
			}
			next := l.advance()
			switch next {
			case '"', '\\':
				runes = append(runes, next)
			case 'n':
				runes = append(runes, '\n')
			case 't':
				runes = append(runes, '\t')
			default:
				// Unknown escape, keep as-is
				runes = append(runes, next)
			}
			continue
		}
		runes = append(runes, ch)
	}

	return Token{Type: STRING, Lexeme: string(runes), Pos: startPos}
}

func (l *Lexer) readIdentOrKeyword() Token {
	startPos := Position{Line: l.line, Col: l.col}
	start := l.pos
	for {
		if l.pos >= len(l.src) {
			break
		}
		ch := l.peek()
		if isLetter(ch) || isDigit(ch) || ch == '_' || ch == '.' {
			l.advance()
			continue
		}
		break
	}
	lexeme := string(l.src[start:l.pos])

	// Keywords
	switch lexeme {
	case "target":
		return Token{Type: KW_TARGET, Lexeme: lexeme, Pos: startPos}
	case "node":
		return Token{Type: KW_NODE, Lexeme: lexeme, Pos: startPos}
	case "let":
		return Token{Type: KW_LET, Lexeme: lexeme, Pos: startPos}
	case "module":
		return Token{Type: KW_MODULE, Lexeme: lexeme, Pos: startPos}
	case "step":
		return Token{Type: KW_STEP, Lexeme: lexeme, Pos: startPos}
	case "for":
		return Token{Type: KW_FOR, Lexeme: lexeme, Pos: startPos}
	case "in":
		return Token{Type: KW_IN, Lexeme: lexeme, Pos: startPos}
	case "param":
		return Token{Type: KW_PARAM, Lexeme: lexeme, Pos: startPos}
	case "version":
		return Token{Type: KW_VERSION, Lexeme: lexeme, Pos: startPos}
	case "fleet":
		return Token{Type: KW_FLEET, Lexeme: lexeme, Pos: startPos}
	case "primitive":
		return Token{Type: KW_PRIMITIVE, Lexeme: lexeme, Pos: startPos}
	case "inputs":
		// Only treat as keyword if it's likely a block start in a primitive/step
		// For now, always treat as keyword to simplify parsing top-level blocks.
		return Token{Type: KW_INPUTS, Lexeme: lexeme, Pos: startPos}
	case "body":
		return Token{Type: KW_BODY, Lexeme: lexeme, Pos: startPos}
	case "contract":
		return Token{Type: KW_CONTRACT, Lexeme: lexeme, Pos: startPos}
	case "probe":
		return Token{Type: KW_PROBE, Lexeme: lexeme, Pos: startPos}
	case "desired":
		return Token{Type: KW_DESIRED, Lexeme: lexeme, Pos: startPos}
	case "prepare":
		return Token{Type: KW_PREPARE, Lexeme: lexeme, Pos: startPos}
	case "foreach":
		return Token{Type: KW_FOREACH, Lexeme: lexeme, Pos: startPos}
	case "import":
		return Token{Type: KW_IMPORT, Lexeme: lexeme, Pos: startPos}
	case "fn":
		return Token{Type: KW_FN, Lexeme: lexeme, Pos: startPos}
	case "as":
		return Token{Type: KW_AS, Lexeme: lexeme, Pos: startPos}
	case "true", "false":
		return Token{Type: BOOL, Lexeme: lexeme, Pos: startPos}
	}

	return Token{Type: IDENT, Lexeme: lexeme, Pos: startPos}
}

func isLetter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}
