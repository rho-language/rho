//
// @file range.go
// @brief Rho Compiler - IR Range type.
//
// This file defines the IR representation of the Rho Range type.
// Range in Rho is a deterministic, half-open interval with explicit
// start, end, and step fields, designed to map directly into C
// (`uint64_t start; uint64_t end; uint64_t step;`).
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package ir

/* -------------------------------------------------------------------------- */
/*  Range Type                                                                */
/* -------------------------------------------------------------------------- */

// RangeIR represents a Rho range:
//
//	start..end step step
//
// It is always half-open [start, end), with a positive step.
type RangeIR struct {
	Start uint64
	End   uint64
	Step  uint64
}

/* -------------------------------------------------------------------------- */
/*  Constructors                                                              */
/* -------------------------------------------------------------------------- */

// NewRangeIR creates a new RangeIR with the given start, end, and step.
// Step must be > 0; no implicit validation is performed here.
func NewRangeIR(start, end, step uint64) RangeIR {
	return RangeIR{
		Start: start,
		End:   end,
		Step:  step,
	}
}

// NewUnitRangeIR creates a simple unit step range [start, end).
func NewUnitRangeIR(start, end uint64) RangeIR {
	return RangeIR{
		Start: start,
		End:   end,
		Step:  1,
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

// Length returns the number of steps in the range, assuming
// half-open semantics and positive step.
func (r RangeIR) Length() uint64 {
	if r.End <= r.Start || r.Step == 0 {
		return 0
	}
	distance := r.End - r.Start
	return 1 + (distance-1)/r.Step
}

// IsEmpty reports whether the range contains no elements.
func (r RangeIR) IsEmpty() bool {
	return r.Length() == 0
}
