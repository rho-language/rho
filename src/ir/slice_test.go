//
// @file slice_test.go
// @brief Rho Compiler - IR Slice tests.
//
// This file contains deterministic unit tests for the Rho SliceIR type.
// Tests validate correct construction of IR slices, length/capacity
// semantics, emptiness checks, and deterministic equality comparison.
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

func TestNewSliceIR(t *testing.T) {
	s := NewSliceIR("abcdef")

	if s.Data != "abcdef" {
		t.Fatalf("expected Data='abcdef', got %q", s.Data)
	}
	if s.Length != 6 {
		t.Fatalf("expected Length=6, got %d", s.Length)
	}
	if s.Capacity != 6 {
		t.Fatalf("expected Capacity=6, got %d", s.Capacity)
	}
}

func TestNewEmptySliceIR(t *testing.T) {
	s := NewEmptySliceIR()

	if s.Data != "" {
		t.Fatalf("expected empty Data, got %q", s.Data)
	}
	if s.Length != 0 {
		t.Fatalf("expected Length=0, got %d", s.Length)
	}
	if s.Capacity != 0 {
		t.Fatalf("expected Capacity=0, got %d", s.Capacity)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: IsEmpty()                                                          */
/* -------------------------------------------------------------------------- */

func TestSliceIsEmpty(t *testing.T) {
	s1 := NewSliceIR("rho")
	if s1.IsEmpty() {
		t.Fatalf("expected non-empty slice")
	}

	s2 := NewEmptySliceIR()
	if !s2.IsEmpty() {
		t.Fatalf("expected empty slice")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Equals()                                                           */
/* -------------------------------------------------------------------------- */

func TestSliceEquals(t *testing.T) {
	a := NewSliceIR("abc")
	b := NewSliceIR("abc")
	c := NewSliceIR("xyz")
	d := NewSliceIR("abcd")

	if !a.Equals(b) {
		t.Fatalf("expected 'abc' == 'abc'")
	}

	if a.Equals(c) {
		t.Fatalf("expected 'abc' != 'xyz'")
	}

	if a.Equals(d) {
		t.Fatalf("expected 'abc' != 'abcd'")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: HasCapacity()                                                      */
/* -------------------------------------------------------------------------- */

func TestSliceHasCapacity(t *testing.T) {
	s := NewSliceIR("abcdef") // length=6, capacity=6

	if !s.HasCapacity(6) {
		t.Fatalf("expected capacity >= 6")
	}

	if !s.HasCapacity(3) {
		t.Fatalf("expected capacity >= 3")
	}

	if s.HasCapacity(7) {
		t.Fatalf("expected capacity < 7")
	}
}
