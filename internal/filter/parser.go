package filter

import (
	"fmt"
	"strconv"

	"github.com/KIRKR101/hardcover-cli/internal/errs"
)

type Parser struct {
	tokens []Token
	pos    int
}

func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens}
}

func (p *Parser) Parse() (Expr, error) {
	return p.parseOr()
}

func (p *Parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	for p.current().Type == TokenOr {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &Logical{Op: "OR", Left: left, Right: right}
	}

	return left, nil
}

func (p *Parser) parseAnd() (Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}

	for p.current().Type == TokenAnd {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &Logical{Op: "AND", Left: left, Right: right}
	}

	return left, nil
}

func (p *Parser) parseNot() (Expr, error) {
	if p.current().Type == TokenNot {
		p.advance()
		expr, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &Not{Expr: expr}, nil
	}
	return p.parsePrimary()
}

func (p *Parser) parsePrimary() (Expr, error) {
	tok := p.current()

	if tok.Type == TokenLParen {
		p.advance()
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.current().Type != TokenRParen {
			return nil, fmt.Errorf("expected ')' at position %d: %w", p.current().Pos, errs.ErrInvalid)
		}
		p.advance()
		return expr, nil
	}

	if tok.Type == TokenIdent {
		return p.parseComparison()
	}

	return nil, fmt.Errorf("unexpected token %q at position %d: %w", tok.Value, tok.Pos, errs.ErrInvalid)
}

func (p *Parser) parseComparison() (Expr, error) {
	fieldTok := p.current()
	if fieldTok.Type != TokenIdent {
		return nil, fmt.Errorf("expected field name at position %d: %w", fieldTok.Pos, errs.ErrInvalid)
	}
	p.advance()

	opTok := p.current()
	if opTok.Type != TokenOp {
		return nil, fmt.Errorf("expected operator after %q at position %d: %w", fieldTok.Value, opTok.Pos, errs.ErrInvalid)
	}
	p.advance()

	valTok := p.current()
	var value any

	switch valTok.Type {
	case TokenString:
		value = valTok.Value
		p.advance()
	case TokenNumber:
		f, err := strconv.ParseFloat(valTok.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q at position %d: %w", valTok.Value, valTok.Pos, errs.ErrInvalid)
		}
		value = f
		p.advance()
	case TokenBool:
		value = valTok.Value == "true"
		p.advance()
	case TokenDate:
		value = valTok.Value
		p.advance()
	case TokenIdent:
		value = valTok.Value
		p.advance()
	default:
		return nil, fmt.Errorf("expected value after operator at position %d: %w", valTok.Pos, errs.ErrInvalid)
	}

	return &Comparison{Field: fieldTok.Value, Op: opTok.Value, Value: value}, nil
}

func (p *Parser) current() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() {
	if p.pos < len(p.tokens) {
		p.pos++
	}
}
