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
	"strings"
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

// An enum, a pattern and `if let` survive formatting as they were written:
// every variant form, a generic enum's defaults, and a variant named with its
// type arguments.
func TestFormatKeepsEnumsAndPatterns(t *testing.T) {
	source := `module app

enum Level { Low = -2, Mid, High = 10 }

enum Shape[T = u32] {
    Circle(T)
    Rect { w: T, h: T }
    Empty
}

fn area(s: Shape) u32 {
    match s {
        Shape.Circle(r) => { return r }
        Shape.Rect { w, h } => { return w * h }
        _ => { return 0 }
    }
}

fn main() void {
    let none = Option[u32].None
    if let Some(x) = none {
        print("some")
    } else {
        print("none")
    }
}
`
	once, err := FormatSource(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"enum Level {\n    Low = -2\n    Mid\n    High = 10\n}", "enum Shape[T = u32] {", "    Circle(T)\n", "    Rect { w: T, h: T }\n", "    Empty\n",
		"Shape.Rect{ w, h } => {", "Option[u32].None", "if let Some(x) = none {", "} else {"} {
		if !strings.Contains(once, want) {
			t.Errorf("the formatted source has no %q:\n%s", want, once)
		}
	}
	twice, err := FormatSource(once)
	if err != nil || twice != once {
		t.Errorf("formatting is not stable (%v):\n%s\n---\n%s", err, once, twice)
	}
}

// A cut is printed as it was written, each bound present or left out, so
// formatting a program that slices does not change what it slices.
func TestFormatKeepsSubslices(t *testing.T) {
	source := `module app

fn main() void {
    var buf: u32[8]
    var s = buf[..0]
    s = append(s, 1)
    let a = buf[1..n + 1]
    let b = s[2..]
    let c = s[..]
    let d = Slice.from_raw(&buf[0], 4)
    print(int_to_str(a[0] + b[0] + c[0] + d[0]))
}
`
	once, err := FormatSource(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"buf[..0]", "buf[1..n + 1]", "s[2..]", "s[..]", "Slice.from_raw(&buf[0], 4)", "s = append(s, 1)"} {
		if !strings.Contains(once, want) {
			t.Errorf("the formatted source has no %q:\n%s", want, once)
		}
	}
	twice, err := FormatSource(once)
	if err != nil || twice != once {
		t.Errorf("formatting is not stable (%v):\n%s\n---\n%s", err, once, twice)
	}
}

// A function type and a closure are printed as they were written: the type in
// a parameter, a field, a local and a result, and a literal's body indented
// from the statement it is in, nested ones included.
func TestFormatKeepsFunctionTypesAndClosures(t *testing.T) {
	source := `module app

struct Handler {
    run: fn(i32) -> i32
    done: fn()
}

fn apply(f: fn(i32, string) -> bool, v: i32) -> bool {
    return f(v, "x")
}

fn make(k: i32) -> fn(i32) -> i32 {
    return fn(x: i32) -> i32 { return x + k }
}

fn main() void {
    var table: (fn(i32) -> i32)[2]
    let add = fn(x: i32) -> i32 {
        let inner = fn(y: i32) -> i32 { return y * x }
        return inner(x)
    }
    let say = fn(m: string) { print(m) }
    if apply(fn(a: i32, b: string) -> bool { return a > 0 }, 3) {
        say("yes")
    }
}
`
	once, err := FormatSource(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"run: fn(i32) -> i32", "done: fn()", "f: fn(i32, string) -> bool", "fn make(k: i32) fn(i32) -> i32 {",
		"var table: (fn(i32) -> i32)[2]", "let add = fn(x: i32) -> i32 {\n        let inner = fn(y: i32) -> i32 {\n            return y * x\n        }",
		"let say = fn(m: string) {\n        print(m)\n    }",
		"if apply(fn(a: i32, b: string) -> bool {",
	} {
		if !strings.Contains(once, want) {
			t.Errorf("the formatted source has no %q:\n%s", want, once)
		}
	}
	twice, err := FormatSource(once)
	if err != nil || twice != once {
		t.Errorf("formatting is not stable (%v):\n%s\n---\n%s", err, once, twice)
	}
}
