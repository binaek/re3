package re3

import (
	"strconv"
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
	tokens         []token
	pos            int
	curToken       token
	peekToken      token
	groupCount     int
	expr           string
	prefixParseFns map[tokenType]func() (node, error)
	infixParseFns  map[tokenType]func(node) (node, error)
}

func newParser(tokens []token, expr string) *parser {
	p := &parser{tokens: tokens, pos: -1, expr: expr}
	p.prefixParseFns = map[tokenType]func() (node, error){
		tokenLiteral:    p.parseLiteral,
		tokenEscape:     p.parseEscape,
		tokenCharClass:  p.parseCharClass,
		tokenComplement: p.parseComplement,
		tokenLParen:     p.parseGroup,
		tokenDot:        p.parseDot,
		tokenLookAhead:  p.parseLookAhead,
		tokenLookBehind: p.parseLookBehind,
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
	return p.parseExpression(LOWEST)
}

func (p *parser) parseExpression(precedence int) (node, error) {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
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
	return &literalNode{Value: p.curToken.Value}, nil
}

func (p *parser) parseEscape() (node, error) {
	val := p.curToken.Value

	if val == 'd' || val == 'w' || val == 's' {
		return &charClassNode{Class: "\\" + string(val)}, nil
	}
	return &literalNode{Value: val}, nil
}
func (p *parser) parseCharClass() (node, error) {
	return &charClassNode{Class: p.curToken.Text}, nil
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
func (p *parser) parseUnion(left node) (node, error) {
	p.nextToken()
	right, err := p.parseExpression(UNION)
	if err != nil {
		return nil, err
	}
	return newUnionNode(left, right), nil
}
func (p *parser) parseIntersect(left node) (node, error) {
	p.nextToken()
	right, err := p.parseExpression(INTERSECT)
	if err != nil {
		return nil, err
	}
	return newIntersectNode(left, right), nil
}
func (p *parser) parseStar(left node) (node, error)     { return &starNode{Child: left}, nil }
func (p *parser) parsePlus(left node) (node, error)     { return newConcatNode(left, &starNode{Child: left}), nil }
func (p *parser) parseQuestion(left node) (node, error) { return newUnionNode(left, &emptyNode{}), nil }

func (p *parser) parseBoundedRepeat(left node) (node, error) {
	p.nextToken()
	if p.curToken.Type != tokenNumber {
		return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
	}
	n, err := strconv.Atoi(p.curToken.Text)
	if err != nil || n < 0 {
		return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
	}
	hasComma := false
	var m int
	p.nextToken()
	if p.curToken.Type == tokenComma {
		hasComma = true
		p.nextToken()
		if p.curToken.Type == tokenRBrace {
			p.nextToken()
			return desugarRepeatMinLeft(left, n), nil
		}
		if p.curToken.Type != tokenNumber {
			return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
		}
		m, err = strconv.Atoi(p.curToken.Text)
		if err != nil || m < 0 || m < n {
			return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
		}
		p.nextToken()
	}
	if p.curToken.Type != tokenRBrace {
		return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
	}
	p.nextToken()
	if !hasComma {
		return desugarRepeatExact(left, n), nil
	}
	return desugarRepeatMinMax(left, n, m, p.expr)
}

func desugarRepeatExact(child node, n int) node {
	if n == 0 {
		return &emptyNode{}
	}
	out := child
	for i := 1; i < n; i++ {
		out = newConcatNode(out, child)
	}
	return out
}

func desugarRepeatMinLeft(child node, n int) node {
	if n == 0 {
		return &starNode{Child: child}
	}
	out := desugarRepeatExact(child, n)
	return newConcatNode(out, &starNode{Child: child})
}

func desugarRepeatMinMax(child node, n, m int, expr string) (node, error) {
	if n > m {
		return nil, &Error{Code: ErrInvalidRepeatSize, Expr: expr}
	}
	if n == 0 && m == 0 {
		return &emptyNode{}, nil
	}
	out := desugarRepeatExact(child, n)
	for i := 0; i < m-n; i++ {
		out = newConcatNode(out, newUnionNode(child, &emptyNode{}))
	}
	return out, nil
}

func (p *parser) isPeekStartOfExpression() bool {
	t := p.peekToken.Type
	return t == tokenLiteral || t == tokenLParen || t == tokenComplement ||
		t == tokenCharClass || t == tokenEscape || t == tokenDot ||
		t == tokenLookAhead || t == tokenLookBehind
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
	return &anyNode{}, nil
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
