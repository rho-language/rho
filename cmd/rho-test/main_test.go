//
// @file main_test.go
// @brief Tests for Rho Test CLI entrypoint - Enterprise v1.
//
// Notes:
//   - We only test argument parsing and basic flow.
//   - We do NOT execute real rho or gcc.
//   - We simulate invocation by calling main() indirectly.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"os"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Main: No Arguments                                                         */
/* -------------------------------------------------------------------------- */

func TestMainNoArgs(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Fatalf("run returned %d, want 2", code)
	}
}

func TestRunNoArgs(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Fatalf("run returned %d, want 2", code)
	}
}

/* -------------------------------------------------------------------------- */
/*  Main: Directory With No Tests                                              */
/* -------------------------------------------------------------------------- */

func TestMainEmptyDir(t *testing.T) {
	os.Mkdir("emptydir", 0755)
	defer os.RemoveAll("emptydir")
	if code := run([]string{"emptydir"}); code != 0 {
		t.Fatalf("run returned %d, want 0", code)
	}
}
