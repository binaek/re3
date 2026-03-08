package re3

type tokenType int

const (
	tokenError tokenType = iota
	tokenEOF
	tokenLiteral
	tokenUnion      // |
	tokenIntersect  // &
	tokenComplement // ~
	tokenStar       // *
	tokenPlus       // +
	tokenQuestion   // ?
	tokenLParen     // (
	tokenRParen     // )
	tokenCharClass  // [...]
	tokenEscape     // \
	tokenDot        // .
	tokenLBrace     // {
	tokenRBrace     // }
	tokenNumber     // digits for {n,m}
	tokenComma      // ,
	tokenLookAhead  // (?=
	tokenLookBehind // (?<=
)

type token struct {
	Type  tokenType
	Value rune
	Text  string
}

type lexer struct {
	input     []rune
	pos       int
	lastType  tokenType
}

func newLexer(input string) *lexer {
	return &lexer{input: []rune(input), pos: 0}
}

func (l *lexer) nextToken() token {
	if l.pos >= len(l.input) {
		return token{Type: tokenEOF}
	}
	ch := l.input[l.pos]
	l.pos++

	switch ch {
	case '|':
		l.lastType = tokenUnion
		return token{Type: tokenUnion}
	case '&':
		l.lastType = tokenIntersect
		return token{Type: tokenIntersect}
	case '~':
		l.lastType = tokenComplement
		return token{Type: tokenComplement}
	case '*':
		l.lastType = tokenStar
		return token{Type: tokenStar}
	case '+':
		l.lastType = tokenPlus
		return token{Type: tokenPlus}
	case '?':
		l.lastType = tokenQuestion
		return token{Type: tokenQuestion}
	case '(':
		if l.pos+1 < len(l.input) && l.input[l.pos] == '?' {
			l.pos++
			if l.input[l.pos] == '=' {
				l.pos++
				l.lastType = tokenLookAhead
				return token{Type: tokenLookAhead}
			}
			if l.pos+1 < len(l.input) && l.input[l.pos] == '<' && l.input[l.pos+1] == '=' {
				l.pos += 2
				l.lastType = tokenLookBehind
				return token{Type: tokenLookBehind}
			}
			l.pos--
		}
		l.lastType = tokenLParen
		return token{Type: tokenLParen}
	case ')':
		l.lastType = tokenRParen
		return token{Type: tokenRParen}
	case '.':
		l.lastType = tokenDot
		return token{Type: tokenDot}
	case '{':
		l.lastType = tokenLBrace
		return token{Type: tokenLBrace}
	case '}':
		l.lastType = tokenRBrace
		return token{Type: tokenRBrace}
	case ',':
		l.lastType = tokenComma
		return token{Type: tokenComma}
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		if l.lastType == tokenLBrace || l.lastType == tokenComma {
			l.pos--
			return l.lexNumber()
		}
		l.lastType = tokenLiteral
		return token{Type: tokenLiteral, Value: ch}
	case '[':
		t := l.lexCharacterClass()
		l.lastType = t.Type
		return t
	case '\\':
		if l.pos >= len(l.input) {
			return token{Type: tokenError, Text: "trailing backslash"}
		}
		escaped := l.input[l.pos]
		l.pos++
		l.lastType = tokenEscape
		return token{Type: tokenEscape, Value: escaped}
	default:
		l.lastType = tokenLiteral
		return token{Type: tokenLiteral, Value: ch}
	}
}

func (l *lexer) lexNumber() token {
	start := l.pos
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch >= '0' && ch <= '9' {
			l.pos++
		} else {
			break
		}
	}
	l.lastType = tokenNumber
	return token{Type: tokenNumber, Text: string(l.input[start:l.pos])}
}

func (l *lexer) lexCharacterClass() token {
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
			return token{Type: tokenCharClass, Text: string(l.input[start : l.pos-1])}
		}
	}
	return token{Type: tokenError, Text: "unclosed character class"}
}

func (l *lexer) lexAll() []token {
	var tokens []token
	for {
		tok := l.nextToken()
		tokens = append(tokens, tok)
		if tok.Type == tokenEOF || tok.Type == tokenError {
			break
		}
	}
	return tokens
}
