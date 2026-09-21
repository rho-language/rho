//
// @file main_test.go
// @brief Tests for Rho CLI - Enterprise v3.
//
// Validates:
// - rho build <file>
// - rho build -o <out.c>
// - rho run <file>
// - error handling
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
/*  Test: rho build                                                           */
/* -------------------------------------------------------------------------- */

func TestCLIBuildDefault(t *testing.T) {
	src := `
module cli_test

fn main() void {
}
`
	in := "cli_build_default.rho"
	out := "cli_build_default.c"

	defer os.Remove(in)
	defer os.Remove(out)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	buildCLI([]string{in})

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected %s to exist", out)
	}
}

func TestCLIBuildCustomOut(t *testing.T) {
	src := `
module cli_custom

fn main() void {
}
`
	in := "cli_custom.rho"
	out := "cli_custom_out.c"

	defer os.Remove(in)
	defer os.Remove(out)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	buildCLI([]string{"-o", out, in})

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected custom output %s to exist", out)
	}
}
