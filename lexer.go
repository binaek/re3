package re3

type tokenType int

const (
	tokenError tokenType = iota
	tokenEOF
	tokenLiteral
	tokenUnion       // |
	tokenIntersect   // &
	tokenComplement  // ~
	tokenStar        // *
	tokenPlus        // +
	tokenQuestion    // ?
	tokenLParen      // (
	tokenRParen      // )
	tokenCharClass   // [...]
	tokenEscape      // \
	tokenDot         // .
	tokenLBrace      // {
	tokenRBrace      // }
	tokenNumber      // digits for {n,m}
	tokenComma       // ,
	tokenLookAhead   // (?=
	tokenLookBehind  // (?<=
	tokenNonCapParen // (?:
	tokenInlineFlags // (?i)
	tokenEmpty       // zero-width anchors
)

type token struct {
	Type  tokenType
	Value rune
	Text  string
}

type lexer struct {
	input    []rune
	pos      int
	lastType tokenType
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
	// case '&':
	// 	l.lastType = tokenIntersect
	// 	return token{Type: tokenIntersect}
	// case '~':
	// 	l.lastType = tokenComplement
	// 	return token{Type: tokenComplement}
	case '*':
		if l.pos < len(l.input) && l.input[l.pos] == '?' {
			l.pos++ // consume non-greedy modifier (re3 ignores it)
		}
		l.lastType = tokenStar
		return token{Type: tokenStar}
	case '+':
		if l.pos < len(l.input) && l.input[l.pos] == '?' {
			l.pos++
		}
		l.lastType = tokenPlus
		return token{Type: tokenPlus}
	case '?':
		if l.pos < len(l.input) && l.input[l.pos] == '?' {
			l.pos++
		}
		l.lastType = tokenQuestion
		return token{Type: tokenQuestion}
	case '^', '$':
		l.lastType = tokenEmpty
		return token{Type: tokenEmpty}
	case '(':
		if l.pos < len(l.input) && l.input[l.pos] == '?' {
			start := l.pos - 1
			l.pos++
			if l.pos < len(l.input) && l.input[l.pos] == ':' {
				l.pos++
				l.lastType = tokenNonCapParen
				return token{Type: tokenNonCapParen}
			}
			if l.pos < len(l.input) && l.input[l.pos] == '=' {
				l.pos++
				l.lastType = tokenLookAhead
				return token{Type: tokenLookAhead}
			}
			if l.pos+1 < len(l.input) && l.input[l.pos] == '<' && l.input[l.pos+1] == '=' {
				l.pos += 2
				l.lastType = tokenLookBehind
				return token{Type: tokenLookBehind}
			}
			// Named Capture Group (?P<name>...)
			if l.pos < len(l.input) && (l.input[l.pos] == 'P' || l.input[l.pos] == '<') {
				if l.input[l.pos] == 'P' {
					l.pos++
				}
				if l.pos < len(l.input) && l.input[l.pos] == '<' {
					l.pos++
					for l.pos < len(l.input) && l.input[l.pos] != '>' {
						l.pos++
					}
					if l.pos < len(l.input) && l.input[l.pos] == '>' {
						l.pos++
						l.lastType = tokenLParen
						return token{Type: tokenLParen}
					}
				}
			}
			// Inline Flags (?i) or (?s:...)
			tempPos := l.pos
			isFlags := true
			for tempPos < len(l.input) {
				c := l.input[tempPos]
				if c == ':' {
					l.pos = tempPos + 1
					l.lastType = tokenNonCapParen
					return token{Type: tokenNonCapParen}
				}
				if c == ')' {
					l.pos = tempPos + 1
					l.lastType = tokenInlineFlags
					return token{Type: tokenInlineFlags}
				}
				if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-' {
					tempPos++
				} else {
					isFlags = false
					break
				}
			}
			if !isFlags {
				l.pos = start + 1
			}
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
		if l.isLookaheadRepeat() {
			l.lastType = tokenLBrace
			return token{Type: tokenLBrace}
		}
		l.lastType = tokenLiteral
		return token{Type: tokenLiteral, Value: '{'}
	case '}':
		if l.lastType == tokenNumber || l.lastType == tokenComma || l.lastType == tokenLBrace {
			if l.pos < len(l.input) && l.input[l.pos] == '?' {
				l.pos++
			}
			l.lastType = tokenRBrace
			return token{Type: tokenRBrace}
		}
		l.lastType = tokenLiteral
		return token{Type: tokenLiteral, Value: '}'}
	case ',':
		if l.lastType == tokenNumber || l.lastType == tokenLBrace {
			l.lastType = tokenComma
			return token{Type: tokenComma}
		}
		l.lastType = tokenLiteral
		return token{Type: tokenLiteral, Value: ','}
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
		if escaped == 'b' || escaped == 'B' || escaped == 'A' || escaped == 'z' || escaped == 'Z' {
			l.lastType = tokenEmpty
			return token{Type: tokenEmpty}
		}
		if escaped == 'p' || escaped == 'P' {
			start := l.pos - 2
			if l.pos < len(l.input) && l.input[l.pos] == '{' {
				l.pos++
				for l.pos < len(l.input) && l.input[l.pos] != '}' {
					l.pos++
				}
				if l.pos < len(l.input) && l.input[l.pos] == '}' {
					l.pos++
					l.lastType = tokenCharClass
					return token{Type: tokenCharClass, Text: string(l.input[start:l.pos])}
				}
			} else if l.pos < len(l.input) {
				l.pos++
				l.lastType = tokenCharClass
				return token{Type: tokenCharClass, Text: string(l.input[start:l.pos])}
			}
		}
		l.lastType = tokenEscape
		return token{Type: tokenEscape, Value: escaped}
	default:
		l.lastType = tokenLiteral
		return token{Type: tokenLiteral, Value: ch}
	}
}

func (l *lexer) isLookaheadRepeat() bool {
	pos := l.pos
	hasDigit := false
	for pos < len(l.input) && l.input[pos] >= '0' && l.input[pos] <= '9' {
		hasDigit = true
		pos++
	}
	if !hasDigit {
		return false
	}
	if pos < len(l.input) && l.input[pos] == ',' {
		pos++
		for pos < len(l.input) && l.input[pos] >= '0' && l.input[pos] <= '9' {
			pos++
		}
	}
	return pos < len(l.input) && l.input[pos] == '}'
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
