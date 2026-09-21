//
// @file mem.go
// @brief Rho Memory API.
//
// High-level memory API built on top of the Arena allocator.
// Provides typed allocation helpers.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package memory

import "fmt"

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// AllocBytes allocates a byte slice of size n.
func AllocBytes(a *Arena, n int) ([]byte, error) {
	b, ok := a.Alloc(n)
	if !ok {
		return nil, ErrOutOfMemory
	}
	return b, nil
}

// AllocString allocates a string inside the arena.
func AllocString(a *Arena, s string) (string, error) {
	b, ok := a.Alloc(len(s))
	if !ok {
		return "", ErrOutOfMemory
	}
	copy(b, s)
	return string(b), nil
}

/* -------------------------------------------------------------------------- */
/*  Errors                                                                    */
/* -------------------------------------------------------------------------- */

var ErrOutOfMemory = fmt.Errorf("arena out of memory")
