package re3

import (
	"sort"
	"strconv"
	"unicode"
	"unicode/utf8"
)

const (
	_ int = iota
	LOWEST
	UNION
	INTERSECT
	CONCAT
	PREFIX
	POSTFIX
)

type parser struct {
	tokens          []token
	pos             int
	curToken        token
	peekToken       token
	groupCount      int
	expr            string
	caseInsensitive bool
	dotAll          bool
	unicodeMode     bool
	prefixParseFns  map[tokenType]func() (node, error)
	infixParseFns   map[tokenType]func(node) (node, error)
}

func newParser(tokens []token, expr string) *parser {
	p := &parser{tokens: tokens, pos: -1, expr: expr}
	p.prefixParseFns = map[tokenType]func() (node, error){
		tokenLiteral:                p.parseLiteral,
		tokenEscape:                 p.parseEscape,
		tokenCharClass:              p.parseCharClass,
		tokenLParen:                 p.parseGroup,
		tokenDot:                    p.parseDot,
		tokenLookAhead:              p.parseLookAhead,
		tokenLookBehind:             p.parseLookBehind,
		tokenNonCapParen:            p.parseNonCapGroup,
		tokenInlineFlags:            p.parseInlineFlags,
		tokenEmpty:                  func() (node, error) { return &emptyNode{}, nil },
		tokenStart:                  func() (node, error) { return &startNode{}, nil },
		tokenEnd:                    func() (node, error) { return &endNode{}, nil },
		tokenWordBoundary: func() (node, error) {
			return &wordBoundaryNode{Unicode: p.unicodeMode}, nil
		},
		tokenNotWordBoundary: func() (node, error) {
			return &notWordBoundaryNode{Unicode: p.unicodeMode}, nil
		},
		tokenBeginText:              func() (node, error) { return &beginTextNode{}, nil },
		tokenEndText:                func() (node, error) { return &endTextNode{}, nil },
		tokenEndTextOptionalNewline: func() (node, error) { return &endTextOptionalNewlineNode{}, nil },
		tokenComma:                  func() (node, error) { return lowerRuneLiteral(','), nil },
		tokenLBrace:                 func() (node, error) { return lowerRuneLiteral('{'), nil },
		tokenRBrace:                 func() (node, error) { return lowerRuneLiteral('}'), nil },
		tokenUnion:                  p.parseEmptyLeftUnion,
	}
	p.infixParseFns = map[tokenType]func(node) (node, error){
		tokenUnion:     p.parseUnion,
		tokenIntersect: p.parseIntersect,
		tokenStar:      p.parseStar,
		tokenPlus:      p.parsePlus,
		tokenQuestion:  p.parseQuestion,
		tokenLBrace:    p.parseBoundedRepeat,
	}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *parser) nextToken() {
	p.curToken = p.peekToken
	p.pos++
	if p.pos < len(p.tokens) {
		p.peekToken = p.tokens[p.pos]
	} else {
		p.peekToken = token{Type: tokenEOF}
	}
}

func (p *parser) parse() (node, error) {
	if p.curToken.Type == tokenEOF {
		return &emptyNode{}, nil
	}
	return p.parseExpression(LOWEST)
}

func (p *parser) parseExpression(precedence int) (node, error) {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		if p.curToken.Type == tokenStar || p.curToken.Type == tokenPlus || p.curToken.Type == tokenQuestion {
			return nil, &Error{Code: ErrMissingRepeatArgument, Expr: p.expr}
		}
		return nil, &Error{Code: ErrInternalError, Expr: p.expr}
	}
	leftExp, err := prefix()
	if err != nil {
		return nil, err
	}

	for p.peekToken.Type != tokenEOF && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			if p.isPeekStartOfExpression() {
				leftExp, err = p.parseImplicitConcat(leftExp)
				if err != nil {
					return nil, err
				}
				continue
			}
			return leftExp, nil
		}
		p.nextToken()
		leftExp, err = infix(leftExp)
		if err != nil {
			return nil, err
		}
	}
	return leftExp, nil
}

// --- PARSING HANDLERS ---
func (p *parser) parseLiteral() (node, error) {
	return p.literalRuneNode(p.curToken.Value), nil
}

func (p *parser) parseEscape() (node, error) {
	val := p.curToken.Value
	switch val {
	case 'd', 'w', 's', 'D', 'W', 'S':
		if p.unicodeMode {
			return unicodeEscapeNode(val), nil
		}
		return &charClassNode{Class: "\\" + string(val)}, nil
	case 'n':
		return p.literalRuneNode('\n'), nil
	case 'r':
		return p.literalRuneNode('\r'), nil
	case 't':
		return p.literalRuneNode('\t'), nil
	case 'v':
		return p.literalRuneNode('\v'), nil
	case 'f':
		return p.literalRuneNode('\f'), nil
	case 'a':
		return p.literalRuneNode('\a'), nil
	}
	return p.literalRuneNode(val), nil
}
func (p *parser) parseCharClass() (node, error) {
	return compileCharClassNode(p.curToken.Text, p.caseInsensitive, p.unicodeMode), nil
}
func (p *parser) parseComplement() (node, error) {
	p.nextToken()
	child, err := p.parseExpression(PREFIX)
	if err != nil {
		return nil, err
	}
	return newComplementNode(child), nil
}
func (p *parser) parseGroup() (node, error) {
	p.nextToken()
	p.groupCount++
	id := p.groupCount

	if p.curToken.Type == tokenRParen {
		return &groupNode{GroupID: id, Child: &emptyNode{}}, nil
	}

	exp, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != tokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return &groupNode{GroupID: id, Child: exp}, nil
}

func (p *parser) parseNonCapGroup() (node, error) {
	prevCaseInsensitive := p.caseInsensitive
	prevDotAll := p.dotAll
	prevUnicodeMode := p.unicodeMode
	p.applyInlineFlags(p.curToken.Text)
	defer func() {
		p.caseInsensitive = prevCaseInsensitive
		p.dotAll = prevDotAll
		p.unicodeMode = prevUnicodeMode
	}()

	p.nextToken()

	if p.curToken.Type == tokenRParen {
		return &emptyNode{}, nil
	}

	exp, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != tokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return exp, nil
}
func (p *parser) parseUnion(left node) (node, error) {
	acc := left
	for {
		if p.peekToken.Type == tokenRParen || p.peekToken.Type == tokenUnion || p.peekToken.Type == tokenEOF {
			acc = newUnionNode(acc, &emptyNode{})
			if p.peekToken.Type == tokenUnion {
				// Consume repeated empty alternations like a||b iteratively.
				p.nextToken()
				continue
			}
			return acc, nil
		}
		p.nextToken()
		right, err := p.parseExpression(UNION)
		if err != nil {
			return nil, err
		}
		acc = newUnionNode(acc, right)
		if p.peekToken.Type != tokenUnion {
			return acc, nil
		}
		// Consume the next '|' and continue parsing in an iterative loop.
		p.nextToken()
	}
}
func (p *parser) parseIntersect(left node) (node, error) {
	p.nextToken()
	right, err := p.parseExpression(INTERSECT)
	if err != nil {
		return nil, err
	}
	return newIntersectNode(left, right), nil
}
func (p *parser) parseStar(left node) (node, error) { return &starNode{Child: left}, nil }
func (p *parser) parsePlus(left node) (node, error) {
	return newConcatNode(left, &starNode{Child: left}), nil
}
func (p *parser) parseQuestion(left node) (node, error) { return newUnionNode(left, &emptyNode{}), nil }

func (p *parser) parseBoundedRepeat(left node) (node, error) {
	p.nextToken() // curToken is now the number
	n, _ := strconv.Atoi(p.curToken.Text)
	p.nextToken() // curToken is now ',' or '}'

	if p.curToken.Type == tokenComma {
		p.nextToken() // curToken is now number or '}'
		if p.curToken.Type == tokenRBrace {
			// e.g. {n,} -> Repeat exact `n` times, followed by a Star
			if n == 0 {
				return &starNode{Child: left}, nil
			}
			return newConcatNode(newRepeatNode(left, n, n), &starNode{Child: left}), nil
		}

		m, _ := strconv.Atoi(p.curToken.Text)
		p.nextToken() // curToken is now '}'

		if n > m {
			return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
		}
		return newRepeatNode(left, n, m), nil
	}

	// Exact repeat {n}
	return newRepeatNode(left, n, n), nil
}

func (p *parser) parseEmptyLeftUnion() (node, error) {
	if p.peekToken.Type == tokenRParen || p.peekToken.Type == tokenUnion || p.peekToken.Type == tokenEOF {
		return newUnionNode(&emptyNode{}, &emptyNode{}), nil
	}
	p.nextToken()
	right, err := p.parseExpression(UNION)
	if err != nil {
		return nil, err
	}
	return newUnionNode(&emptyNode{}, right), nil
}

func (p *parser) isPeekStartOfExpression() bool {
	t := p.peekToken.Type
	return t == tokenLiteral || t == tokenLParen || t == tokenNonCapParen || t == tokenComplement ||
		t == tokenCharClass || t == tokenEscape || t == tokenDot ||
		t == tokenLookAhead || t == tokenLookBehind || t == tokenInlineFlags || t == tokenEmpty ||
		t == tokenStart || t == tokenEnd || t == tokenWordBoundary || t == tokenNotWordBoundary ||
		t == tokenBeginText || t == tokenEndText || t == tokenEndTextOptionalNewline ||
		t == tokenComma || t == tokenLBrace || t == tokenRBrace
}
func (p *parser) parseImplicitConcat(left node) (node, error) {
	p.nextToken()
	right, err := p.parseExpression(CONCAT)
	if err != nil {
		return nil, err
	}
	return newConcatNode(left, right), nil
}
func (p *parser) parseDot() (node, error) {
	if p.unicodeMode {
		if p.dotAll {
			return newAnyRuneNode(false), nil
		}
		return newAnyNode(), nil
	}
	if p.dotAll {
		return &anyByteNode{}, nil
	}
	return &anyNode{}, nil
}

func (p *parser) parseInlineFlags() (node, error) {
	p.applyInlineFlags(p.curToken.Text)
	return &emptyNode{}, nil
}

func (p *parser) parseLookAhead() (node, error) {
	p.nextToken()
	child, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != tokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return &lookAheadNode{Child: child}, nil
}

func (p *parser) parseLookBehind() (node, error) {
	p.nextToken()
	child, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != tokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return &lookBehindNode{Child: child}, nil
}
func (p *parser) peekPrecedence() int {
	if p.isPeekStartOfExpression() {
		return CONCAT
	}
	precedences := map[tokenType]int{
		tokenUnion:     UNION,
		tokenIntersect: INTERSECT,
		tokenStar:      POSTFIX,
		tokenPlus:      POSTFIX,
		tokenQuestion:  POSTFIX,
		tokenLBrace:    POSTFIX,
	}
	if prec, ok := precedences[p.peekToken.Type]; ok {
		return prec
	}
	return LOWEST
}

func lowerRuneLiteral(r rune) node {
	var buf [utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], r)
	if n == 1 {
		return &literalNode{Value: buf[0]}
	}
	out := node(&literalNode{Value: buf[0]})
	for i := 1; i < n; i++ {
		out = newConcatNode(out, &literalNode{Value: buf[i]})
	}
	return out
}

func (p *parser) literalRuneNode(r rune) node {
	if !p.caseInsensitive {
		return lowerRuneLiteral(r)
	}
	if !p.unicodeMode {
		if r >= 'a' && r <= 'z' {
			return unionNodes(lowerRuneLiteral(r), lowerRuneLiteral(r-'a'+'A'))
		}
		if r >= 'A' && r <= 'Z' {
			return unionNodes(lowerRuneLiteral(r), lowerRuneLiteral(r-'A'+'a'))
		}
		return lowerRuneLiteral(r)
	}
	folds := map[rune]struct{}{r: {}}
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		folds[f] = struct{}{}
	}
	if len(folds) == 1 {
		return lowerRuneLiteral(r)
	}
	runes := make([]rune, 0, len(folds))
	for rr := range folds {
		runes = append(runes, rr)
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	var nodes []node
	for _, rr := range runes {
		nodes = append(nodes, lowerRuneLiteral(rr))
	}
	return unionNodes(nodes...)
}

func (p *parser) applyInlineFlags(flags string) {
	if flags == "" {
		return
	}
	enable := true
	for _, ch := range flags {
		switch ch {
		case '-':
			enable = false
		case 'i', 'I':
			p.caseInsensitive = enable
		case 's', 'S':
			p.dotAll = enable
		case 'u', 'U':
			p.unicodeMode = enable
		default:
			// Ignore unsupported flags for now.
		}
	}
}

func unicodeEscapeNode(val rune) node {
	switch val {
	case 'd':
		return compileUnicodeProperty("Nd")
	case 'D':
		return newIntersectNode(newAnyRuneNode(false), newComplementNode(compileUnicodeProperty("Nd")))
	case 's':
		return compileUnicodeProperty("White_Space")
	case 'S':
		return newIntersectNode(newAnyRuneNode(false), newComplementNode(compileUnicodeProperty("White_Space")))
	case 'w':
		word := unionNodes(
			compileUnicodeProperty("L"),
			compileUnicodeProperty("M"),
			compileUnicodeProperty("N"),
			compileUnicodeProperty("Pc"),
			compileUnicodeProperty("Join_Control"),
		)
		return word
	case 'W':
		word := unionNodes(
			compileUnicodeProperty("L"),
			compileUnicodeProperty("M"),
			compileUnicodeProperty("N"),
			compileUnicodeProperty("Pc"),
			compileUnicodeProperty("Join_Control"),
		)
		return newIntersectNode(newAnyRuneNode(false), newComplementNode(word))
	default:
		return lowerRuneLiteral(val)
	}
}
