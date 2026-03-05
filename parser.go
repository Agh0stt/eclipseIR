package main

import (
	"fmt"
	"os"
	"strings"
)

// ─────────────────────────────────────────────
//  parser.go — EclipseIR Parser
//  Converts token stream → Program AST
// ─────────────────────────────────────────────

type ParseError struct {
	Line int
	Col  int
	Msg  string
}

func (e ParseError) Error() string {
	return fmt.Sprintf("parse error [%d:%d]: %s", e.Line, e.Col, e.Msg)
}

type Parser struct {
	tokens []Token
	pos    int
	errors []ParseError
	vregs  int // next virtual register id
}

func newParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens}
}

// ── Token navigation ───────────────────────────

func (p *Parser) peek() Token {
	for p.pos < len(p.tokens) && p.tokens[p.pos].Kind == TOK_NEWLINE {
		// Don't skip newlines in peek — callers handle them
		return p.tokens[p.pos]
	}
	if p.pos >= len(p.tokens) {
		return Token{Kind: TOK_EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peekSkipNL() Token {
	i := p.pos
	for i < len(p.tokens) && p.tokens[i].Kind == TOK_NEWLINE {
		i++
	}
	if i >= len(p.tokens) {
		return Token{Kind: TOK_EOF}
	}
	return p.tokens[i]
}

func (p *Parser) advance() Token {
	tok := p.tokens[p.pos]
	p.pos++
	return tok
}

func (p *Parser) skipNewlines() {
	for p.pos < len(p.tokens) && p.tokens[p.pos].Kind == TOK_NEWLINE {
		p.pos++
	}
}

func (p *Parser) expect(kind TokenKind) (Token, bool) {
	p.skipNewlines()
	tok := p.peek()
	if tok.Kind != kind {
		p.errorf(tok, "expected %v, got %v (%q)", kind, tok.Kind, tok.Value)
		return tok, false
	}
	return p.advance(), true
}

func (p *Parser) check(kind TokenKind) bool {
	return p.peek().Kind == kind
}

func (p *Parser) match(kind TokenKind) bool {
	if p.check(kind) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) errorf(tok Token, format string, args ...interface{}) {
	p.errors = append(p.errors, ParseError{
		Line: tok.Line,
		Col:  tok.Col,
		Msg:  fmt.Sprintf(format, args...),
	})
}

func (p *Parser) nextVreg() int {
	v := p.vregs
	p.vregs++
	return v
}

// ── Top-level parse ────────────────────────────

// ParseIR reads an .ir file and returns a Program.
func ParseIR(filename string) (*Program, error) {
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	lex := NewLexer(string(src))
	tokens, lexErrs := lex.Tokenize()

	if len(lexErrs) > 0 {
		msgs := make([]string, len(lexErrs))
		for i, e := range lexErrs {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("lex errors:\n  %s", strings.Join(msgs, "\n  "))
	}

	parser := newParser(tokens)
	prog := parser.parseProgram()

	if len(parser.errors) > 0 {
		msgs := make([]string, len(parser.errors))
		for i, e := range parser.errors {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("parse errors:\n  %s", strings.Join(msgs, "\n  "))
	}

	return prog, nil
}

func (p *Parser) parseProgram() *Program {
	prog := &Program{}
	p.skipNewlines()

	for p.peek().Kind != TOK_EOF {
		tok := p.peek()

		switch tok.Kind {
		case TOK_AT:
			// global declaration: @name = ...
			g, ok := p.parseGlobal()
			if ok {
				prog.Globals = append(prog.Globals, g)
			}
		case TOK_FUNC:
			f, ok := p.parseFunction()
			if ok {
				prog.Funcs = append(prog.Funcs, f)
			}
		case TOK_NEWLINE:
			p.advance()
		default:
			p.errorf(tok, "unexpected token at top level: %q", tok.Value)
			p.advance()
		}
	}

	return prog
}

// ── Global ─────────────────────────────────────
// @name = i32 42
// @name = c"hello"
// @name = f64 3.14

func (p *Parser) parseGlobal() (Global, bool) {
	p.advance() // @
	nameTok, ok := p.expect(TOK_IDENT)
	if !ok {
		return Global{}, false
	}
	if _, ok := p.expect(TOK_ASSIGN); !ok {
		return Global{}, false
	}

	g := Global{Name: nameTok.Value}

	tok := p.peek()
	// String literal
	if tok.Kind == TOK_STRING {
		p.advance()
		g.Type = TY_STR
		g.Value = tok.Value
		p.skipNewlines()
		return g, true
	}

	// Typed value: <type> <literal>
	typeTok := p.advance()
	ty, ok2 := parseType(typeTok.Value)
	if !ok2 {
		p.errorf(typeTok, "unknown type %q in global", typeTok.Value)
		return Global{}, false
	}
	g.Type = ty

	valTok := p.peek()
	if ty.IsFloat() {
		if valTok.Kind == TOK_FLOAT {
			p.advance()
			g.ImmF = valTok.FltVal
		} else if valTok.Kind == TOK_INT {
			p.advance()
			g.ImmF = float64(valTok.IntVal)
		} else {
			p.errorf(valTok, "expected float literal for global %q", g.Name)
			return Global{}, false
		}
	} else {
		if valTok.Kind == TOK_INT {
			p.advance()
			g.ImmI = valTok.IntVal
		} else if valTok.Kind == TOK_STRING {
			p.advance()
			g.Value = valTok.Value
			g.Type = TY_STR
		} else {
			p.errorf(valTok, "expected literal for global %q", g.Name)
			return Global{}, false
		}
	}
	p.skipNewlines()
	return g, true
}

// ── Function ───────────────────────────────────
// func @name(i32 %0, i32 %1) -> i32 {
//   ...
// }

func (p *Parser) parseFunction() (Function, bool) {
	p.advance() // func
	if _, ok := p.expect(TOK_AT); !ok {
		return Function{}, false
	}
	nameTok, ok := p.expect(TOK_IDENT)
	if !ok {
		return Function{}, false
	}
	fn := Function{Name: nameTok.Value}

	// Params
	if _, ok := p.expect(TOK_LPAREN); !ok {
		return Function{}, false
	}
	for !p.check(TOK_RPAREN) && !p.check(TOK_EOF) {
		param, ok := p.parseParam()
		if !ok {
			return Function{}, false
		}
		fn.Params = append(fn.Params, param)
		if !p.match(TOK_COMMA) {
			break
		}
	}
	if _, ok := p.expect(TOK_RPAREN); !ok {
		return Function{}, false
	}

	// Return type
	if p.peek().Kind == TOK_ARROW {
		p.advance()
		retTok := p.advance()
		ty, ok2 := parseType(retTok.Value)
		if !ok2 {
			p.errorf(retTok, "unknown return type %q", retTok.Value)
			return Function{}, false
		}
		fn.RetType = ty
	}

	// Body
	p.skipNewlines()
	if _, ok := p.expect(TOK_LBRACE); !ok {
		return Function{}, false
	}
	p.skipNewlines()

	// Track max vreg seen in params
	for _, param := range fn.Params {
		if param.Vreg >= p.vregs {
			p.vregs = param.Vreg + 1
		}
	}

	for !p.check(TOK_RBRACE) && !p.check(TOK_EOF) {
		p.skipNewlines()
		if p.check(TOK_RBRACE) {
			break
		}
		in, ok := p.parseInstr()
		if ok {
			fn.Instrs = append(fn.Instrs, in)
		}
		p.skipNewlines()
	}

	if _, ok := p.expect(TOK_RBRACE); !ok {
		return Function{}, false
	}
	p.skipNewlines()
	return fn, true
}

func (p *Parser) parseParam() (Param, bool) {
	typeTok := p.advance()
	ty, ok := parseType(typeTok.Value)
	if !ok {
		p.errorf(typeTok, "unknown param type %q", typeTok.Value)
		return Param{}, false
	}
	if _, ok := p.expect(TOK_PERCENT); !ok {
		return Param{}, false
	}
	numTok, ok := p.expect(TOK_INT)
	if !ok {
		return Param{}, false
	}
	return Param{Type: ty, Vreg: int(numTok.IntVal)}, true
}

// ── Instructions ───────────────────────────────

func (p *Parser) parseInstr() (Instr, bool) {
	tok := p.peek()

	// Label: Lx:
	if tok.Kind == TOK_IDENT {
		if p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == TOK_COLON {
			name := tok.Value
			p.advance() // ident
			p.advance() // :
			p.skipNewlines()
			return Instr{Kind: INS_LABEL, Dst: -1, Label: name, Line: tok.Line}, true
		}
	}

	in := Instr{Dst: -1, Line: tok.Line}

	// Optional: %dst =
	if tok.Kind == TOK_PERCENT {
		p.advance() // %
		numTok, ok := p.expect(TOK_INT)
		if !ok {
			return Instr{}, false
		}
		in.Dst = int(numTok.IntVal)
		if _, ok := p.expect(TOK_ASSIGN); !ok {
			return Instr{}, false
		}
	}

	opTok := p.advance()
	switch opTok.Kind {
	case TOK_CONST:
		return p.parseConst(in)
	case TOK_ADD:
		return p.parseBinop(in, INS_ADD)
	case TOK_SUB:
		return p.parseBinop(in, INS_SUB)
	case TOK_MUL:
		return p.parseBinop(in, INS_MUL)
	case TOK_DIV:
		return p.parseBinop(in, INS_DIV)
	case TOK_MOD:
		return p.parseBinop(in, INS_MOD)
	case TOK_AND:
		return p.parseBinop(in, INS_AND)
	case TOK_OR:
		return p.parseBinop(in, INS_OR)
	case TOK_GT:
		return p.parseBinop(in, INS_CMP_GT)
	case TOK_LT:
		return p.parseBinop(in, INS_CMP_LT)
	case TOK_EQ:
		return p.parseBinop(in, INS_CMP_EQ)
	case TOK_GE:
		return p.parseBinop(in, INS_CMP_GE)
	case TOK_LE:
		return p.parseBinop(in, INS_CMP_LE)
	case TOK_NE:
		return p.parseBinop(in, INS_CMP_NE)
	case TOK_NOT:
		return p.parseUnop(in, INS_NOT)
	case TOK_NEG:
		return p.parseUnop(in, INS_NEG)
	case TOK_GOTO:
		return p.parseGoto(in)
	case TOK_IF_GOTO:
		return p.parseIfGoto(in)
	case TOK_RET:
		return p.parseRet(in)
	case TOK_PUTS:
		return p.parsePuts(in)
	case TOK_STRLEN:
		return p.parseStrlen(in)
	case TOK_CALL:
		return p.parseCall(in)
	case TOK_SYSCALL:
		return p.parseSyscall(in)
	case TOK_ALLOC:
		return p.parseAlloc(in)
	case TOK_LOAD:
		return p.parseLoad(in)
	case TOK_STORE:
		return p.parseStore(in)
	default:
		p.errorf(opTok, "unknown instruction %q", opTok.Value)
		// Skip to next newline for error recovery
		for p.pos < len(p.tokens) && p.tokens[p.pos].Kind != TOK_NEWLINE {
			p.pos++
		}
		return Instr{}, false
	}
}

// const <type> <value>
func (p *Parser) parseConst(in Instr) (Instr, bool) {
	in.Kind = INS_CONST
	typeTok := p.advance()
	ty, ok := parseType(typeTok.Value)
	if !ok {
		p.errorf(typeTok, "unknown type in const: %q", typeTok.Value)
		return Instr{}, false
	}
	in.Type = ty
	valTok := p.advance()
	if ty.IsFloat() {
		switch valTok.Kind {
		case TOK_FLOAT:
			in.ImmF = valTok.FltVal
		case TOK_INT:
			in.ImmF = float64(valTok.IntVal)
		default:
			p.errorf(valTok, "expected float literal, got %q", valTok.Value)
			return Instr{}, false
		}
	} else {
		if valTok.Kind != TOK_INT {
			p.errorf(valTok, "expected int literal, got %q", valTok.Value)
			return Instr{}, false
		}
		in.ImmI = valTok.IntVal
	}
	p.skipNewlines()
	return in, true
}

// <op> <type> %s1 %s2
func (p *Parser) parseBinop(in Instr, kind InstrKind) (Instr, bool) {
	in.Kind = kind
	typeTok := p.advance()
	ty, ok := parseType(typeTok.Value)
	if !ok {
		p.errorf(typeTok, "unknown type in binop: %q", typeTok.Value)
		return Instr{}, false
	}
	in.Type = ty
	s1, ok := p.parseVreg()
	if !ok {
		return Instr{}, false
	}
	s2, ok := p.parseVreg()
	if !ok {
		return Instr{}, false
	}
	in.Src = []int{s1, s2}
	p.skipNewlines()
	return in, true
}

// <op> <type> %s1
func (p *Parser) parseUnop(in Instr, kind InstrKind) (Instr, bool) {
	in.Kind = kind
	typeTok := p.advance()
	ty, ok := parseType(typeTok.Value)
	if !ok {
		p.errorf(typeTok, "unknown type in unop: %q", typeTok.Value)
		return Instr{}, false
	}
	in.Type = ty
	s1, ok := p.parseVreg()
	if !ok {
		return Instr{}, false
	}
	in.Src = []int{s1}
	p.skipNewlines()
	return in, true
}

// goto <label>
func (p *Parser) parseGoto(in Instr) (Instr, bool) {
	in.Kind = INS_GOTO
	labelTok, ok := p.expect(TOK_IDENT)
	if !ok {
		return Instr{}, false
	}
	in.Label = labelTok.Value
	p.skipNewlines()
	return in, true
}

// if_goto %cond <label>
func (p *Parser) parseIfGoto(in Instr) (Instr, bool) {
	in.Kind = INS_IF_GOTO
	cond, ok := p.parseVreg()
	if !ok {
		return Instr{}, false
	}
	labelTok, ok := p.expect(TOK_IDENT)
	if !ok {
		return Instr{}, false
	}
	in.Src = []int{cond}
	in.Label = labelTok.Value
	in.Type = TY_BOOL
	p.skipNewlines()
	return in, true
}

// ret [<type> %val]
func (p *Parser) parseRet(in Instr) (Instr, bool) {
	in.Kind = INS_RET
	// Optional return value
	tok := p.peek()
	if tok.Kind == TOK_NEWLINE || tok.Kind == TOK_RBRACE || tok.Kind == TOK_EOF {
		in.Type = TY_VOID
		p.skipNewlines()
		return in, true
	}
	// Try to parse type
	ty, ok := parseType(tok.Value)
	if ok {
		p.advance()
		in.Type = ty
		vreg, ok := p.parseVreg()
		if !ok {
			return Instr{}, false
		}
		in.Src = []int{vreg}
	} else if tok.Kind == TOK_PERCENT {
		// ret %vreg — infer type as i32
		vreg, ok := p.parseVreg()
		if !ok {
			return Instr{}, false
		}
		in.Src = []int{vreg}
		in.Type = TY_I32
	}
	p.skipNewlines()
	return in, true
}

// puts @label
func (p *Parser) parsePuts(in Instr) (Instr, bool) {
	in.Kind = INS_PUTS
	if _, ok := p.expect(TOK_AT); !ok {
		return Instr{}, false
	}
	nameTok, ok := p.expect(TOK_IDENT)
	if !ok {
		return Instr{}, false
	}
	in.Label = nameTok.Value
	in.Type = TY_VOID
	p.skipNewlines()
	return in, true
}

// [%dst =] call @func(%0, %1, ...)
func (p *Parser) parseCall(in Instr) (Instr, bool) {
	in.Kind = INS_CALL
	if _, ok := p.expect(TOK_AT); !ok {
		return Instr{}, false
	}
	nameTok, ok := p.expect(TOK_IDENT)
	if !ok {
		return Instr{}, false
	}
	in.Label = nameTok.Value

	if _, ok := p.expect(TOK_LPAREN); !ok {
		return Instr{}, false
	}
	for !p.check(TOK_RPAREN) && !p.check(TOK_EOF) {
		vreg, ok := p.parseVreg()
		if !ok {
			return Instr{}, false
		}
		in.Src = append(in.Src, vreg)
		if !p.match(TOK_COMMA) {
			break
		}
	}
	if _, ok := p.expect(TOK_RPAREN); !ok {
		return Instr{}, false
	}
	in.ArgCount = len(in.Src)
	in.Type = TY_I32 // default; checker will resolve
	p.skipNewlines()
	return in, true
}

// [%dst =] syscall <nr> %a0 %a1 ...
// syscall 64 %0 %1 %2   => sys_write
func (p *Parser) parseSyscall(in Instr) (Instr, bool) {
	in.Kind = INS_SYSCALL
	nrTok, ok := p.expect(TOK_INT)
	if !ok {
		return Instr{}, false
	}
	in.ImmI = nrTok.IntVal // syscall number
	// remaining args are vregs
	for {
		tok := p.peek()
		if tok.Kind != TOK_PERCENT {
			break
		}
		vreg, ok := p.parseVreg()
		if !ok {
			break
		}
		in.Src = append(in.Src, vreg)
	}
	in.ArgCount = len(in.Src)
	in.Type = TY_I64
	p.skipNewlines()
	return in, true
}

// %dst = alloc <type> <count>
func (p *Parser) parseAlloc(in Instr) (Instr, bool) {
	in.Kind = INS_ALLOC
	typeTok := p.advance()
	ty, ok := parseType(typeTok.Value)
	if !ok {
		p.errorf(typeTok, "unknown type in alloc: %q", typeTok.Value)
		return Instr{}, false
	}
	in.Type = ty
	// optional count
	tok := p.peek()
	if tok.Kind == TOK_INT {
		p.advance()
		in.ImmI = tok.IntVal
	} else {
		in.ImmI = 1
	}
	in.Dst = in.Dst // pointer result
	p.skipNewlines()
	return in, true
}

// %dst = load <type> %ptr
func (p *Parser) parseLoad(in Instr) (Instr, bool) {
	in.Kind = INS_LOAD
	typeTok := p.advance()
	ty, ok := parseType(typeTok.Value)
	if !ok {
		p.errorf(typeTok, "unknown type in load: %q", typeTok.Value)
		return Instr{}, false
	}
	in.Type = ty
	ptr, ok := p.parseVreg()
	if !ok {
		return Instr{}, false
	}
	in.Src = []int{ptr}
	p.skipNewlines()
	return in, true
}

// store <type> %ptr %val
func (p *Parser) parseStore(in Instr) (Instr, bool) {
	in.Kind = INS_STORE
	typeTok := p.advance()
	ty, ok := parseType(typeTok.Value)
	if !ok {
		p.errorf(typeTok, "unknown type in store: %q", typeTok.Value)
		return Instr{}, false
	}
	in.Type = ty
	ptr, ok := p.parseVreg()
	if !ok {
		return Instr{}, false
	}
	val, ok := p.parseVreg()
	if !ok {
		return Instr{}, false
	}
	in.Src = []int{ptr, val}
	p.skipNewlines()
	return in, true
}

// ── Helpers ────────────────────────────────────

func (p *Parser) parseVreg() (int, bool) {
	if _, ok := p.expect(TOK_PERCENT); !ok {
		return -1, false
	}
	numTok, ok := p.expect(TOK_INT)
	if !ok {
		return -1, false
	}
	return int(numTok.IntVal), true
}
// strlen @global -> %dst
func (p *Parser) parseStrlen(in Instr) (Instr, bool) {
	in.Kind = INS_STRLEN
	if _, ok := p.expect(TOK_AT); !ok {
		return Instr{}, false
	}
	nameTok, ok := p.expect(TOK_IDENT)
	if !ok {
		return Instr{}, false
	}
	in.Label = nameTok.Value
	in.Type = TY_I64
	p.skipNewlines()
	return in, true
}
