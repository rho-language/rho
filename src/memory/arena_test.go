//
// @file arena_test.go
// @brief Rho Memory Arena tests.
//
// Tests the deterministic bump-pointer arena allocator.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package memory

import "testing"

/* -------------------------------------------------------------------------- */
/*  Tests: Basic Allocation                                                    */
/* -------------------------------------------------------------------------- */

func TestArenaAlloc(t *testing.T) {
	a := NewArena(16)

	b1, ok1 := a.Alloc(8)
	if !ok1 || len(b1) != 8 {
		t.Fatalf("expected allocation of 8 bytes")
	}

	b2, ok2 := a.Alloc(8)
	if !ok2 || len(b2) != 8 {
		t.Fatalf("expected allocation of another 8 bytes")
	}

	if a.Used() != 16 {
		t.Fatalf("expected Used=16, got %d", a.Used())
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Out of Memory                                                       */
/* -------------------------------------------------------------------------- */

func TestArenaOutOfMemory(t *testing.T) {
	a := NewArena(8)

	_, ok := a.Alloc(8)
	if !ok {
		t.Fatalf("expected first allocation to succeed")
	}

	_, ok = a.Alloc(1)
	if ok {
		t.Fatalf("expected second allocation to fail")
	}
}

func TestArenaRejectsNegativeAllocation(t *testing.T) {
	a := NewArena(8)
	if _, ok := a.Alloc(-1); ok {
		t.Fatalf("expected negative allocation to fail")
	}
	if a.Used() != 0 {
		t.Fatalf("negative allocation changed arena usage")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Reset                                                               */
/* -------------------------------------------------------------------------- */

func TestArenaReset(t *testing.T) {
	a := NewArena(8)

	_, ok := a.Alloc(8)
	if !ok {
		t.Fatalf("expected allocation to succeed")
	}

	a.Reset()

	if a.Used() != 0 {
		t.Fatalf("expected Used=0 after reset")
	}

	_, ok = a.Alloc(8)
	if !ok {
		t.Fatalf("expected allocation after reset to succeed")
	}
}
