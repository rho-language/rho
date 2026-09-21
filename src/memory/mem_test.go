//
// @file mem_test.go
// @brief Rho Memory API tests.
//
// Tests the high-level memory API built on top of the Arena allocator.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package memory

import "testing"

/* -------------------------------------------------------------------------- */
/*  Tests: AllocBytes                                                          */
/* -------------------------------------------------------------------------- */

func TestAllocBytes(t *testing.T) {
    a := NewArena(4)

    b, err := AllocBytes(a, 4)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if len(b) != 4 {
        t.Fatalf("expected 4 bytes")
    }
}

/* -------------------------------------------------------------------------- */
/*  Tests: AllocString                                                         */
/* -------------------------------------------------------------------------- */

func TestAllocString(t *testing.T) {
    a := NewArena(16)

    s, err := AllocString(a, "rho")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if s != "rho" {
        t.Fatalf("expected string 'rho'")
    }
}

/* -------------------------------------------------------------------------- */
/*  Tests: Out of Memory                                                       */
/* -------------------------------------------------------------------------- */

func TestAllocStringOutOfMemory(t *testing.T) {
    a := NewArena(2)

    _, err := AllocString(a, "rho") // needs 3 bytes
    if err == nil {
        t.Fatalf("expected out-of-memory error")
    }
}
