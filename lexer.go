package re3

type TokenType int

const (
	TokenError TokenType = iota
	TokenEOF
	TokenLiteral
	TokenUnion      // |
	TokenIntersect  // &
	TokenComplement // ~
	TokenStar       // *
	TokenPlus       // +
	TokenQuestion   // ?
	TokenLParen     // (
	TokenRParen     // )
	TokenCharClass  // [...]
	TokenEscape     // \
	TokenDot        // .
)

type Token struct {
	Type  TokenType
	Value rune
	Text  string
}

type Lexer struct {
	input []rune
	pos   int
}

func NewLexer(input string) *Lexer {
	return &Lexer{input: []rune(input), pos: 0}
}

func (l *Lexer) NextToken() Token {
	if l.pos >= len(l.input) {
		return Token{Type: TokenEOF}
	}
	ch := l.input[l.pos]
	l.pos++

	switch ch {
	case '|':
		return Token{Type: TokenUnion}
	case '&':
		return Token{Type: TokenIntersect}
	case '~':
		return Token{Type: TokenComplement}
	case '*':
		return Token{Type: TokenStar}
	case '+':
		return Token{Type: TokenPlus}
	case '?':
		return Token{Type: TokenQuestion}
	case '(':
		return Token{Type: TokenLParen}
	case ')':
		return Token{Type: TokenRParen}
	case '.':
		return Token{Type: TokenDot}
	case '[':
		return l.lexCharacterClass()
	case '\\':
		if l.pos >= len(l.input) {
			return Token{Type: TokenError, Text: "trailing backslash"}
		}
		escaped := l.input[l.pos]
		l.pos++
		return Token{Type: TokenEscape, Value: escaped}
	default:
		return Token{Type: TokenLiteral, Value: ch}
	}
}

func (l *Lexer) lexCharacterClass() Token {
	start := l.pos
	escaped := false
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		l.pos++
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == ']' {
			return Token{Type: TokenCharClass, Text: string(l.input[start : l.pos-1])}
		}
	}
	return Token{Type: TokenError, Text: "unclosed character class"}
}

func (l *Lexer) LexAll() []Token {
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF || tok.Type == TokenError {
			break
		}
	}
	return tokens
}
