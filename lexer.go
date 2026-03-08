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
	TokenLBrace     // {
	TokenRBrace     // }
	TokenNumber     // digits for {n,m}
	TokenComma      // ,
	TokenLookAhead  // (?=
	TokenLookBehind // (?<=
)

type Token struct {
	Type  TokenType
	Value rune
	Text  string
}

type Lexer struct {
	input     []rune
	pos       int
	lastType  TokenType
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
		l.lastType = TokenUnion
		return Token{Type: TokenUnion}
	case '&':
		l.lastType = TokenIntersect
		return Token{Type: TokenIntersect}
	case '~':
		l.lastType = TokenComplement
		return Token{Type: TokenComplement}
	case '*':
		l.lastType = TokenStar
		return Token{Type: TokenStar}
	case '+':
		l.lastType = TokenPlus
		return Token{Type: TokenPlus}
	case '?':
		l.lastType = TokenQuestion
		return Token{Type: TokenQuestion}
	case '(':
		if l.pos+1 < len(l.input) && l.input[l.pos] == '?' {
			l.pos++
			if l.input[l.pos] == '=' {
				l.pos++
				l.lastType = TokenLookAhead
				return Token{Type: TokenLookAhead}
			}
			if l.pos+1 < len(l.input) && l.input[l.pos] == '<' && l.input[l.pos+1] == '=' {
				l.pos += 2
				l.lastType = TokenLookBehind
				return Token{Type: TokenLookBehind}
			}
			l.pos--
		}
		l.lastType = TokenLParen
		return Token{Type: TokenLParen}
	case ')':
		l.lastType = TokenRParen
		return Token{Type: TokenRParen}
	case '.':
		l.lastType = TokenDot
		return Token{Type: TokenDot}
	case '{':
		l.lastType = TokenLBrace
		return Token{Type: TokenLBrace}
	case '}':
		l.lastType = TokenRBrace
		return Token{Type: TokenRBrace}
	case ',':
		l.lastType = TokenComma
		return Token{Type: TokenComma}
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		if l.lastType == TokenLBrace || l.lastType == TokenComma {
			l.pos--
			return l.lexNumber()
		}
		l.lastType = TokenLiteral
		return Token{Type: TokenLiteral, Value: ch}
	case '[':
		t := l.lexCharacterClass()
		l.lastType = t.Type
		return t
	case '\\':
		if l.pos >= len(l.input) {
			return Token{Type: TokenError, Text: "trailing backslash"}
		}
		escaped := l.input[l.pos]
		l.pos++
		l.lastType = TokenEscape
		return Token{Type: TokenEscape, Value: escaped}
	default:
		l.lastType = TokenLiteral
		return Token{Type: TokenLiteral, Value: ch}
	}
}

func (l *Lexer) lexNumber() Token {
	start := l.pos
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch >= '0' && ch <= '9' {
			l.pos++
		} else {
			break
		}
	}
	l.lastType = TokenNumber
	return Token{Type: TokenNumber, Text: string(l.input[start:l.pos])}
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
