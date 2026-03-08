package re3

import "fmt"

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
	prefixParseFns map[TokenType]func() Node
	infixParseFns  map[TokenType]func(Node) Node
}

func NewParser(tokens []Token) *Parser {
	p := &Parser{tokens: tokens, pos: -1}
	p.prefixParseFns = map[TokenType]func() Node{
		TokenLiteral:    p.parseLiteral,
		TokenEscape:     p.parseEscape,
		TokenCharClass:  p.parseCharClass,
		TokenComplement: p.parseComplement,
		TokenLParen:     p.parseGroup,
		TokenDot:        p.parseDot, // Add this!
	}
	p.infixParseFns = map[TokenType]func(Node) Node{
		TokenUnion:     p.parseUnion,
		TokenIntersect: p.parseIntersect,
		TokenStar:      p.parseStar,
		TokenPlus:      p.parsePlus,
		TokenQuestion:  p.parseQuestion,
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

func (p *Parser) Parse() Node {
	return p.parseExpression(LOWEST)
}

func (p *Parser) parseExpression(precedence int) Node {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		panic(fmt.Sprintf("no prefix parse function for %v", p.curToken.Type))
	}
	leftExp := prefix()

	for p.peekToken.Type != TokenEOF && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			if p.isPeekStartOfExpression() {
				leftExp = p.parseImplicitConcat(leftExp)
				continue
			}
			return leftExp
		}
		p.nextToken()
		leftExp = infix(leftExp)
	}
	return leftExp
}

// --- PARSING HANDLERS ---
func (p *Parser) parseLiteral() Node { return &LiteralNode{Value: p.curToken.Value} }

func (p *Parser) parseEscape() Node {
	val := p.curToken.Value

	// If it is a known shorthand class, treat it exactly like a bracketed CharClassNode!
	if val == 'd' || val == 'w' || val == 's' {
		return &CharClassNode{Class: "\\" + string(val)}
	}

	// Otherwise, it is a standard escaped literal (like \*, \+, \.)
	return &LiteralNode{Value: val}
}
func (p *Parser) parseCharClass() Node { return &CharClassNode{Class: p.curToken.Text} }
func (p *Parser) parseComplement() Node {
	p.nextToken()
	return NewComplementNode(p.parseExpression(PREFIX))
}
func (p *Parser) parseGroup() Node {
	p.nextToken()
	p.groupCount++
	id := p.groupCount
	exp := p.parseExpression(LOWEST)
	if p.peekToken.Type != TokenRParen {
		panic("expected closing parenthesis")
	}
	p.nextToken()
	return &GroupNode{GroupID: id, Child: exp}
}
func (p *Parser) parseUnion(left Node) Node {
	p.nextToken()
	return NewUnionNode(left, p.parseExpression(UNION))
}
func (p *Parser) parseIntersect(left Node) Node {
	p.nextToken()
	return NewIntersectNode(left, p.parseExpression(INTERSECT))
}
func (p *Parser) parseStar(left Node) Node     { return &StarNode{Child: left} }
func (p *Parser) parsePlus(left Node) Node     { return NewConcatNode(left, &StarNode{Child: left}) }
func (p *Parser) parseQuestion(left Node) Node { return NewUnionNode(left, &EmptyNode{}) }

func (p *Parser) isPeekStartOfExpression() bool {
	t := p.peekToken.Type
	return t == TokenLiteral || t == TokenLParen || t == TokenComplement ||
		t == TokenCharClass || t == TokenEscape || t == TokenDot
}
func (p *Parser) parseImplicitConcat(left Node) Node {
	p.nextToken()
	return NewConcatNode(left, p.parseExpression(CONCAT))
}
func (p *Parser) parseDot() Node {
	return &AnyNode{}
}
func (p *Parser) peekPrecedence() int {
	if p.isPeekStartOfExpression() {
		return CONCAT
	}
	precedences := map[TokenType]int{TokenUnion: UNION, TokenIntersect: INTERSECT, TokenStar: POSTFIX, TokenPlus: POSTFIX, TokenQuestion: POSTFIX}
	if prec, ok := precedences[p.peekToken.Type]; ok {
		return prec
	}
	return LOWEST
}
