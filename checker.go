package main

import (
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────
//  checker.go — EclipseIR Semantic Checker
//
//  Validates:
//  - Virtual register definitions before use
//  - Type consistency across binops/cmp
//  - Return type matches function signature
//  - Labels referenced in goto/if_goto exist
//  - Call targets exist (if defined in program)
//  - Syscall argument count
//  - Alloc/load/store pointer types
// ─────────────────────────────────────────────

type CheckError struct {
	Func string
	Line int
	Msg  string
}

func (e CheckError) Error() string {
	if e.Func != "" {
		return fmt.Sprintf("check error in %q [line %d]: %s", e.Func, e.Line, e.Msg)
	}
	return fmt.Sprintf("check error [line %d]: %s", e.Line, e.Msg)
}

type Checker struct {
	prog   *Program
	errors []CheckError
}

func NewChecker(prog *Program) *Checker {
	return &Checker{prog: prog}
}

func (c *Checker) Check() []CheckError {
	// Build global function name index
	funcIndex := map[string]*Function{}
	for i := range c.prog.Funcs {
		f := &c.prog.Funcs[i]
		funcIndex[f.Name] = f
	}

	// Build global name index
	globalIndex := map[string]*Global{}
	for i := range c.prog.Globals {
		g := &c.prog.Globals[i]
		globalIndex[g.Name] = g
	}

	for i := range c.prog.Funcs {
		c.checkFunc(&c.prog.Funcs[i], funcIndex, globalIndex)
	}
	return c.errors
}

func (c *Checker) errorf(fn string, line int, format string, args ...interface{}) {
	c.errors = append(c.errors, CheckError{
		Func: fn,
		Line: line,
		Msg:  fmt.Sprintf(format, args...),
	})
}

func (c *Checker) checkFunc(fn *Function, funcIdx map[string]*Function, globalIdx map[string]*Global) {
	// vreg → type map
	vregTypes := map[int]Type{}

	// Seed with params
	for _, p := range fn.Params {
		vregTypes[p.Vreg] = p.Type
	}

	// Collect labels defined in this function
	labels := map[string]bool{}
	for _, in := range fn.Instrs {
		if in.Kind == INS_LABEL {
			if labels[in.Label] {
				c.errorf(fn.Name, in.Line, "duplicate label %q", in.Label)
			}
			labels[in.Label] = true
		}
	}

	hasRet := false

	for idx := range fn.Instrs {
		in := &fn.Instrs[idx]

		// Check source vregs are defined
		for _, s := range in.Src {
			if _, ok := vregTypes[s]; !ok {
				c.errorf(fn.Name, in.Line,
					"instruction %s uses undefined vreg %%%d", in.Kind, s)
			}
		}

		switch in.Kind {
		case INS_CONST:
			if in.Dst < 0 {
				c.errorf(fn.Name, in.Line, "const must have a destination register")
				continue
			}
			vregTypes[in.Dst] = in.Type

		case INS_ADD, INS_SUB, INS_MUL, INS_DIV, INS_MOD:
			if len(in.Src) != 2 {
				c.errorf(fn.Name, in.Line, "%s requires exactly 2 source regs, got %d", in.Kind, len(in.Src))
				continue
			}
			t1, t2 := vregTypes[in.Src[0]], vregTypes[in.Src[1]]
			if t1 != t2 {
				c.warnTypeMismatch(fn.Name, in.Kind.String(), in.Line, t1, t2)
			}
			if in.Dst >= 0 {
				vregTypes[in.Dst] = in.Type
			}

		case INS_AND, INS_OR, INS_XOR, INS_SHL, INS_SHR:
			if len(in.Src) != 2 {
				c.errorf(fn.Name, in.Line, "%s requires 2 source regs", in.Kind)
			}
			if !in.Type.IsInt() {
				c.errorf(fn.Name, in.Line, "%s only valid on integer types, got %s", in.Kind, in.Type)
			}
			if in.Dst >= 0 {
				vregTypes[in.Dst] = in.Type
			}

		case INS_NOT, INS_NEG:
			if len(in.Src) != 1 {
				c.errorf(fn.Name, in.Line, "%s requires 1 source reg", in.Kind)
			}
			if in.Dst >= 0 {
				vregTypes[in.Dst] = in.Type
			}

		case INS_CMP_GT, INS_CMP_LT, INS_CMP_EQ, INS_CMP_GE, INS_CMP_LE, INS_CMP_NE:
			if len(in.Src) != 2 {
				c.errorf(fn.Name, in.Line, "comparison %s requires 2 source regs", in.Kind)
				continue
			}
			t1, t2 := vregTypes[in.Src[0]], vregTypes[in.Src[1]]
			if t1 != t2 {
				c.warnTypeMismatch(fn.Name, in.Kind.String(), in.Line, t1, t2)
			}
			if in.Dst >= 0 {
				vregTypes[in.Dst] = TY_BOOL
			}

		case INS_LABEL:
			// already collected above

		case INS_GOTO:
			if !labels[in.Label] {
				c.errorf(fn.Name, in.Line, "goto references undefined label %q", in.Label)
			}

		case INS_IF_GOTO:
			if len(in.Src) < 1 {
				c.errorf(fn.Name, in.Line, "if_goto requires a condition register")
			}
			if !labels[in.Label] {
				c.errorf(fn.Name, in.Line, "if_goto references undefined label %q", in.Label)
			}
			if len(in.Src) > 0 {
				ct := vregTypes[in.Src[0]]
				if ct != TY_BOOL && ct != TY_I32 && ct != TY_I64 && ct != 0 {
					c.errorf(fn.Name, in.Line,
						"if_goto condition should be bool or int, got %s", ct)
				}
			}

		case INS_RET:
			hasRet = true
			if fn.RetType == TY_VOID {
				if len(in.Src) > 0 {
					c.errorf(fn.Name, in.Line,
						"void function %q returns a value", fn.Name)
				}
			} else {
				if len(in.Src) == 0 {
					c.errorf(fn.Name, in.Line,
						"non-void function %q missing return value", fn.Name)
				} else {
					rt := vregTypes[in.Src[0]]
					if !typesCompatible(rt, fn.RetType) {
						c.errorf(fn.Name, in.Line,
							"return type mismatch: function returns %s, got %s",
							fn.RetType, rt)
					}
				}
			}

		case INS_STRLEN:
			if in.Dst < 0 {
				c.errorf(fn.Name, in.Line, "strlen must have a destination register")
			} else {
				vregTypes[in.Dst] = TY_I64
			}
			if _, ok := globalIdx[in.Label]; !ok {
				c.errorf(fn.Name, in.Line, "strlen references unknown global %q", in.Label)
			}

		case INS_PUTS:
			if _, ok := globalIdx[in.Label]; !ok {
				// Soft warning — could be external
				c.errorf(fn.Name, in.Line, "puts references unknown global %q", in.Label)
			}

		case INS_CALL:
			if in.Dst >= 0 {
				vregTypes[in.Dst] = in.Type
			}
			// Resolve return type from function index if available
			if target, ok := funcIdx[in.Label]; ok {
				if in.Dst >= 0 {
					vregTypes[in.Dst] = target.RetType
					in.Type = target.RetType
				}
				if len(in.Src) != len(target.Params) {
					c.errorf(fn.Name, in.Line,
						"call to %q: expected %d args, got %d",
						in.Label, len(target.Params), len(in.Src))
				}
			}

		case INS_SYSCALL:
			if in.ArgCount > 6 {
				c.errorf(fn.Name, in.Line,
					"syscall takes at most 6 args (AArch64 x0-x5), got %d", in.ArgCount)
			}
			if in.Dst >= 0 {
				vregTypes[in.Dst] = TY_I64
			}

		case INS_ALLOC:
			if in.Dst < 0 {
				c.errorf(fn.Name, in.Line, "alloc must have a destination register")
			} else {
				vregTypes[in.Dst] = TY_PTR
			}

		case INS_LOAD:
			if in.Dst < 0 {
				c.errorf(fn.Name, in.Line, "load must have a destination register")
			}
			if len(in.Src) < 1 {
				c.errorf(fn.Name, in.Line, "load requires a pointer source")
			} else {
				pt := vregTypes[in.Src[0]]
				if pt != TY_PTR && pt != TY_I64 && pt != 0 {
					c.errorf(fn.Name, in.Line,
						"load source must be a pointer, got %s", pt)
				}
			}
			if in.Dst >= 0 {
				vregTypes[in.Dst] = in.Type
			}

		case INS_STORE:
			if len(in.Src) < 2 {
				c.errorf(fn.Name, in.Line, "store requires pointer and value")
			} else {
				pt := vregTypes[in.Src[0]]
				if pt != TY_PTR && pt != TY_I64 && pt != 0 {
					c.errorf(fn.Name, in.Line,
						"store destination must be a pointer, got %s", pt)
				}
			}
		}
	}

	// Non-void functions must have at least one ret
	if fn.RetType != TY_VOID && !hasRet {
		c.errorf(fn.Name, 0, "non-void function %q has no return instruction", fn.Name)
	}
}

func (c *Checker) warnTypeMismatch(fn, op string, line int, t1, t2 Type) {
	// For now, emit as an error; could be downgraded to warning
	c.errorf(fn, line,
		"type mismatch in %s: left is %s, right is %s", op, t1, t2)
}

// typesCompatible returns true if src can be used where dst is expected.
func typesCompatible(src, dst Type) bool {
	if src == dst {
		return true
	}
	// Allow int widths to be interchangeable for now
	if src.IsInt() && dst.IsInt() {
		return true
	}
	// Allow float widths to be interchangeable
	if src.IsFloat() && dst.IsFloat() {
		return true
	}
	// Treat unknown (0 = TY_VOID when undefined) as compatible
	if src == TY_VOID || dst == TY_VOID {
		return true
	}
	return false
}

// FormatErrors formats checker errors into a readable string.
func FormatCheckErrors(errs []CheckError) string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = "  " + e.Error()
	}
	return strings.Join(msgs, "\n")
}
