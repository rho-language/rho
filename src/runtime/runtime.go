//
// @file runtime.go
// @brief Rho Runtime - Minimal support layer.
//
// Minimal deterministic runtime for Rho 0.1.
// Provides:
//   - panic/abort helpers
//   - assert
//   - logging hooks
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package runtime

import (
    "fmt"
    "os"
)

/* -------------------------------------------------------------------------- */
/*  Panic / Abort                                                             */
/* -------------------------------------------------------------------------- */

// Panic aborts the program with a message.
// In C backend this maps to abort() or exit(1).
func Panic(msg string) {
    fmt.Fprintln(os.Stderr, "rho panic:", msg)
    os.Exit(1)
}

// Panicf is a formatted variant of Panic.
func Panicf(format string, args ...any) {
    Panic(fmt.Sprintf(format, args...))
}

/* -------------------------------------------------------------------------- */
/*  Assert                                                                    */
/* -------------------------------------------------------------------------- */

// Assert fails with Panic if cond is false.
func Assert(cond bool, msg string) {
    if !cond {
        Panic("assert failed: " + msg)
    }
}

/* -------------------------------------------------------------------------- */
/*  Logging                                                                   */
/* -------------------------------------------------------------------------- */

// Log writes a message to stderr with a rho prefix.
func Log(msg string) {
    fmt.Fprintln(os.Stderr, "rho:", msg)
}

// Logf writes a formatted message to stderr.
func Logf(format string, args ...any) {
    fmt.Fprintf(os.Stderr, "rho: "+format+"\n", args...)
}
