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
		tokenLiteral:     p.parseLiteral,
		tokenEscape:      p.parseEscape,
		tokenCharClass:   p.parseCharClass,
		tokenLParen:      p.parseGroup,
		tokenDot:         p.parseDot,
		tokenLookAhead:   p.parseLookAhead,
		tokenLookBehind:  p.parseLookBehind,
		tokenNonCapParen: p.parseNonCapGroup,
		tokenInlineFlags: func() (node, error) { return &emptyNode{}, nil },
		tokenEmpty:       func() (node, error) { return &emptyNode{}, nil },
		tokenComma:       func() (node, error) { return &literalNode{Value: ','}, nil },
		tokenLBrace:      func() (node, error) { return &literalNode{Value: '{'}, nil },
		tokenRBrace:      func() (node, error) { return &literalNode{Value: '}'}, nil },
		tokenUnion:       p.parseEmptyLeftUnion,
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
	return &literalNode{Value: p.curToken.Value}, nil
}

func (p *parser) parseEscape() (node, error) {
	val := p.curToken.Value
	switch val {
	case 'd', 'w', 's', 'D', 'W', 'S':
		return &charClassNode{Class: "\\" + string(val)}, nil
	case 'n':
		return &literalNode{Value: '\n'}, nil
	case 'r':
		return &literalNode{Value: '\r'}, nil
	case 't':
		return &literalNode{Value: '\t'}, nil
	case 'v':
		return &literalNode{Value: '\v'}, nil
	case 'f':
		return &literalNode{Value: '\f'}, nil
	case 'a':
		return &literalNode{Value: '\a'}, nil
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
	if p.peekToken.Type == tokenRParen || p.peekToken.Type == tokenUnion || p.peekToken.Type == tokenEOF {
		return newUnionNode(left, &emptyNode{}), nil
	}
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
