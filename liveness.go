package main

import "fmt"

// ─────────────────────────────────────────────
//  liveness.go — Live Range Analysis
//
//  Computes, for each virtual register in a function:
//    Start: index of first definition
//    End:   index of last use
//
//  Used by the linear scan allocator to determine
//  which vregs are live at each program point.
// ─────────────────────────────────────────────

// LiveRange describes the lifetime of a virtual register.
type LiveRange struct {
	Vreg  int
	Type  Type
	Start int // instruction index of first def
	End   int // instruction index of last use
}

func (lr LiveRange) String() string {
	return fmt.Sprintf("%%%d(%s) [%d, %d]", lr.Vreg, lr.Type, lr.Start, lr.End)
}

// ComputeLiveness performs a single-pass liveness analysis over fn.
// Returns a slice of LiveRange sorted by Start.
func ComputeLiveness(fn *Function) []LiveRange {
	// vreg → (start, end, type)
	type entry struct {
		start int
		end   int
		ty    Type
	}
	table := map[int]*entry{}

	def := func(vreg int, ty Type, idx int) {
		if vreg < 0 {
			return
		}
		if e, ok := table[vreg]; ok {
			// re-def — extend if needed
			if idx < e.start {
				e.start = idx
			}
		} else {
			table[vreg] = &entry{start: idx, end: idx, ty: ty}
		}
	}

	use := func(vreg int, ty Type, idx int) {
		if vreg < 0 {
			return
		}
		if e, ok := table[vreg]; ok {
			if idx > e.end {
				e.end = idx
			}
		} else {
			// used before defined (e.g. function param) — start at 0
			table[vreg] = &entry{start: 0, end: idx, ty: ty}
		}
	}

	// Seed params as defined at instruction -1 (before body)
	for _, p := range fn.Params {
		table[p.Vreg] = &entry{start: 0, end: 0, ty: p.Type}
	}

	for idx, in := range fn.Instrs {
		// Record definition
		if in.Dst >= 0 {
			def(in.Dst, in.Type, idx)
		}
		// Record uses
		for _, s := range in.Src {
			use(s, in.Type, idx)
		}
	}

	// Convert to slice
	ranges := make([]LiveRange, 0, len(table))
	for vreg, e := range table {
		ranges = append(ranges, LiveRange{
			Vreg:  vreg,
			Type:  e.ty,
			Start: e.start,
			End:   e.end,
		})
	}

	// Sort by start point
	sortLiveRanges(ranges)
	return ranges
}

// sortLiveRanges sorts in place by Start (insertion sort — small N).
func sortLiveRanges(r []LiveRange) {
	for i := 1; i < len(r); i++ {
		key := r[i]
		j := i - 1
		for j >= 0 && r[j].Start > key.Start {
			r[j+1] = r[j]
			j--
		}
		r[j+1] = key
	}
}
