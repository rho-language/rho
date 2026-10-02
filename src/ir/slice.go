//
// @file slice.go
// @brief Rho Compiler - IR Slice type.
//
// This file defines the IR representation of the Rho Slice type.
// Rho slices are deterministic, immutable views over contiguous memory.
// They map directly into C as:
//
//     struct {
//         const uint8_t* data;
//         uint64_t       length;
//         uint64_t       capacity;
//     }
//
// No implicit allocation, no GC, no resizing. All behavior is explicit.
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
/*  SliceIR Type                                                              */
/* -------------------------------------------------------------------------- */

// SliceIR represents a deterministic, immutable slice view in IR.
// It is a low-level memory view used by the Rho runtime and backend C.
type SliceIR struct {
	Data     string // raw Go string used as byte storage (compiler-side)
	Length   uint64 // number of elements
	Capacity uint64 // total available elements
}

/* -------------------------------------------------------------------------- */
/*  Constructors                                                              */
/* -------------------------------------------------------------------------- */

// NewSliceIR creates a new IR slice from a Go string.
// Capacity is equal to Length for immutable slices.
func NewSliceIR(data string) SliceIR {
	return SliceIR{
		Data:     data,
		Length:   uint64(len(data)),
		Capacity: uint64(len(data)),
	}
}

// NewEmptySliceIR creates an empty IR slice.
func NewEmptySliceIR() SliceIR {
	return SliceIR{
		Data:     "",
		Length:   0,
		Capacity: 0,
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

// IsEmpty reports whether the slice has zero length.
func (s SliceIR) IsEmpty() bool {
	return s.Length == 0
}

// Equals performs deterministic slice comparison.
// IR-level comparison is always byte-by-byte.
func (s SliceIR) Equals(other SliceIR) bool {
	if s.Length != other.Length {
		return false
	}
	return s.Data == other.Data
}

// HasCapacity reports whether the slice has at least n elements of capacity.
func (s SliceIR) HasCapacity(n uint64) bool {
	return s.Capacity >= n
}
