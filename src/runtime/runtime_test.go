//
// @file runtime_test.go
// @brief Rho Runtime tests.
//
// Tests the minimal runtime helpers (Log, Assert).
// Panic-related functions are not tested directly because they exit
// the process; instead we focus on non-fatal behavior.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package runtime

import (
    "bytes"
    "os"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Helpers for capturing stderr                                              */
/* -------------------------------------------------------------------------- */

func captureStderr(f func()) string {
    old := os.Stderr
    r, w, _ := os.Pipe()
    os.Stderr = w

    f()

    w.Close()
    os.Stderr = old

    var buf bytes.Buffer
    buf.ReadFrom(r)
    return buf.String()
}

/* -------------------------------------------------------------------------- */
/*  Tests: Log                                                                */
/* -------------------------------------------------------------------------- */

func TestLog(t *testing.T) {
    out := captureStderr(func() {
        Log("hello")
    })

    if !bytes.Contains([]byte(out), []byte("rho: hello")) {
        t.Fatalf("expected log output to contain 'rho: hello', got %q", out)
    }
}

/* -------------------------------------------------------------------------- */
/*  Tests: Logf                                                               */
/* -------------------------------------------------------------------------- */

func TestLogf(t *testing.T) {
    out := captureStderr(func() {
        Logf("value=%d", 42)
    })

    if !bytes.Contains([]byte(out), []byte("rho: value=42")) {
        t.Fatalf("expected logf output to contain 'rho: value=42', got %q", out)
    }
}

/* -------------------------------------------------------------------------- */
/*  Tests: Assert (true)                                                      */
/* -------------------------------------------------------------------------- */

func TestAssertTrue(t *testing.T) {
    // Should not panic/exit when condition is true.
    Assert(true, "should not fail")
}
