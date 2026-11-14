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
	{
		name: "a nested negation is two negations, not a decrement",
		source: `module app

fn main() void {
    let b: i32 = 5
    let c: i32 = -(-b)
    print(int_to_str(u32(c)))
    print(int_to_str(u32(b)))
}
`,
	},
	{
		name: "a widened operand keeps its sign",
		source: `module app

fn main() void {
    let a: i64 = 3
    let b: i32 = 5
    if a > -b {
        print("greater")
    } else {
        print("not greater")
    }
    let c: i64 = i64(-b)
    if c == -5 {
        print("cast whole")
    }
}
`,
	},
	{
		name: "an if expression evaluates only the branch taken",
		source: `module app

fn main() void {
    let n: i64 = 0
    let t: i64 = 10
    let r: i64 = if n > 0 { t / n } else { 7 }
    print(int_to_str(u32(r)))
    let a: i64 = -3
    let b: i32 = 4
    let m: i64 = if a > 0 { a } else { -b }
    if m == -4 {
        print("arms of two widths agree")
    }
    let s: string = if n > 0 { "positive" } else { "zero" }
    print(s)
}
`,
	},
	{
		name: "a bitwise not is the same on both backends",
		source: `module app

fn main() void {
    let a: u32 = 5
    let b: u32 = 3
    print(int_to_str(a & ~b))
}
`,
	},
	{
		name: "the text an application reads from a form and writes to a page",
		source: `module app

fn show(label: string, value: string) void {
    print(label + "=[" + value + "]")
}

fn milli(text: string) void {
    var ok: string = "no"
    if milli_valid(text) {
        ok = "yes"
    }
    show("milli " + text, ok + " " + milli_format(milli_parse(text)))
}

fn main() void {
    show("trim", str_trim("  \t a b \r\n"))
    show("trim-empty", str_trim(" \n "))
    show("encode", str_url_encode("a b&c=d/é~x"))
    show("decode", str_url_decode("a+b%26c%3Dd%2F%C3%A9%7Ex%2%zz"))
    let body: string = "name=Tank+1&temp=24%2C5&empty=&flag&name=second"
    show("field-name", form_field(body, "name"))
    show("field-temp", form_field(body, "temp"))
    show("field-empty", form_field(body, "empty"))
    show("field-flag", form_field(body, "flag"))
    show("field-none", form_field(body, "nope"))
    show("field-encoded-key", form_field("a%20b=1", "a b"))
    milli("24.5")
    milli("24,5")
    milli("-0.25")
    milli("+3")
    milli(" 7 ")
    milli("1.23456")
    milli(".5")
    milli("5.")
    milli("")
    milli("-")
    milli("1.2.3")
    milli("abc")
    milli("1234567890123456")
    show("format", milli_format(-1500) + " " + milli_format(20000) + " " + milli_format(1) + " " + milli_format(0) + " " + milli_format(-10))
    show("round", milli_format(milli_round(2345, 2)) + " " + milli_format(milli_round(-2345, 2)) + " " + milli_format(milli_round(2500, 0)) + " " + milli_format(milli_round(2449, 1)) + " " + milli_format(milli_round(2345, 3)))
}
`,
		want: "trim=[a b]trim-empty=[]encode=[a%20b%26c%3Dd%2F%C3%A9~x]decode=[a b&c=d/é~x%2%zz]field-name=[Tank 1]field-temp=[24,5]field-empty=[]field-flag=[]field-none=[]field-encoded-key=[1]milli 24.5=[yes 24.5]milli 24,5=[yes 24.5]milli -0.25=[yes -0.25]milli +3=[yes 3]milli  7 =[yes 7]milli 1.23456=[yes 1.234]milli .5=[yes 0.5]milli 5.=[yes 5]milli =[no 0]milli -=[no 0]milli 1.2.3=[no 0]milli abc=[no 0]milli 1234567890123456=[no 0]format=[-1.5 20 0.001 0 -0.01]round=[2.35 -2.35 3 2.4 2.345]",
	},
	{
		name: "numbers of any width, case, fields and flat JSON",
		source: `module app

fn show(label: string, value: string) void {
    print(label + "=[" + value + "]")
}

fn main() void {
    let big: i64 = 9000000000
    show("i64", i64_to_str(big) + " " + i64_to_str(0 - big) + " " + i64_to_str(0))
    show("from", i64_to_str(i64_from_str(" -42 ")) + " " + i64_to_str(i64_from_str("+7")) + " " + i64_to_str(i64_from_str("4x")) + " " + i64_to_str(i64_from_str("")))
    show("case", str_lower("Tank A-1") + " " + str_upper("Tank a-1"))
    show("replace", str_replace("{n} of {n}", "{n}", "3") + " " + str_replace("aaa", "aa", "b") + " " + str_replace("keep", "", "x") + " " + str_replace("a-b", "-", ""))
    show("field", str_field("a|b||d", "|", 1) + "," + str_field("a|b||d", "|", 2) + "," + str_field("a|b||d", "|", 3) + "," + str_field("a|b", "|", 5) + "," + str_field("one::two", "::", 1))
    let doc: string = "{\"name\": \"Tank \\\"A\\\"\", \"total\":{\"power\": -12.5e1, \"n\":3}, \"list\":[1]}"
    show("json", json_str_field(doc, "name") + "|" + json_num_field(doc, "power") + "|" + json_num_field(doc, "n") + "|" + json_str_field(doc, "missing") + "|" + json_num_field(doc, "name"))
}
`,
		want: "i64=[9000000000 -9000000000 0]from=[-42 7 0 0]case=[tank a-1 TANK A-1]replace=[3 of 3 ba keep ab]field=[b,,d,,two]json=[Tank \"A\"|-12.5e1|3||]",
	},
	{
		name: "the floating-point functions through std.math",
		source: `module app

import std.math.float
import std.math.fast
import std.math.linear

fn milli(x: f64) string {
    return int_to_str(u32(x * 1000.0 + 0.5))
}

fn main() void {
    print("float=[" + milli(sqrt(16.0)) + " " + milli(pow(2.0, 10.0)) + " " + milli(sin(0.5)) + " " + milli(cos(0.5)) + " " + milli(atan2(1.0, 1.0)) + " " + milli(abs_f64(-1.5)) + " " + milli(wrap_pi(7.0)) + " " + milli(lerp(0.0, 10.0, 0.25)) + "]")
    print("fast=[" + milli(fast_sqrt(81.0)) + " " + milli(fast_sin(1.0)) + " " + milli(fast_cos(1.0)) + " " + milli(fast_atan2(1.0, 2.0)) + "]")
    print("linear=[" + milli(vec2_length(Vec2{ 3.0, 4.0 })) + "]")
}
`,
		want: "float=[4000 1024000 479 878 785 1500 717 2500]fast=[9000 841 540 464]linear=[5000]",
	},
	{
		name: "bare floating-point builtins with nothing imported",
		source: `module app

fn milli(x: f64) string {
    return int_to_str(u32(x * 1000.0 + 0.5))
}

fn main() void {
    print(milli(sqrt(2.0)) + " " + milli(floor(2.7)) + " " + milli(sin(0.5)) + " " + milli(cos(0.5)) + " " + milli(atan2(1.0, 1.0)))
    print(" " + milli(clamp_f64(-1.0, 0.0, 5.0)) + " " + milli(lerp(2.0, 4.0, 0.5)))
}
`,
		want: "1414 2000 479 878 785 0 3000",
	},
	{
		name: "std.os: the clock, the environment and exit",
		source: `module app

import std.os.sys
import std.os.time

fn main() void {
    setenv("RHO_PARITY_VAR", "one")
    setenv("RHO_PARITY_VAR", "two")
    print("env=[" + getenv("RHO_PARITY_VAR") + "]")
    let t: TimeSpec = now()
    if t.sec > 1700000000 {
        print(" clock")
    }
    if t.nsec < 1000000000 {
        print(" nsec")
    }
    exit(0)
    print(" after")
}
`,
		want: "env=[two] clock nsec",
	},
}
