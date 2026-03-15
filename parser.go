package re3

import (
	"context"
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
	prefixParseFns  map[tokenType]func(context.Context) (node, error)
	infixParseFns   map[tokenType]func(context.Context, node) (node, error)
}

func newParser(tokens []token, expr string) *parser {
	p := &parser{tokens: tokens, pos: -1, expr: expr}
	p.prefixParseFns = map[tokenType]func(context.Context) (node, error){
		tokenLiteral:     p.parseLiteral,
		tokenEscape:      p.parseEscape,
		tokenCharClass:   p.parseCharClass,
		tokenLParen:      p.parseGroup,
		tokenDot:         p.parseDot,
		tokenLookAhead:   p.parseLookAhead,
		tokenLookBehind:  p.parseLookBehind,
		tokenNonCapParen: p.parseNonCapGroup,
		tokenInlineFlags: p.parseInlineFlags,
		tokenComplement:  p.parseComplement,
		tokenEmpty:       func(ctx context.Context) (node, error) { return newEmptyNode(ctx), nil },
		tokenStart:       func(ctx context.Context) (node, error) { return newStartNode(ctx), nil },
		tokenEnd:         func(ctx context.Context) (node, error) { return newEndNode(ctx), nil },
		tokenWordBoundary: func(ctx context.Context) (node, error) {
			return newWordBoundaryNode(ctx, p.unicodeMode), nil
		},
		tokenNotWordBoundary: func(ctx context.Context) (node, error) {
			return newNotWordBoundaryNode(ctx, p.unicodeMode), nil
		},
		tokenBeginText:              func(ctx context.Context) (node, error) { return newBeginTextNode(ctx), nil },
		tokenEndText:                func(ctx context.Context) (node, error) { return newEndTextNode(ctx), nil },
		tokenEndTextOptionalNewline: func(ctx context.Context) (node, error) { return newEndTextOptionalNewlineNode(ctx), nil },
		tokenComma:                  func(ctx context.Context) (node, error) { return lowerRuneLiteral(ctx, ','), nil },
		tokenLBrace:                 func(ctx context.Context) (node, error) { return lowerRuneLiteral(ctx, '{'), nil },
		tokenRBrace:                 func(ctx context.Context) (node, error) { return lowerRuneLiteral(ctx, '}'), nil },
		tokenUnion:                  p.parseEmptyLeftUnion,
	}
	p.infixParseFns = map[tokenType]func(context.Context, node) (node, error){
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

func (p *parser) parse(ctx context.Context) (node, error) {
	if p.curToken.Type == tokenEOF {
		return newEmptyNode(ctx), nil
	}
	return p.parseExpression(ctx, LOWEST)
}

func (p *parser) parseExpression(ctx context.Context, precedence int) (node, error) {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		if p.curToken.Type == tokenStar || p.curToken.Type == tokenPlus || p.curToken.Type == tokenQuestion {
			return nil, &Error{Code: ErrMissingRepeatArgument, Expr: p.expr}
		}
		return nil, &Error{Code: ErrInternalError, Expr: p.expr}
	}
	leftExp, err := prefix(ctx)
	if err != nil {
		return nil, err
	}

	for p.peekToken.Type != tokenEOF && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			if p.isPeekStartOfExpression() {
				leftExp, err = p.parseImplicitConcat(ctx, leftExp)
				if err != nil {
					return nil, err
				}
				continue
			}
			return leftExp, nil
		}
		p.nextToken()
		leftExp, err = infix(ctx, leftExp)
		if err != nil {
			return nil, err
		}
	}
	return leftExp, nil
}

// --- PARSING HANDLERS ---
func (p *parser) parseLiteral(ctx context.Context) (node, error) {
	return p.literalRuneNode(ctx, p.curToken.Value), nil
}

func (p *parser) parseEscape(ctx context.Context) (node, error) {
	val := p.curToken.Value
	switch val {
	case 'd', 'w', 's', 'D', 'W', 'S':
		if p.unicodeMode {
			return unicodeEscapeNode(ctx, val), nil
		}
		return newCharClassNode(ctx, "\\"+string(val), predicate{}), nil
	case 'n':
		return p.literalRuneNode(ctx, '\n'), nil
	case 'r':
		return p.literalRuneNode(ctx, '\r'), nil
	case 't':
		return p.literalRuneNode(ctx, '\t'), nil
	case 'v':
		return p.literalRuneNode(ctx, '\v'), nil
	case 'f':
		return p.literalRuneNode(ctx, '\f'), nil
	case 'a':
		return p.literalRuneNode(ctx, '\a'), nil
	}
	return p.literalRuneNode(ctx, val), nil
}
func (p *parser) parseCharClass(ctx context.Context) (node, error) {
	return compileCharClassNode(ctx, p.curToken.Text, p.caseInsensitive, p.unicodeMode), nil
}
func (p *parser) parseComplement(ctx context.Context) (node, error) {
	p.nextToken()
	child, err := p.parseExpression(ctx, PREFIX)
	if err != nil {
		return nil, err
	}
	return newComplementNode(ctx, child), nil
}
func (p *parser) parseGroup(ctx context.Context) (node, error) {
	p.nextToken()
	p.groupCount++
	id := p.groupCount

	if p.curToken.Type == tokenRParen {
		return newGroupNode(ctx, id, newEmptyNode(ctx)), nil
	}

	exp, err := p.parseExpression(ctx, LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != tokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return newGroupNode(ctx, id, exp), nil
}

func (p *parser) parseNonCapGroup(ctx context.Context) (node, error) {
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
		return newEmptyNode(ctx), nil
	}

	exp, err := p.parseExpression(ctx, LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != tokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return exp, nil
}
func (p *parser) parseUnion(ctx context.Context, left node) (node, error) {
	acc := left
	for {
		if p.peekToken.Type == tokenRParen || p.peekToken.Type == tokenUnion || p.peekToken.Type == tokenEOF {
			acc = newUnionNode(ctx, acc, newEmptyNode(ctx))
			if p.peekToken.Type == tokenUnion {
				// Consume repeated empty alternations like a||b iteratively.
				p.nextToken()
				continue
			}
			return acc, nil
		}
		p.nextToken()
		right, err := p.parseExpression(ctx, UNION)
		if err != nil {
			return nil, err
		}
		acc = newUnionNode(ctx, acc, right)
		if p.peekToken.Type != tokenUnion {
			return acc, nil
		}
		// Consume the next '|' and continue parsing in an iterative loop.
		p.nextToken()
	}
}
func (p *parser) parseIntersect(ctx context.Context, left node) (node, error) {
	p.nextToken()
	right, err := p.parseExpression(ctx, INTERSECT)
	if err != nil {
		return nil, err
	}
	return newIntersectNode(ctx, left, right), nil
}
func (p *parser) parseStar(ctx context.Context, left node) (node, error) {
	return newStarNode(ctx, left), nil
}
func (p *parser) parsePlus(ctx context.Context, left node) (node, error) {
	return newConcatNode(ctx, left, newStarNode(ctx, left)), nil
}
func (p *parser) parseQuestion(ctx context.Context, left node) (node, error) {
	return newUnionNode(ctx, left, newEmptyNode(ctx)), nil
}

func (p *parser) parseBoundedRepeat(ctx context.Context, left node) (node, error) {
	p.nextToken() // curToken is now the number
	n, _ := strconv.Atoi(p.curToken.Text)
	p.nextToken() // curToken is now ',' or '}'

	if p.curToken.Type == tokenComma {
		p.nextToken() // curToken is now number or '}'
		if p.curToken.Type == tokenRBrace {
			// e.g. {n,} -> Repeat exact `n` times, followed by a Star
			if n == 0 {
				return newStarNode(ctx, left), nil
			}
			return newConcatNode(ctx, newRepeatNode(ctx, left, n, n), newStarNode(ctx, left)), nil
		}

		m, _ := strconv.Atoi(p.curToken.Text)
		p.nextToken() // curToken is now '}'

		if n > m {
			return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
		}
		return newRepeatNode(ctx, left, n, m), nil
	}

	// Exact repeat {n}
	return newRepeatNode(ctx, left, n, n), nil
}

func (p *parser) parseEmptyLeftUnion(ctx context.Context) (node, error) {
	if p.peekToken.Type == tokenRParen || p.peekToken.Type == tokenUnion || p.peekToken.Type == tokenEOF {
		return newUnionNode(ctx, newEmptyNode(ctx), newEmptyNode(ctx)), nil
	}
	p.nextToken()
	right, err := p.parseExpression(ctx, UNION)
	if err != nil {
		return nil, err
	}
	return newUnionNode(ctx, newEmptyNode(ctx), right), nil
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
func (p *parser) parseImplicitConcat(ctx context.Context, left node) (node, error) {
	p.nextToken()
	right, err := p.parseExpression(ctx, CONCAT)
	if err != nil {
		return nil, err
	}
	return newConcatNode(ctx, left, right), nil
}
func (p *parser) parseDot(ctx context.Context) (node, error) {
	if p.unicodeMode {
		if p.dotAll {
			return newAnyRuneNode(ctx, false), nil
		}
		return newAnyNode(ctx), nil
	}
	if p.dotAll {
		return newAnyByteNode(ctx), nil
	}
	return newAnyNodeSimple(ctx), nil
}

func (p *parser) parseInlineFlags(ctx context.Context) (node, error) {
	p.applyInlineFlags(p.curToken.Text)
	return newEmptyNode(ctx), nil
}

func (p *parser) parseLookAhead(ctx context.Context) (node, error) {
	p.nextToken()
	child, err := p.parseExpression(ctx, LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != tokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return newLookAheadNode(ctx, child), nil
}

func (p *parser) parseLookBehind(ctx context.Context) (node, error) {
	p.nextToken()
	child, err := p.parseExpression(ctx, LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != tokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return newLookBehindNode(ctx, child), nil
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

func lowerRuneLiteral(ctx context.Context, r rune) node {
	var buf [utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], r)
	if n == 1 {
		return newLiteralNode(ctx, buf[0])
	}
	out := node(newLiteralNode(ctx, buf[0]))
	for i := 1; i < n; i++ {
		out = newConcatNode(ctx, out, newLiteralNode(ctx, buf[i]))
	}
	return out
}

func (p *parser) literalRuneNode(ctx context.Context, r rune) node {
	if !p.caseInsensitive {
		return lowerRuneLiteral(ctx, r)
	}
	if !p.unicodeMode {
		if r >= 'a' && r <= 'z' {
			return unionNodes(ctx, lowerRuneLiteral(ctx, r), lowerRuneLiteral(ctx, r-'a'+'A'))
		}
		if r >= 'A' && r <= 'Z' {
			return unionNodes(ctx, lowerRuneLiteral(ctx, r), lowerRuneLiteral(ctx, r-'A'+'a'))
		}
		return lowerRuneLiteral(ctx, r)
	}
	folds := map[rune]struct{}{r: {}}
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		folds[f] = struct{}{}
	}
	if len(folds) == 1 {
		return lowerRuneLiteral(ctx, r)
	}
	runes := make([]rune, 0, len(folds))
	for rr := range folds {
		runes = append(runes, rr)
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	var nodes []node
	for _, rr := range runes {
		nodes = append(nodes, lowerRuneLiteral(ctx, rr))
	}
	return unionNodes(ctx, nodes...)
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

func unicodeEscapeNode(ctx context.Context, val rune) node {
	switch val {
	case 'd':
		return compileUnicodeProperty(ctx, "Nd")
	case 'D':
		return newIntersectNode(ctx, newAnyRuneNode(ctx, false), newComplementNode(ctx, compileUnicodeProperty(ctx, "Nd")))
	case 's':
		return compileUnicodeProperty(ctx, "White_Space")
	case 'S':
		return newIntersectNode(ctx, newAnyRuneNode(ctx, false), newComplementNode(ctx, compileUnicodeProperty(ctx, "White_Space")))
	case 'w':
		word := unionNodes(ctx,
			compileUnicodeProperty(ctx, "L"),
			compileUnicodeProperty(ctx, "M"),
			compileUnicodeProperty(ctx, "N"),
			compileUnicodeProperty(ctx, "Pc"),
			compileUnicodeProperty(ctx, "Join_Control"),
		)
		return word
	case 'W':
		word := unionNodes(ctx,
			compileUnicodeProperty(ctx, "L"),
			compileUnicodeProperty(ctx, "M"),
			compileUnicodeProperty(ctx, "N"),
			compileUnicodeProperty(ctx, "Pc"),
			compileUnicodeProperty(ctx, "Join_Control"),
		)
		return newIntersectNode(ctx, newAnyRuneNode(ctx, false), newComplementNode(ctx, word))
	default:
		return lowerRuneLiteral(ctx, val)
	}
}
