//
// @file arena.go
// @brief Rho Memory Arena Allocator.
//
// Minimal deterministic arena allocator used by the Rho compiler and runtime.
// Provides bump-pointer allocation, reset, and capacity checks.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package memory

/* -------------------------------------------------------------------------- */
/*  Arena Type                                                                */
/* -------------------------------------------------------------------------- */

type Arena struct {
	buf []byte
	off int
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// NewArena creates a new arena with the given capacity.
func NewArena(capacity int) *Arena {
	return &Arena{
		buf: make([]byte, capacity),
		off: 0,
	}
}

// Alloc reserves n bytes and returns a slice pointing into the arena.
func (a *Arena) Alloc(n int) ([]byte, bool) {
	if n < 0 || a.off > len(a.buf) || n > len(a.buf)-a.off {
		return nil, false
	}
	start := a.off
	a.off += n
	return a.buf[start:a.off], true
}

// Reset clears the arena without freeing memory.
func (a *Arena) Reset() {
	a.off = 0
}

// Used returns the number of bytes allocated.
func (a *Arena) Used() int {
	return a.off
}

// Capacity returns total arena capacity.
func (a *Arena) Capacity() int {
	return len(a.buf)
}
