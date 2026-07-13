package filter

import (
	"fmt"
	"strings"
	"unicode"
)

type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdent
	TokenString
	TokenNumber
	TokenBool
	TokenDate
	TokenOp
	TokenAnd
	TokenOr
	TokenNot
	TokenLParen
	TokenRParen
)

type Token struct {
	Type  TokenType
	Value string
	Pos   int
}

type Lexer struct {
	input  string
	pos    int
	tokens []Token
}

func NewLexer(input string) *Lexer {
	return &Lexer{input: input}
}

func (l *Lexer) Tokenize() []Token {
	for l.pos < len(l.input) {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			break
		}

		ch := l.input[l.pos]

		switch {
		case ch == '(':
			l.tokens = append(l.tokens, Token{TokenLParen, "(", l.pos})
			l.pos++
		case ch == ')':
			l.tokens = append(l.tokens, Token{TokenRParen, ")", l.pos})
			l.pos++
		case ch == '\'' || ch == '"':
			l.readString(ch)
		case ch == '=' || ch == '!' || ch == '>' || ch == '<' || ch == '~':
			l.readOp()
		case unicode.IsDigit(rune(ch)) || ch == '-':
			l.readNumberOrDate()
		case unicode.IsLetter(rune(ch)):
			l.readIdentOrKeyword()
		default:
			l.tokens = append(l.tokens, Token{TokenIdent, string(ch), l.pos})
			l.pos++
		}
	}
	l.tokens = append(l.tokens, Token{TokenEOF, "", l.pos})
	return l.tokens
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && unicode.IsSpace(rune(l.input[l.pos])) {
		l.pos++
	}
}

func (l *Lexer) readString(quote byte) {
	start := l.pos
	l.pos++
	for l.pos < len(l.input) && l.input[l.pos] != quote {
		l.pos++
	}
	if l.pos < len(l.input) {
		l.pos++
	}
	l.tokens = append(l.tokens, Token{TokenString, l.input[start+1 : l.pos-1], start})
}

func (l *Lexer) readOp() {
	start := l.pos
	ch := l.input[l.pos]

	switch ch {
	case '=':
		l.pos++
	case '!':
		l.pos++
		if l.pos < len(l.input) && (l.input[l.pos] == '=' || l.input[l.pos] == '~') {
			l.pos++
		}
	case '>', '<':
		l.pos++
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
		}
	case '~':
		l.pos++
	}

	l.tokens = append(l.tokens, Token{TokenOp, l.input[start:l.pos], start})
}

func (l *Lexer) readNumberOrDate() {
	start := l.pos
	if l.input[l.pos] == '-' {
		l.pos++
	}

	for l.pos < len(l.input) && (unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '-') {
		l.pos++
	}

	val := l.input[start:l.pos]
	if strings.Contains(val, "-") && len(val) == 10 {
		l.tokens = append(l.tokens, Token{TokenDate, val, start})
	} else if strings.Contains(val, ".") {
		l.tokens = append(l.tokens, Token{TokenNumber, val, start})
	} else {
		l.tokens = append(l.tokens, Token{TokenNumber, val, start})
	}
}

func (l *Lexer) readIdentOrKeyword() {
	start := l.pos
	for l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '_') {
		l.pos++
	}

	val := l.input[start:l.pos]
	lower := strings.ToLower(val)

	switch lower {
	case "and":
		l.tokens = append(l.tokens, Token{TokenAnd, val, start})
	case "or":
		l.tokens = append(l.tokens, Token{TokenOr, val, start})
	case "not":
		l.tokens = append(l.tokens, Token{TokenNot, val, start})
	case "true", "false":
		l.tokens = append(l.tokens, Token{TokenBool, lower, start})
	default:
		l.tokens = append(l.tokens, Token{TokenIdent, val, start})
	}
}

func (t *Token) String() string {
	return fmt.Sprintf("Token(%d, %q, %d)", t.Type, t.Value, t.Pos)
}
