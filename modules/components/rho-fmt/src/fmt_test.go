//
// @file fmt_test.go
// @brief Rho Formatter tests.
//
// Tests deterministic formatting of Rho source files.
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
/*  Tests: Basic Formatting                                                    */
/* -------------------------------------------------------------------------- */

func TestFormatFile(t *testing.T) {
	src := `
module  test

struct  A   {
x: u8
}

fn   f (a: u8)   void {
}
`

	expected := `module test

struct A {
    x: u8
}

fn f(a: u8) void {
}
`

	in := "fmt_test.rho"
	defer os.Remove(in)

	if err := os.WriteFile(in, []byte(src), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	out, err := FormatFile(in)
	if err != nil {
		t.Fatalf("FormatFile failed: %v", err)
	}

	if out != expected {
		t.Fatalf("unexpected formatting:\n--- got ---\n%s\n--- want ---\n%s", out, expected)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: CLI -w                                                             */
/* -------------------------------------------------------------------------- */

func TestCLIWrite(t *testing.T) {
	src := `
module  x
fn  main() void {}
`

	expected := `module x

fn main() void {
}
`

	in := "fmt_cli.rho"
	defer os.Remove(in)

	os.WriteFile(in, []byte(src), 0644)

	// simulate: rho fmt -w fmt_cli.rho
	os.Args = []string{"rho-fmt", "-w", in}
	main()

	out, _ := os.ReadFile(in)
	if string(out) != expected {
		t.Fatalf("CLI -w did not write expected output")
	}
}
