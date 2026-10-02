//
// @file string_test.go
// @brief Rho Compiler - IR String tests.
//
// This file contains deterministic unit tests for the Rho StringIR type.
// Tests validate correct construction of IR strings, length semantics,
// emptiness checks, and deterministic equality comparison.
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
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Constructors                                                        */
/* -------------------------------------------------------------------------- */

func TestNewStringIR(t *testing.T) {
	s := NewStringIR("rho")

	if s.Data != "rho" {
		t.Fatalf("expected Data='rho', got %q", s.Data)
	}
	if s.Length != 3 {
		t.Fatalf("expected Length=3, got %d", s.Length)
	}
}

func TestNewEmptyStringIR(t *testing.T) {
	s := NewEmptyStringIR()

	if s.Data != "" {
		t.Fatalf("expected empty Data, got %q", s.Data)
	}
	if s.Length != 0 {
		t.Fatalf("expected Length=0, got %d", s.Length)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: IsEmpty()                                                          */
/* -------------------------------------------------------------------------- */

func TestStringIsEmpty(t *testing.T) {
	s1 := NewStringIR("rho")
	if s1.IsEmpty() {
		t.Fatalf("expected non-empty string")
	}

	s2 := NewEmptyStringIR()
	if !s2.IsEmpty() {
		t.Fatalf("expected empty string")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Equals()                                                           */
/* -------------------------------------------------------------------------- */

func TestStringEquals(t *testing.T) {
	a := NewStringIR("abc")
	b := NewStringIR("abc")
	c := NewStringIR("xyz")
	d := NewStringIR("abcd")

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
