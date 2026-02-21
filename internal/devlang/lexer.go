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

	// Keywords
	KW_TARGET
	KW_NODE
	KW_LET
	KW_MODULE
	KW_STEP
	KW_FOR
	KW_IN

	// Operators & punctuation
	EQUAL     // =
	LBRACE    // {
	RBRACE    // }
	LBRACKET  // [
	RBRACKET  // ]
	COMMA     // ,
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
	case '=':
		l.advance()
		return Token{Type: EQUAL, Lexeme: "=", Pos: startPos}
	case ',':
		l.advance()
		return Token{Type: COMMA, Lexeme: ",", Pos: startPos}
	case '"':
		return l.readString()
	}

	if isLetter(ch) || ch == '_' {
		return l.readIdentOrKeyword()
	}

	// Unknown character
	l.advance()
	return Token{Type: ILLEGAL, Lexeme: string(ch), Pos: startPos}
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
