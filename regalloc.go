package main

import (
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────
//  regalloc.go — Linear Scan Register Allocator
//               with Stack Spilling
//
//  Physical register pools (AArch64):
//    Int/bool   → w9-w19   (11 caller-saved scratch regs)
//    Wide/ptr   → x9-x19   (same regs, 64-bit view)
//    Float f32  → s8-s15   (8 float scratch regs)
//    Float f64  → d8-d15
//
//  Reserved:
//    w0-w7 / x0-x7  — ABI args/return (used transiently)
//    x8              — syscall number
//    x20             — spill reload scratch (int/wide)
//    s16/d16         — spill reload scratch (float)
//    x29             — frame pointer
//    x30             — link register
//    sp              — stack pointer
//
//  Spill layout (from x29 downward, 16-byte aligned slots):
//    [x29, #-16]  slot 0
//    [x29, #-32]  slot 1
//    ...
// ─────────────────────────────────────────────

var intPool = []string{
	"w9", "w10", "w11", "w12", "w13", "w14", "w15", "w16", "w17", "w18", "w19",
}
var widePool = []string{
	"x9", "x10", "x11", "x12", "x13", "x14", "x15", "x16", "x17", "x18", "x19",
}
var f32Pool = []string{
	"s8", "s9", "s10", "s11", "s12", "s13", "s14", "s15",
}
var f64Pool = []string{
	"d8", "d9", "d10", "d11", "d12", "d13", "d14", "d15",
}

type RegClass int

const (
	RC_INT RegClass = iota
	RC_FLOAT32
	RC_FLOAT64
	RC_WIDE
)

func regClassOf(t Type) RegClass {
	switch t {
	case TY_F32:
		return RC_FLOAT32
	case TY_F64:
		return RC_FLOAT64
	case TY_I64, TY_U64, TY_PTR, TY_STR:
		return RC_WIDE
	default:
		return RC_INT
	}
}

type VregLoc int

const (
	LOC_REG   VregLoc = iota
	LOC_SPILL
)

type AllocEntry struct {
	Loc     VregLoc
	PhysReg string
	SlotIdx int
	Type    Type
}

// ── LinearScanAlloc ───────────────────────────

type LinearScanAlloc struct {
	fn        *Function
	ranges    []LiveRange
	alloc     map[int]AllocEntry
	numSpills int
	frameSize int
	freeInt   []string
	freeWide  []string
	freeF32   []string
	freeF64   []string
	activeInt  []LiveRange
	activeWide []LiveRange
	activeF32  []LiveRange
	activeF64  []LiveRange
}

func NewLinearScanAlloc(fn *Function) *LinearScanAlloc {
	return &LinearScanAlloc{
		fn:       fn,
		alloc:    map[int]AllocEntry{},
		freeInt:  append([]string{}, intPool...),
		freeWide: append([]string{}, widePool...),
		freeF32:  append([]string{}, f32Pool...),
		freeF64:  append([]string{}, f64Pool...),
	}
}

func (la *LinearScanAlloc) Run() {
	la.ranges = ComputeLiveness(la.fn)
	for _, lr := range la.ranges {
		la.expireOld(lr)
		free := la.freeListFor(lr.Type)
		if len(*free) == 0 {
			la.spill(lr)
		} else {
			reg := (*free)[0]
			*free = (*free)[1:]
			la.alloc[lr.Vreg] = AllocEntry{Loc: LOC_REG, PhysReg: reg, Type: lr.Type}
			la.addActive(lr)
		}
	}
	la.frameSize = 16 + la.numSpills*16
	if la.frameSize%16 != 0 {
		la.frameSize = (la.frameSize+15) &^ 15
	}
	if la.frameSize < 32 {
		la.frameSize = 32
	}
}

func (la *LinearScanAlloc) expireOld(cur LiveRange) {
	la.expireList(&la.activeInt, &la.freeInt, cur.Start)
	la.expireList(&la.activeWide, &la.freeWide, cur.Start)
	la.expireList(&la.activeF32, &la.freeF32, cur.Start)
	la.expireList(&la.activeF64, &la.freeF64, cur.Start)
}

func (la *LinearScanAlloc) expireList(active *[]LiveRange, free *[]string, point int) {
	remaining := (*active)[:0]
	for _, lr := range *active {
		if lr.End < point {
			if e, ok := la.alloc[lr.Vreg]; ok && e.Loc == LOC_REG {
				*free = append(*free, e.PhysReg)
			}
		} else {
			remaining = append(remaining, lr)
		}
	}
	*active = remaining
}

func (la *LinearScanAlloc) spill(lr LiveRange) {
	slot := la.numSpills
	la.numSpills++
	la.alloc[lr.Vreg] = AllocEntry{Loc: LOC_SPILL, SlotIdx: slot, Type: lr.Type}
}

func (la *LinearScanAlloc) addActive(lr LiveRange) {
	switch regClassOf(lr.Type) {
	case RC_INT:
		la.activeInt = insertSortedByEnd(la.activeInt, lr)
	case RC_WIDE:
		la.activeWide = insertSortedByEnd(la.activeWide, lr)
	case RC_FLOAT32:
		la.activeF32 = insertSortedByEnd(la.activeF32, lr)
	case RC_FLOAT64:
		la.activeF64 = insertSortedByEnd(la.activeF64, lr)
	}
}

func (la *LinearScanAlloc) freeListFor(t Type) *[]string {
	switch regClassOf(t) {
	case RC_FLOAT32:
		return &la.freeF32
	case RC_FLOAT64:
		return &la.freeF64
	case RC_WIDE:
		return &la.freeWide
	default:
		return &la.freeInt
	}
}

func insertSortedByEnd(list []LiveRange, lr LiveRange) []LiveRange {
	list = append(list, lr)
	for i := len(list) - 1; i > 0 && list[i].End < list[i-1].End; i-- {
		list[i], list[i-1] = list[i-1], list[i]
	}
	return list
}

// spillOffset returns stack offset for slot i: [x29, #-offset]
func spillOffset(slotIdx int) int {
	return (slotIdx + 1) * 16
}

// ── RegAlloc (codegen interface) ──────────────

type RegAlloc struct {
	la          *LinearScanAlloc
	emitter     func(f string, args ...interface{})
	scratchInt  string
	scratchWide string
	scratchF32  string
	scratchF64  string
}

func NewRegAlloc() *RegAlloc {
	return &RegAlloc{
		scratchInt:  "w20",
		scratchWide: "x20",
		scratchF32:  "s16",
		scratchF64:  "d16",
	}
}

func (ra *RegAlloc) InitForFunc(fn *Function, emit func(string, ...interface{})) {
	legacyCache = map[int]string{} // reset per function
	la := NewLinearScanAlloc(fn)
	la.Run()
	ra.la = la
	ra.emitter = emit
}

func (ra *RegAlloc) FrameSize() int {
	if ra.la == nil {
		return 32
	}
	return ra.la.frameSize
}

func (ra *RegAlloc) NumSpills() int {
	if ra.la == nil {
		return 0
	}
	return ra.la.numSpills
}

func (ra *RegAlloc) Get(vreg int, t Type) string {
	if vreg < 0 {
		return ra.scratchFor(t)
	}
	if ra.la == nil {
		return ra.legacyName(vreg, t)
	}
	e, ok := ra.la.alloc[vreg]
	if !ok {
		return ra.scratchFor(t)
	}
	if e.Loc == LOC_REG {
		return e.PhysReg
	}
	// Spilled: emit reload into scratch
	offset := spillOffset(e.SlotIdx)
	scratch := ra.scratchFor(t)
	if ra.emitter != nil {
		ra.emitter("    ldr %s, [x29, #-%d]  // reload %%%d", scratch, offset, vreg)
	}
	return scratch
}

func (ra *RegAlloc) Assign(vreg int, t Type) string {
	if vreg < 0 {
		return ra.scratchFor(t)
	}
	if ra.la == nil {
		return ra.legacyAssign(vreg, t)
	}
	e, ok := ra.la.alloc[vreg]
	if !ok {
		return ra.scratchFor(t)
	}
	if e.Loc == LOC_REG {
		return e.PhysReg
	}
	return ra.scratchFor(t)
}

func (ra *RegAlloc) SpillDst(vreg int, t Type) {
	if ra.la == nil || vreg < 0 {
		return
	}
	e, ok := ra.la.alloc[vreg]
	if !ok || e.Loc != LOC_SPILL {
		return
	}
	offset := spillOffset(e.SlotIdx)
	scratch := ra.scratchFor(t)
	if ra.emitter != nil {
		ra.emitter("    str %s, [x29, #-%d]  // spill %%%d", scratch, offset, vreg)
	}
}

func (ra *RegAlloc) IsSpilled(vreg int) bool {
	if ra.la == nil {
		return false
	}
	e, ok := ra.la.alloc[vreg]
	return ok && e.Loc == LOC_SPILL
}

func (ra *RegAlloc) DumpAlloc() string {
	if ra.la == nil {
		return "  (no allocation data)\n"
	}
	var b strings.Builder
	for _, lr := range ra.la.ranges {
		e, ok := ra.la.alloc[lr.Vreg]
		if !ok {
			continue
		}
		if e.Loc == LOC_REG {
			fmt.Fprintf(&b, "  %%%d (%s) live[%d,%d] → %s\n",
				lr.Vreg, lr.Type, lr.Start, lr.End, e.PhysReg)
		} else {
			fmt.Fprintf(&b, "  %%%d (%s) live[%d,%d] → [x29,#-%d] (spill slot %d)\n",
				lr.Vreg, lr.Type, lr.Start, lr.End,
				spillOffset(e.SlotIdx), e.SlotIdx)
		}
	}
	fmt.Fprintf(&b, "  frame: %d bytes, spills: %d\n",
		ra.la.frameSize, ra.la.numSpills)
	return b.String()
}

func (ra *RegAlloc) DumpLiveness() string {
	if ra.la == nil {
		return "  (no liveness data)\n"
	}
	var b strings.Builder
	for _, lr := range ra.la.ranges {
		fmt.Fprintf(&b, "  %s\n", lr)
	}
	return b.String()
}

func (ra *RegAlloc) scratchFor(t Type) string {
	switch regClassOf(t) {
	case RC_FLOAT32:
		return ra.scratchF32
	case RC_FLOAT64:
		return ra.scratchF64
	case RC_WIDE:
		return ra.scratchWide
	default:
		return ra.scratchInt
	}
}

// ── Legacy fallback ───────────────────────────

var legacyCache = map[int]string{}

func (ra *RegAlloc) legacyAssign(vreg int, t Type) string {
	if s, ok := legacyCache[vreg]; ok {
		return s
	}
	name := legacyPhysName(vreg, t)
	legacyCache[vreg] = name
	return name
}

func (ra *RegAlloc) legacyName(vreg int, t Type) string {
	if s, ok := legacyCache[vreg]; ok {
		return s
	}
	return ra.legacyAssign(vreg, t)
}

func legacyPhysName(vreg int, t Type) string {
	const n = 11
	switch regClassOf(t) {
	case RC_FLOAT32:
		return fmt.Sprintf("s%d", 8+vreg%8)
	case RC_FLOAT64:
		return fmt.Sprintf("d%d", 8+vreg%8)
	case RC_WIDE:
		return fmt.Sprintf("x%d", 9+vreg%n)
	default:
		return fmt.Sprintf("w%d", 9+vreg%n)
	}
}

// ── widenReg ──────────────────────────────────

func widenReg(reg string) string {
	if len(reg) >= 2 && reg[0] == 'w' {
		return "x" + reg[1:]
	}
	return reg
}
