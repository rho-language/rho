//
// @file parity_cases_test.go
// @brief The programs the two backends are compared on.
//
// Each exercises one thing whose meaning a backend could get wrong: integer width,
// signedness, overflow, shifts, division, string handling, loops and arrays. Each
// prints what it computed, so a disagreement shows as two different lines rather
// than as a pass.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rho

var parityCases = []parityCase{
	{
		name: "unsigned arithmetic stays in its width",
		source: `module app

fn main() void {
    let a: u8 = 200
    let b: u8 = 100
    let sum: u8 = a + b
    print(int_to_str(u32(sum)))
    let c: u32 = 4000000000
    let d: u32 = c + c
    print(int_to_str(d))
}
`,
	},
	{
		name: "signed values and division truncate toward zero",
		source: `module app

fn main() void {
    let a: i32 = -7
    let b: i32 = 2
    print(int_to_str(u32(a / b)))
    print(int_to_str(u32(a % b)))
}
`,
	},
	{
		name: "shifts within the width",
		source: `module app

fn main() void {
    let v: u32 = 1
    print(int_to_str(v << 31))
    print(int_to_str((v << 31) >> 31))
    let s: i32 = -8
    print(int_to_str(u32(s >> 1)))
}
`,
	},
	{
		name: "narrowing casts",
		source: `module app

fn main() void {
    let wide: u32 = 300
    let byte: u8 = wide as u8
    print(int_to_str(u32(byte)))
    let neg: i32 = -1
    print(int_to_str(neg as u32))
}
`,
	},
	{
		name: "loops and an array",
		source: `module app

fn main() void {
    let values: u32[4] = [1, 2, 3, 4]
    var sum: u32 = 0
    for value in values {
        sum = sum + value
    }
    print(int_to_str(sum))
    var i: u32 = 0
    while i < 3 {
        i = i + 1
    }
    print(int_to_str(i))
    for value in values step 2 {
        print(int_to_str(value))
    }
}
`,
	},
	{
		name: "continue in a for loop still advances",
		source: `module app

fn main() void {
    let values: u32[5] = [1, 2, 3, 4, 5]
    var sum: u32 = 0
    for value in values {
        if value == 3 {
            continue
        }
        sum = sum + value
    }
    print(int_to_str(sum))
}
`,
	},
	{
		name: "strings",
		source: `module app

fn main() void {
    let a: string = "sensor "
    let b: string = a + "one"
    print(b)
    print("tab\there")
    print(int_to_str(u32(strlen(b))))
}
`,
	},
	{
		name: "a constant conversion that does not fit is a Go build error",
		// C wraps the constant at run time; Go refuses it at compile time. Neither
		// is wrong in isolation, but one program that builds on one backend and
		// not on the other is two languages. The fix is for the type checker to
		// refuse it on both.
		diverges: "Go rejects `u32(7 / -2)` at compile time (constant -3 overflows uint32); C accepts it",
		source: `module app

fn main() void {
    print(int_to_str(u32(7 / -2)))
}
`,
	},
	{
		name:     "a shift by the width of the type",
		diverges: "C is undefined behaviour (UBSan: shift exponent 32 is too large for 32-bit type); Go yields 0",
		source: `module app

fn main() void {
    var s: u32 = 32
    let v: u32 = 1
    print(int_to_str(v << s))
}
`,
	},
	{
		name:     "signed overflow",
		diverges: "C is undefined behaviour (UBSan: signed integer overflow); Go wraps two's-complement",
		source: `module app

fn main() void {
    var a: i32 = 2147483647
    a = a + 1
    print(int_to_str(u32(a)))
}
`,
	},
	{
		name:     "an index past the end of a fixed array",
		diverges: "Go panics with a bounds error; the generated C emits no bounds check, and only UBSan notices — without it the C program reads whatever follows the array",
		source: `module app

fn main() void {
    let v: u32[4] = [1, 2, 3, 4]
    var i: u32 = 4
    print(int_to_str(v[i]))
}
`,
	},
	{
		name:     "division by zero",
		diverges: "Go panics; C is undefined behaviour (a SIGFPE on x86, and nothing guaranteed elsewhere — an embedded target may return garbage)",
		source: `module app

fn main() void {
    var z: u32 = 0
    print(int_to_str(10 / z))
}
`,
	},
}
