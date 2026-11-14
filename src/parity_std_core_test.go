//
// @file parity_std_core_test.go
// @brief std.core, std.assert and std.strings.utf8 on both backends.
//
// The pure modules of the library's core: they are plain Rho over values, so they
// must print the same on C and Go. A failed assertion and an unwrap of an Err
// stop the program on both, after printing the same thing.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

func init() {
	parityCases = append(parityCases, stdCoreParityCases...)
}

var stdCoreParityCases = []parityCase{
	{
		name: "std.core.result: Ok, Err and every accessor",
		source: `module app

import std.core.result

fn half(n: u32) Result[u32, string] {
    if n % 2 != 0 {
        return Err[u32, string]("odd")
    }
    return Ok[u32, string](n / 2)
}

fn main() void {
    let a: Result[u32, string] = half(10)
    let b: Result[u32, string] = half(7)
    if is_ok(a) && !is_err(a) {
        print("a=" + int_to_str(value(a)) + " " + int_to_str(unwrap(a)))
    }
    if is_err(b) && !is_ok(b) {
        // An Err holds the default of the value type beside its error.
        print(" b=" + error(b) + " v=" + int_to_str(value(b)))
    }
    let c: Result[i64, u32] = Err[i64, u32](404)
    print(" c=" + int_to_str(error(c)) + " " + i64_to_str(value(c)))
    let d: Result[i64, u32] = Ok[i64, u32](-5)
    print(" d=" + i64_to_str(unwrap(d)) + " " + int_to_str(error(d)))
}
`,
		want: "a=5 5 b=odd v=0 c=404 0 d=-5 0",
	},
	// The panic message goes to standard error on both backends, so standard output
	// is only what the program printed before it.
	{
		name: "std.core.result: unwrap of an Err stops the program on both backends",
		source: `module app

import std.core.result

fn main() void {
    let e: Result[u32, u32] = Err[u32, u32](7)
    print("err=" + int_to_str(error(e)) + " ")
    let v: u32 = unwrap(e)
    print("after " + int_to_str(v))
}
`,
		wantFailure: true,
		want:        "err=7 ",
	},
	{
		name: "std.core.range: walking a range with next",
		source: `module app

import std.core.option
import std.core.range

fn main() void {
    let r: Range = range(2, 5)
    var i: u32 = r.start
    var n: Option[u32] = next(r, i)
    while n.is_some {
        print(int_to_str(n.value) + " ")
        i = i + 1
        n = next(r, i)
    }
    print("end=" + int_to_str(r.end))
    print(if next(r, 1).is_some { " 1in" } else { " 1out" })
    print(if next(range(3, 3), 3).is_some { " empty-in" } else { " empty-out" })
}
`,
		want: "2 3 4 end=5 1out empty-out",
	},
	// std.core.builtin used to redeclare the primitive types, so importing it made
	// `int_to_str(u32(n))` a type error ("has type u32, want u32").
	{
		name: "std.core.builtin: importing it changes nothing",
		source: `module app

import std.core.builtin

fn main() void {
    let n: u32 = u32(strlen("four"))
    print("len=" + int_to_str(n))
    if str_eq("a", "a") { print(" eq") }
    let x: u8 = 250
    let y: i16 = -300
    let z: f32 = 1.5
    let w: u16 = 65535
    let q: i8 = -128
    print(" " + int_to_str(u32(x)) + " " + i64_to_str(i64(y)) + " " + int_to_str(u32(f64(z) * 2.0)) + " " + int_to_str(u32(w)) + " " + i64_to_str(i64(q)))
}
`,
		want: "len=4 eq 250 -300 3 65535 -128",
	},
	{
		name: "std.assert.assert: assertions that hold",
		source: `module app

import std.assert.assert

fn main() void {
    assert(1 + 1 == 2)
    assert_true(3 > 2)
    assert_false(2 > 3)
    assert_eq[u32](3, 3)
    assert_ne[u32](3, 4)
    assert_eq[string]("a", "a")
    assert_ne[string]("a", "b")
    assert_eq[f64](1.5, 1.5)
    assert_ne[i32](-1, 1)
    assert_eq[bool](true, true)
    print("passed")
}
`,
		want: "passed",
	},
	{
		name: "std.assert.assert: a failed assert_eq prints and aborts",
		source: `module app

import std.assert.assert

fn main() void {
    assert_eq[u32](3, 3)
    print("before ")
    assert_eq[u32](3, 4)
    print("after")
}
`,
		wantFailure: true,
		want:        "before ASSERTION FAILED: assert_eq failed",
	},
	{
		name: "std.assert.assert: fail prints its message and aborts",
		source: `module app

import std.assert.assert

fn main() void {
    assert_true(true)
    print("ok ")
    fail("stop here")
    print("after")
}
`,
		wantFailure: true,
		want:        "ok ASSERTION FAILED: stop here",
	},
	{
		name: "std.strings.utf8: is_ascii and first_codepoint",
		source: `module app

import std.core.option
import std.strings.string
import std.strings.utf8

fn main() void {
    // "hi" and "é" (0xC3 0xA9)
    var bytes: u8[4] = [104, 105, 195, 169]
    let all: String = String{ Ptr[u8](&bytes[0]), 4 }
    let head: String = String{ Ptr[u8](&bytes[0]), 2 }
    let tail: String = String{ Ptr[u8](&bytes[2]), 2 }
    let empty: String = String{ Ptr[u8](&bytes[0]), 0 }
    if is_ascii(head) { print("head ascii ") }
    if !is_ascii(all) { print("all not ") }
    if !is_ascii(tail) { print("tail not ") }
    if is_ascii(empty) { print("empty ascii ") }
    let c: Option[u32] = first_codepoint(all)
    print(int_to_str(c.value))
    if !first_codepoint(tail).is_some { print(" tail none") }
    if !first_codepoint(empty).is_some { print(" empty none") }
}
`,
		want: "head ascii all not tail not empty ascii 104 tail none empty none",
	},
	// std.core.cmp: min, max and clamp over Ordered, each specialised for the
	// type it is called with.
	{
		name: "std.core.cmp: min, max and clamp on integers of every width, floats and strings",
		source: `module app

import std.core.cmp

fn main() void {
    print(int_to_str(max(3, 9) as u32) + " " + i64_to_str(min(-3, 9) as i64) + " ")
    let lo: u8 = 10
    let hi: u8 = 200
    print(int_to_str(clamp(250, lo, hi) as u32) + " " + int_to_str(clamp(5, lo, hi) as u32) + " " + int_to_str(clamp(77, lo, hi) as u32) + " ")
    let big: u64 = 18446744073709551615
    let small: u64 = 1
    print(i64_to_str((max(big, small) - 1) as i64 - 9223372036854775807) + " ")
    let a: i16 = -32768
    let b: i16 = 32767
    print(i64_to_str(min(a, b) as i64) + " ")
    let f: f64 = clamp(1.5, 0.0, 1.0)
    let g: f32 = min(2.5, 0.25)
    print(i64_to_str((f * 10.0) as i64) + " " + i64_to_str((g * 100.0) as i64) + " ")
    print(max("pear", "apple") + " " + min("pear", "apple") + " " + clamp("m", "a", "k") + " ")
    let x: u32 = 4
    let y: u32 = 4
    print(int_to_str(max(x, y)) + " " + int_to_str(min(x, y)))
}
`,
		want: "9 -3 200 10 77 9223372036854775807 -32768 10 25 pear apple k 4 4",
	},
}
