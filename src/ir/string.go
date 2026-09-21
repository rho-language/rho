//
// @file string.go
// @brief Rho Compiler - IR String type.
//
// This file defines the IR representation of the Rho String type.
// Rho strings are deterministic, immutable views over raw bytes.
// They map directly into C as:
//
//     struct {
//         const uint8_t* data;
//         uint64_t       length;
//     }
//
// No implicit allocation, no GC, no refcount.
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
/*  StringIR Type                                                             */
/* -------------------------------------------------------------------------- */

// StringIR represents a deterministic, immutable string view in IR.
// It is always UTF-8, but the IR does not validate encoding.
type StringIR struct {
    Data   string // raw Go string (compiler-side)
    Length uint64 // explicit length for C backend
}

/* -------------------------------------------------------------------------- */
/*  Constructors                                                              */
/* -------------------------------------------------------------------------- */

// NewStringIR creates a new IR string from a Go string.
// Length is computed deterministically.
func NewStringIR(s string) StringIR {
    return StringIR{
        Data:   s,
        Length: uint64(len(s)),
    }
}

// NewEmptyStringIR creates an empty IR string.
func NewEmptyStringIR() StringIR {
    return StringIR{
        Data:   "",
        Length: 0,
    }
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

// IsEmpty reports whether the string has zero length.
func (s StringIR) IsEmpty() bool {
    return s.Length == 0
}

// Equals performs deterministic string comparison.
// IR-level comparison is always byte-by-byte.
func (s StringIR) Equals(other StringIR) bool {
    if s.Length != other.Length {
        return false
    }
    return s.Data == other.Data
}
