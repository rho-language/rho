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
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
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

/* -------------------------------------------------------------------------- */
/*  Tests: Declarations, array types and generic calls                        */
/* -------------------------------------------------------------------------- */

func TestFormatConstArrayAndGenericCall(t *testing.T) {
	src := `module test
const MAX: u32 = 10
const NAMES: [string] = [ "a", "b" ]
const GRID: u32[2][2] = [ [ 0; 2 ]; 2 ]
struct S {
    buf: u8[4]
}
fn f(src: Ptr[u32], grid: u32[2][2]) void {
    var i: u32 = 0
    let fixed: u32[3] = [ 0; 3 ]
    let p: Ptr[u32] = Ptr[u32](&i)
}
`

	expected := `module test

const MAX: u32 = 10

const NAMES: [string] = [ "a", "b" ]

const GRID: u32[2][2] = [ [ 0; 2 ]; 2 ]

struct S {
    buf: u8[4]
}

fn f(src: Ptr[u32], grid: u32[2][2]) void {
    var i: u32 = 0
    let fixed: u32[3] = [ 0; 3 ]
    let p: Ptr[u32] = Ptr[u32](&i)
}
`

	out, err := FormatSource(src)
	if err != nil {
		t.Fatalf("FormatSource failed: %v", err)
	}
	if out != expected {
		t.Fatalf("unexpected formatting:\n--- got ---\n%s\n--- want ---\n%s", out, expected)
	}

	// Formatting the result must not change it again, and it must still parse.
	again, err := FormatSource(out)
	if err != nil {
		t.Fatalf("re-formatting failed: %v", err)
	}
	if again != out {
		t.Fatalf("formatting is not stable:\n--- once ---\n%s\n--- twice ---\n%s", out, again)
	}
}

func TestTopLevelLetAndVarBecomeConst(t *testing.T) {
	src := `module test
let A: u32 = 1
var B: u32 = 2
`
	expected := `module test

const A: u32 = 1

const B: u32 = 2
`
	out, err := FormatSource(src)
	if err != nil {
		t.Fatalf("FormatSource failed: %v", err)
	}
	if out != expected {
		t.Fatalf("unexpected formatting:\n--- got ---\n%s\n--- want ---\n%s", out, expected)
	}
}

func TestFormatContinueForAndMatch(t *testing.T) {
	src := `module test
fn f(values: u32[4]) void {
    for value in values step 2 {
        if value == 0 { continue }
        if value == 1 { break }
    }
    match values[0] {
        1 => { print("one") }
        _ => { print("many") }
    }
}
`
	expected := `module test

fn f(values: u32[4]) void {
    for value in values step 2 {
        if value == 0 {
            continue
        }
        if value == 1 {
            break
        }
    }
    match values[0] {
        1 => {
            print("one")
        }
        _ => {
            print("many")
        }
    }
}
`
	out, err := FormatSource(src)
	if err != nil {
		t.Fatalf("FormatSource failed: %v", err)
	}
	if out != expected {
		t.Fatalf("unexpected formatting:\n--- got ---\n%s\n--- want ---\n%s", out, expected)
	}
}
