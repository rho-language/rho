//
// @file range_test.go
// @brief Rho Compiler - IR Range tests.
//
// This file contains deterministic unit tests for the Rho RangeIR type.
// Tests validate correct construction of ranges, half-open semantics,
// step behavior, and helper functions such as Length() and IsEmpty().
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package ir

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Constructors                                                        */
/* -------------------------------------------------------------------------- */

func TestNewRangeIR(t *testing.T) {
	r := NewRangeIR(0, 10, 2)

	if r.Start != 0 {
		t.Fatalf("expected Start=0, got %d", r.Start)
	}
	if r.End != 10 {
		t.Fatalf("expected End=10, got %d", r.End)
	}
	if r.Step != 2 {
		t.Fatalf("expected Step=2, got %d", r.Step)
	}
}

func TestNewUnitRangeIR(t *testing.T) {
	r := NewUnitRangeIR(5, 15)

	if r.Start != 5 {
		t.Fatalf("expected Start=5, got %d", r.Start)
	}
	if r.End != 15 {
		t.Fatalf("expected End=15, got %d", r.End)
	}
	if r.Step != 1 {
		t.Fatalf("expected Step=1, got %d", r.Step)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Length()                                                           */
/* -------------------------------------------------------------------------- */

func TestRangeLength(t *testing.T) {
	r := NewRangeIR(0, 10, 2)
	if r.Length() != 5 {
		t.Fatalf("expected Length=5, got %d", r.Length())
	}

	r2 := NewRangeIR(3, 4, 1)
	if r2.Length() != 1 {
		t.Fatalf("expected Length=1, got %d", r2.Length())
	}

	r3 := NewRangeIR(10, 10, 1)
	if r3.Length() != 0 {
		t.Fatalf("expected Length=0 for empty range, got %d", r3.Length())
	}

	r4 := NewRangeIR(10, 5, 1)
	if r4.Length() != 0 {
		t.Fatalf("expected Length=0 for reversed half-open range, got %d", r4.Length())
	}

	r5 := NewRangeIR(1, 5, 2)
	if r5.Length() != 2 {
		t.Fatalf("expected Length=2 for [1,5) step 2, got %d", r5.Length())
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: IsEmpty()                                                          */
/* -------------------------------------------------------------------------- */

func TestRangeIsEmpty(t *testing.T) {
	r := NewRangeIR(0, 10, 1)
	if r.IsEmpty() {
		t.Fatalf("expected non-empty range")
	}

	r2 := NewRangeIR(5, 5, 1)
	if !r2.IsEmpty() {
		t.Fatalf("expected empty range when start==end")
	}

	r3 := NewRangeIR(10, 5, 1)
	if !r3.IsEmpty() {
		t.Fatalf("expected empty range when start>end")
	}
}
