package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ─────────────────────────────────────────────
//  lexer.go — EclipseIR Lexer
// ─────────────────────────────────────────────

type TokenKind int

const (
	// Literals
	TOK_IDENT  TokenKind = iota // foo, main, L0
	TOK_INT                     // 42
	TOK_FLOAT                   // 3.14
	TOK_STRING                  // "hello"

	// Sigils
	TOK_PERCENT  // %
	TOK_AT       // @
	TOK_ARROW    // ->
	TOK_COLON    // :
	TOK_COMMA    // ,
	TOK_LPAREN   // (
	TOK_RPAREN   // )
	TOK_LBRACE   // {
	TOK_RBRACE   // }
	TOK_LBRACKET // [
	TOK_RBRACKET // ]
	TOK_HASH     // #

	// Keywords — instructions
	TOK_FUNC
	TOK_CONST
	TOK_ADD
	TOK_SUB
	TOK_MUL
	TOK_DIV
	TOK_MOD
	TOK_AND
	TOK_OR
	TOK_NOT
	TOK_NEG
	TOK_GT
	TOK_LT
	TOK_EQ
	TOK_GE
	TOK_LE
	TOK_NE
	TOK_GOTO
	TOK_IF_GOTO
	TOK_CALL
	TOK_RET
	TOK_PUTS
	TOK_LOAD
	TOK_STORE
	TOK_ALLOC
	TOK_SYSCALL
	TOK_SYSCALL_NR // syscall number keyword
	TOK_STRLEN

	// Misc
	TOK_ASSIGN // =
	TOK_EOF
	TOK_NEWLINE
	TOK_UNKNOWN
)

var keywords = map[string]TokenKind{
	"func":    TOK_FUNC,
	"const":   TOK_CONST,
	"add":     TOK_ADD,
	"sub":     TOK_SUB,
	"mul":     TOK_MUL,
	"div":     TOK_DIV,
	"mod":     TOK_MOD,
	"and":     TOK_AND,
	"or":      TOK_OR,
	"not":     TOK_NOT,
	"neg":     TOK_NEG,
	"gt":      TOK_GT,
	"lt":      TOK_LT,
	"eq":      TOK_EQ,
	"ge":      TOK_GE,
	"le":      TOK_LE,
	"ne":      TOK_NE,
	"goto":    TOK_GOTO,
	"if_goto": TOK_IF_GOTO,
	"call":    TOK_CALL,
	"ret":     TOK_RET,
	"puts":    TOK_PUTS,
	"load":    TOK_LOAD,
	"store":   TOK_STORE,
	"alloc":   TOK_ALLOC,
	"syscall": TOK_SYSCALL,
	"strlen":  TOK_STRLEN,
}

type Token struct {
	Kind    TokenKind
	Value   string
	IntVal  int64
	FltVal  float64
	Line    int
	Col     int
}

func (t Token) String() string {
	switch t.Kind {
	case TOK_IDENT:
		return fmt.Sprintf("IDENT(%s)", t.Value)
	case TOK_INT:
		return fmt.Sprintf("INT(%d)", t.IntVal)
	case TOK_FLOAT:
		return fmt.Sprintf("FLOAT(%g)", t.FltVal)
	case TOK_STRING:
		return fmt.Sprintf("STR(%q)", t.Value)
	default:
		return fmt.Sprintf("TOK(%s)", t.Value)
	}
}

// ─── Lexer ───────────────────────────────────

type Lexer struct {
	src    []rune
	pos    int
	line   int
	col    int
	tokens []Token
	errors []LexError
}

type LexError struct {
	Line int
	Col  int
	Msg  string
}

func (e LexError) Error() string {
	return fmt.Sprintf("lex error [%d:%d]: %s", e.Line, e.Col, e.Msg)
}

func NewLexer(src string) *Lexer {
	return &Lexer{src: []rune(src), line: 1, col: 1}
}

func (l *Lexer) Tokenize() ([]Token, []LexError) {
	for {
		tok := l.nextToken()
		l.tokens = append(l.tokens, tok)
		if tok.Kind == TOK_EOF {
			break
		}
	}
	return l.tokens, l.errors
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekAt(offset int) rune {
	p := l.pos + offset
	if p >= len(l.src) {
		return 0
	}
	return l.src[p]
}

func (l *Lexer) advance() rune {
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) skipLineComment() {
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.pos++
	}
}

func (l *Lexer) makeToken(kind TokenKind, val string) Token {
	return Token{Kind: kind, Value: val, Line: l.line, Col: l.col}
}

func (l *Lexer) nextToken() Token {
	// Skip whitespace (but not newlines — they're significant as line separators)
	for l.pos < len(l.src) {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
		} else if ch == ';' || (ch == '/' && l.peekAt(1) == '/') {
			l.skipLineComment()
		} else {
			break
		}
	}

	if l.pos >= len(l.src) {
		return l.makeToken(TOK_EOF, "")
	}

	startLine := l.line
	startCol := l.col
	ch := l.peek()

	tok := func(kind TokenKind, val string) Token {
		return Token{Kind: kind, Value: val, Line: startLine, Col: startCol}
	}

	// Newlines — used as statement separators
	if ch == '\n' {
		l.advance()
		return tok(TOK_NEWLINE, "\\n")
	}

	// Single-char punctuation
	switch ch {
	case '%':
		l.advance()
		return tok(TOK_PERCENT, "%")
	case '@':
		l.advance()
		return tok(TOK_AT, "@")
	case ':':
		l.advance()
		return tok(TOK_COLON, ":")
	case ',':
		l.advance()
		return tok(TOK_COMMA, ",")
	case '(':
		l.advance()
		return tok(TOK_LPAREN, "(")
	case ')':
		l.advance()
		return tok(TOK_RPAREN, ")")
	case '{':
		l.advance()
		return tok(TOK_LBRACE, "{")
	case '}':
		l.advance()
		return tok(TOK_RBRACE, "}")
	case '[':
		l.advance()
		return tok(TOK_LBRACKET, "[")
	case ']':
		l.advance()
		return tok(TOK_RBRACKET, "]")
	case '#':
		l.advance()
		return tok(TOK_HASH, "#")
	case '=':
		l.advance()
		return tok(TOK_ASSIGN, "=")
	case '-':
		if l.peekAt(1) == '>' {
			l.advance()
			l.advance()
			return tok(TOK_ARROW, "->")
		}
		// Negative number
		l.advance()
		if l.pos < len(l.src) && unicode.IsDigit(l.peek()) {
			return l.lexNumber(startLine, startCol, true)
		}
		return tok(TOK_UNKNOWN, "-")
	}

	// Numbers
	if unicode.IsDigit(ch) {
		return l.lexNumber(startLine, startCol, false)
	}

	// String literals
	if ch == '"' {
		return l.lexString(startLine, startCol)
	}

	// c"..." string (C-style string, from prototype)
	if ch == 'c' && l.peekAt(1) == '"' {
		l.advance() // skip 'c'
		return l.lexString(startLine, startCol)
	}

	// Identifiers and keywords
	if unicode.IsLetter(ch) || ch == '_' {
		return l.lexIdent(startLine, startCol)
	}

	// Unknown
	l.errors = append(l.errors, LexError{Line: startLine, Col: startCol,
		Msg: fmt.Sprintf("unexpected character %q", ch)})
	l.advance()
	return tok(TOK_UNKNOWN, string(ch))
}

func (l *Lexer) lexNumber(line, col int, negative bool) Token {
	var sb strings.Builder
	if negative {
		sb.WriteRune('-')
	}
	isFloat := false
	for l.pos < len(l.src) {
		ch := l.peek()
		if unicode.IsDigit(ch) {
			sb.WriteRune(ch)
			l.advance()
		} else if ch == '.' && !isFloat {
			isFloat = true
			sb.WriteRune(ch)
			l.advance()
		} else if ch == 'e' || ch == 'E' {
			isFloat = true
			sb.WriteRune(ch)
			l.advance()
			if l.pos < len(l.src) && (l.peek() == '+' || l.peek() == '-') {
				sb.WriteRune(l.advance())
			}
		} else {
			break
		}
	}
	s := sb.String()
	if isFloat {
		fv, _ := strconv.ParseFloat(s, 64)
		return Token{Kind: TOK_FLOAT, Value: s, FltVal: fv, Line: line, Col: col}
	}
	iv, _ := strconv.ParseInt(s, 10, 64)
	return Token{Kind: TOK_INT, Value: s, IntVal: iv, Line: line, Col: col}
}

func (l *Lexer) lexString(line, col int) Token {
	l.advance() // opening "
	var sb strings.Builder
	for l.pos < len(l.src) {
		ch := l.peek()
		if ch == '"' {
			l.advance()
			break
		}
		if ch == '\\' {
			l.advance()
			esc := l.advance()
			switch esc {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case '\\':
				sb.WriteRune('\\')
			case '"':
				sb.WriteRune('"')
			case '0':
				sb.WriteRune(0)
			default:
				sb.WriteRune('\\')
				sb.WriteRune(esc)
			}
		} else {
			sb.WriteRune(l.advance())
		}
	}
	return Token{Kind: TOK_STRING, Value: sb.String(), Line: line, Col: col}
}

func (l *Lexer) lexIdent(line, col int) Token {
	var sb strings.Builder
	for l.pos < len(l.src) {
		ch := l.peek()
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' {
			sb.WriteRune(ch)
			l.advance()
		} else {
			break
		}
	}
	s := sb.String()
	// Check for compound keyword if_goto
	if s == "if" && l.pos < len(l.src) && l.peek() == '_' {
		// peek ahead
		saved := l.pos
		savedLine := l.line
		savedCol := l.col
		l.advance() // _
		var sb2 strings.Builder
		for l.pos < len(l.src) {
			ch := l.peek()
			if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' {
				sb2.WriteRune(ch)
				l.advance()
			} else {
				break
			}
		}
		compound := s + "_" + sb2.String()
		if kind, ok := keywords[compound]; ok {
			return Token{Kind: kind, Value: compound, Line: line, Col: col}
		}
		// Not a keyword, restore
		l.pos = saved
		l.line = savedLine
		l.col = savedCol
	}
	if kind, ok := keywords[s]; ok {
		return Token{Kind: kind, Value: s, Line: line, Col: col}
	}
	return Token{Kind: TOK_IDENT, Value: s, Line: line, Col: col}
}
