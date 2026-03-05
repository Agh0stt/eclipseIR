package main

// ─────────────────────────────────────────────
//  optimizer.go — EclipseIR Optimization Passes
//
//  Passes (applied in order when --optimize):
//  1. Constant folding  — folds const + const at compile time
//  2. Dead code elim    — removes instructions after unconditional goto/ret
//  3. Trivial copy prop — replaces %a = %b patterns
// ─────────────────────────────────────────────

func (p *Program) Optimize() {
	for i := range p.Funcs {
		f := &p.Funcs[i]
		constFold(f)
		deadCodeElim(f)
	}
}

// ── Pass 1: Constant Folding ───────────────────
// If both sources are constants, fold into a single const instruction.

func constFold(fn *Function) {
	// Build a map of vreg → immediate value for known constants
	type constVal struct {
		isInt   bool
		isFloat bool
		ival    int64
		fval    float64
		ty      Type
	}
	consts := map[int]constVal{}

	for i := range fn.Instrs {
		in := &fn.Instrs[i]
		if in.Kind == INS_CONST {
			cv := constVal{ty: in.Type}
			if in.Type.IsFloat() {
				cv.isFloat = true
				cv.fval = in.ImmF
			} else {
				cv.isInt = true
				cv.ival = in.ImmI
			}
			if in.Dst >= 0 {
				consts[in.Dst] = cv
			}
			continue
		}

		// Try folding binary arithmetic
		if len(in.Src) == 2 && in.Dst >= 0 {
			c1, ok1 := consts[in.Src[0]]
			c2, ok2 := consts[in.Src[1]]
			if ok1 && ok2 && c1.isInt && c2.isInt {
				folded, ok := foldInt(in.Kind, c1.ival, c2.ival)
				if ok {
					in.Kind = INS_CONST
					in.ImmI = folded
					in.Src = nil
					in.Type = c1.ty
					consts[in.Dst] = constVal{isInt: true, ival: folded, ty: c1.ty}
					continue
				}
			}
			if ok1 && ok2 && c1.isFloat && c2.isFloat {
				folded, ok := foldFloat(in.Kind, c1.fval, c2.fval)
				if ok {
					in.Kind = INS_CONST
					in.ImmF = folded
					in.Src = nil
					in.Type = c1.ty
					consts[in.Dst] = constVal{isFloat: true, fval: folded, ty: c1.ty}
					continue
				}
			}
		}
	}
}

func foldInt(kind InstrKind, a, b int64) (int64, bool) {
	switch kind {
	case INS_ADD:
		return a + b, true
	case INS_SUB:
		return a - b, true
	case INS_MUL:
		return a * b, true
	case INS_DIV:
		if b == 0 {
			return 0, false
		}
		return a / b, true
	case INS_MOD:
		if b == 0 {
			return 0, false
		}
		return a % b, true
	case INS_AND:
		return a & b, true
	case INS_OR:
		return a | b, true
	case INS_XOR:
		return a ^ b, true
	case INS_SHL:
		return a << uint(b), true
	case INS_SHR:
		return a >> uint(b), true
	case INS_CMP_GT:
		if a > b {
			return 1, true
		}
		return 0, true
	case INS_CMP_LT:
		if a < b {
			return 1, true
		}
		return 0, true
	case INS_CMP_EQ:
		if a == b {
			return 1, true
		}
		return 0, true
	case INS_CMP_GE:
		if a >= b {
			return 1, true
		}
		return 0, true
	case INS_CMP_LE:
		if a <= b {
			return 1, true
		}
		return 0, true
	case INS_CMP_NE:
		if a != b {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func foldFloat(kind InstrKind, a, b float64) (float64, bool) {
	switch kind {
	case INS_ADD:
		return a + b, true
	case INS_SUB:
		return a - b, true
	case INS_MUL:
		return a * b, true
	case INS_DIV:
		if b == 0 {
			return 0, false
		}
		return a / b, true
	}
	return 0, false
}

// ── Pass 2: Dead Code Elimination ─────────────
// Remove instructions that follow an unconditional goto or ret
// within a basic block (i.e. before the next label).

func deadCodeElim(fn *Function) {
	var out []Instr
	dead := false
	for _, in := range fn.Instrs {
		if in.Kind == INS_LABEL {
			dead = false // new block starts
		}
		if !dead {
			out = append(out, in)
		}
		if in.Kind == INS_GOTO || in.Kind == INS_RET {
			dead = true
		}
	}
	fn.Instrs = out
}
