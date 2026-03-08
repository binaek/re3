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

type Parser struct {
	tokens         []Token
	pos            int
	curToken       Token
	peekToken      Token
	groupCount     int
	expr           string // source expression for error reporting
	prefixParseFns map[TokenType]func() (Node, error)
	infixParseFns  map[TokenType]func(Node) (Node, error)
}

func NewParser(tokens []Token, expr string) *Parser {
	p := &Parser{tokens: tokens, pos: -1, expr: expr}
	p.prefixParseFns = map[TokenType]func() (Node, error){
		TokenLiteral:    p.parseLiteral,
		TokenEscape:     p.parseEscape,
		TokenCharClass:  p.parseCharClass,
		TokenComplement: p.parseComplement,
		TokenLParen:     p.parseGroup,
		TokenDot:        p.parseDot,
		TokenLookAhead:  p.parseLookAhead,
		TokenLookBehind: p.parseLookBehind,
	}
	p.infixParseFns = map[TokenType]func(Node) (Node, error){
		TokenUnion:     p.parseUnion,
		TokenIntersect: p.parseIntersect,
		TokenStar:      p.parseStar,
		TokenPlus:      p.parsePlus,
		TokenQuestion:  p.parseQuestion,
		TokenLBrace:    p.parseBoundedRepeat,
	}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.pos++
	if p.pos < len(p.tokens) {
		p.peekToken = p.tokens[p.pos]
	} else {
		p.peekToken = Token{Type: TokenEOF}
	}
}

func (p *Parser) Parse() (Node, error) {
	return p.parseExpression(LOWEST)
}

func (p *Parser) parseExpression(precedence int) (Node, error) {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		return nil, &Error{Code: ErrInternalError, Expr: p.expr}
	}
	leftExp, err := prefix()
	if err != nil {
		return nil, err
	}

	for p.peekToken.Type != TokenEOF && precedence < p.peekPrecedence() {
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
func (p *Parser) parseLiteral() (Node, error) {
	return &LiteralNode{Value: p.curToken.Value}, nil
}

func (p *Parser) parseEscape() (Node, error) {
	val := p.curToken.Value

	// If it is a known shorthand class, treat it exactly like a bracketed CharClassNode!
	if val == 'd' || val == 'w' || val == 's' {
		return &CharClassNode{Class: "\\" + string(val)}, nil
	}

	// Otherwise, it is a standard escaped literal (like \*, \+, \.)
	return &LiteralNode{Value: val}, nil
}
func (p *Parser) parseCharClass() (Node, error) {
	return &CharClassNode{Class: p.curToken.Text}, nil
}
func (p *Parser) parseComplement() (Node, error) {
	p.nextToken()
	child, err := p.parseExpression(PREFIX)
	if err != nil {
		return nil, err
	}
	return NewComplementNode(child), nil
}
func (p *Parser) parseGroup() (Node, error) {
	p.nextToken()
	p.groupCount++
	id := p.groupCount
	exp, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != TokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return &GroupNode{GroupID: id, Child: exp}, nil
}
func (p *Parser) parseUnion(left Node) (Node, error) {
	p.nextToken()
	right, err := p.parseExpression(UNION)
	if err != nil {
		return nil, err
	}
	return NewUnionNode(left, right), nil
}
func (p *Parser) parseIntersect(left Node) (Node, error) {
	p.nextToken()
	right, err := p.parseExpression(INTERSECT)
	if err != nil {
		return nil, err
	}
	return NewIntersectNode(left, right), nil
}
func (p *Parser) parseStar(left Node) (Node, error)     { return &StarNode{Child: left}, nil }
func (p *Parser) parsePlus(left Node) (Node, error)     { return NewConcatNode(left, &StarNode{Child: left}), nil }
func (p *Parser) parseQuestion(left Node) (Node, error) { return NewUnionNode(left, &EmptyNode{}), nil }

func (p *Parser) parseBoundedRepeat(left Node) (Node, error) {
	p.nextToken() // consume LBrace
	if p.curToken.Type != TokenNumber {
		return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
	}
	n, err := strconv.Atoi(p.curToken.Text)
	if err != nil || n < 0 {
		return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
	}
	hasComma := false
	var m int
	p.nextToken()
	if p.curToken.Type == TokenComma {
		hasComma = true
		p.nextToken()
		if p.curToken.Type == TokenRBrace {
			// {n,} = at least n
			p.nextToken() // consume RBrace
			return desugarRepeatMinLeft(left, n), nil
		}
		if p.curToken.Type != TokenNumber {
			return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
		}
		m, err = strconv.Atoi(p.curToken.Text)
		if err != nil || m < 0 || m < n {
			return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
		}
		p.nextToken()
	}
	if p.curToken.Type != TokenRBrace {
		return nil, &Error{Code: ErrInvalidRepeatSize, Expr: p.expr}
	}
	p.nextToken()
	if !hasComma {
		// {n} = exactly n
		return desugarRepeatExact(left, n), nil
	}
	// {n,m}
	return desugarRepeatMinMax(left, n, m, p.expr)
}

func desugarRepeatExact(child Node, n int) Node {
	if n == 0 {
		return &EmptyNode{}
	}
	out := child
	for i := 1; i < n; i++ {
		out = NewConcatNode(out, child)
	}
	return out
}

func desugarRepeatMinLeft(child Node, n int) Node {
	if n == 0 {
		return &StarNode{Child: child}
	}
	out := desugarRepeatExact(child, n)
	return NewConcatNode(out, &StarNode{Child: child})
}

func desugarRepeatMinMax(child Node, n, m int, expr string) (Node, error) {
	if n > m {
		return nil, &Error{Code: ErrInvalidRepeatSize, Expr: expr}
	}
	if n == 0 && m == 0 {
		return &EmptyNode{}, nil
	}
	// n to m: n required, then (m-n) optional. a{1,3} = a (a?)(a?)
	out := desugarRepeatExact(child, n)
	for i := 0; i < m-n; i++ {
		out = NewConcatNode(out, NewUnionNode(child, &EmptyNode{}))
	}
	return out, nil
}

func (p *Parser) isPeekStartOfExpression() bool {
	t := p.peekToken.Type
	return t == TokenLiteral || t == TokenLParen || t == TokenComplement ||
		t == TokenCharClass || t == TokenEscape || t == TokenDot ||
		t == TokenLookAhead || t == TokenLookBehind
}
func (p *Parser) parseImplicitConcat(left Node) (Node, error) {
	p.nextToken()
	right, err := p.parseExpression(CONCAT)
	if err != nil {
		return nil, err
	}
	return NewConcatNode(left, right), nil
}
func (p *Parser) parseDot() (Node, error) {
	return &AnyNode{}, nil
}

func (p *Parser) parseLookAhead() (Node, error) {
	p.nextToken() // consume (?=
	child, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != TokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return &LookAheadNode{Child: child}, nil
}

func (p *Parser) parseLookBehind() (Node, error) {
	p.nextToken() // consume (?<=
	child, err := p.parseExpression(LOWEST)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type != TokenRParen {
		return nil, &Error{Code: ErrMissingParen, Expr: p.expr}
	}
	p.nextToken()
	return &LookBehindNode{Child: child}, nil
}
func (p *Parser) peekPrecedence() int {
	if p.isPeekStartOfExpression() {
		return CONCAT
	}
	precedences := map[TokenType]int{
		TokenUnion:     UNION,
		TokenIntersect: INTERSECT,
		TokenStar:      POSTFIX,
		TokenPlus:      POSTFIX,
		TokenQuestion:  POSTFIX,
		TokenLBrace:    POSTFIX,
	}
	if prec, ok := precedences[p.peekToken.Type]; ok {
		return prec
	}
	return LOWEST
}
