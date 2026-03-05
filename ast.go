package main

import (
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────
//  ast.go — EclipseIR AST / IR Nodes
// ─────────────────────────────────────────────

// InstrKind identifies the operation of an IR instruction.
type InstrKind int

const (
	INS_INVALID InstrKind = iota
	// Arithmetic
	INS_CONST
	INS_ADD
	INS_SUB
	INS_MUL
	INS_DIV
	INS_MOD
	INS_NEG
	// Bitwise / logical
	INS_AND
	INS_OR
	INS_XOR
	INS_NOT
	INS_SHL
	INS_SHR
	// Comparisons
	INS_CMP_GT
	INS_CMP_LT
	INS_CMP_EQ
	INS_CMP_GE
	INS_CMP_LE
	INS_CMP_NE
	// Control flow
	INS_LABEL
	INS_GOTO
	INS_IF_GOTO
	INS_RET
	// Memory
	INS_ALLOC  // alloc <type> <size> -> ptr
	INS_LOAD   // load <type> %ptr -> dst
	INS_STORE  // store <type> %ptr %src
	// Calls
	INS_CALL
	INS_SYSCALL // syscall <nr> %a0 %a1 ...
	// I/O (high level builtins retained for compatibility)
	INS_PUTS
	INS_STRLEN // strlen @global -> dst (i64 length)
)

func (k InstrKind) String() string {
	names := map[InstrKind]string{
		INS_INVALID: "invalid",
		INS_CONST:   "const",
		INS_ADD:     "add",
		INS_SUB:     "sub",
		INS_MUL:     "mul",
		INS_DIV:     "div",
		INS_MOD:     "mod",
		INS_NEG:     "neg",
		INS_AND:     "and",
		INS_OR:      "or",
		INS_XOR:     "xor",
		INS_NOT:     "not",
		INS_SHL:     "shl",
		INS_SHR:     "shr",
		INS_CMP_GT:  "gt",
		INS_CMP_LT:  "lt",
		INS_CMP_EQ:  "eq",
		INS_CMP_GE:  "ge",
		INS_CMP_LE:  "le",
		INS_CMP_NE:  "ne",
		INS_LABEL:   "label",
		INS_GOTO:    "goto",
		INS_IF_GOTO: "if_goto",
		INS_RET:     "ret",
		INS_ALLOC:   "alloc",
		INS_LOAD:    "load",
		INS_STORE:   "store",
		INS_CALL:    "call",
		INS_SYSCALL: "syscall",
		INS_PUTS:    "puts",
		INS_STRLEN:  "strlen",
	}
	if s, ok := names[k]; ok {
		return s
	}
	return fmt.Sprintf("instr(%d)", int(k))
}

// Instr is a single three-address-code instruction in the IR.
type Instr struct {
	Kind     InstrKind
	Dst      int     // destination virtual register (-1 = no dest)
	Src      []int   // source virtual registers
	ImmI     int64   // integer immediate
	ImmF     float64 // float immediate
	Type     Type    // value type
	Label    string  // label/function name
	ArgCount int     // number of call arguments
	Line     int     // source line for diagnostics
}

func (in Instr) String() string {
	var b strings.Builder
	if in.Dst >= 0 {
		fmt.Fprintf(&b, "%%%d = ", in.Dst)
	}
	b.WriteString(in.Kind.String())
	b.WriteString(" ")
	b.WriteString(in.Type.String())
	for _, s := range in.Src {
		fmt.Fprintf(&b, " %%%d", s)
	}
	if in.Kind == INS_CONST {
		if in.Type.IsFloat() {
			fmt.Fprintf(&b, " %g", in.ImmF)
		} else {
			fmt.Fprintf(&b, " %d", in.ImmI)
		}
	}
	if in.Label != "" {
		fmt.Fprintf(&b, " @%s", in.Label)
	}
	return b.String()
}

// Param is a named typed function parameter.
type Param struct {
	Name  string // may be empty for unnamed params
	Vreg  int    // virtual register assigned
	Type  Type
}

// Function is a top-level IR function.
type Function struct {
	Name    string
	RetType Type
	Params  []Param
	Instrs  []Instr
	// Metadata
	IsExtern bool // declared but not defined
}

// Global is a top-level data value.
type Global struct {
	Name  string
	Value string // raw string value
	ImmI  int64
	ImmF  float64
	Type  Type
}

// Program is the full parsed IR program.
type Program struct {
	Globals []Global
	Funcs   []Function
}

// ── Stats ──────────────────────────────────────

func (p *Program) PrintStats(log *Logger) {
	totalInstrs := 0
	for _, f := range p.Funcs {
		totalInstrs += len(f.Instrs)
	}
	log.Info(fmt.Sprintf("  globals    : %d", len(p.Globals)))
	log.Info(fmt.Sprintf("  functions  : %d", len(p.Funcs)))
	log.Info(fmt.Sprintf("  instructions: %d", totalInstrs))
	for _, f := range p.Funcs {
		log.Info(fmt.Sprintf("    %-20s  %d instrs  (%s)", f.Name, len(f.Instrs), f.RetType))
	}
}

// ── IR Dump ────────────────────────────────────

func (p *Program) DumpIR() {
	fmt.Println("// ── EclipseIR Dump ──────────────────────")
	for _, g := range p.Globals {
		if g.Type == TY_STR {
			fmt.Printf("@%s = c\"%s\"\n", g.Name, g.Value)
		} else if g.Type.IsFloat() {
			fmt.Printf("@%s = %s %g\n", g.Name, g.Type, g.ImmF)
		} else {
			fmt.Printf("@%s = %s %d\n", g.Name, g.Type, g.ImmI)
		}
	}
	fmt.Println()
	for _, f := range p.Funcs {
		params := make([]string, len(f.Params))
		for i, p := range f.Params {
			params[i] = fmt.Sprintf("%s %%%d", p.Type, p.Vreg)
		}
		fmt.Printf("func @%s(%s) -> %s {\n", f.Name, strings.Join(params, ", "), f.RetType)
		for _, in := range f.Instrs {
			if in.Kind == INS_LABEL {
				fmt.Printf("  %s:\n", in.Label)
			} else {
				fmt.Printf("    %s\n", in)
			}
		}
		fmt.Println("}")
		fmt.Println()
	}
}
