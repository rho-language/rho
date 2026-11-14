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
	// The string arena. The C side releases strings at the end of each loop
	// iteration and after each statement, and is built with AddressSanitizer,
	// which poisons what is released: a string the compiler gave back too early
	// is a report here rather than the same output by luck.
	{
		name: "strings built in loops are released, and the ones kept survive",
		source: `module app

struct Reading {
    name: string
    value: u32
}

fn label(i: u32) string {
    let unit = "C"
    return "T" + int_to_str(i) + unit
}

fn joined(count: u32) string {
    var out: string = ""
    var k: u32 = 0
    while k < count {
        out = out + label(k + 8) + ","
        k = k + 1
    }
    return out
}

fn first_long(limit: u32) string {
    var i: u32 = 0
    while i < limit {
        let candidate = label(i * 7)
        if strlen(candidate) > 3 {
            return candidate
        }
        i = i + 1
    }
    return "none"
}

fn main() void {
    var i: u32 = 0
    var state: string = "idle"
    while i < 6 {
        let line = label(i) + "/" + state
        if str_contains(line + "!", "T3") {
            state = "warm"
            i = i + 1
            continue
        }
        var j: u32 = 0
        var row: string = ""
        while j < 3 {
            row = row + int_to_str(j)
            if j == 1 {
                let note = "[" + row + "]"
                print(note)
            }
            j = j + 1
        }
        match str_slice(line, 0, 2) {
            "T0" => print("zero ")
            "T5" => {
                print("five ")
                break
            }
            _ => print(str_trim_prefix(line, "T") + " ")
        }
        print(line + ":" + row + " ")
        i = i + 1
    }
    let values: u32[3] = [1, 2, 3]
    for value in values {
        let item = "v" + int_to_str(value)
        if value == 2 {
            continue
        }
        print(item + " ")
    }
    print(joined(2) + " ")
    print(first_long(5) + " ")
    let r = Reading{ label(4), 4 }
    print(r.name + " " + state + "\n")
}
`,
		want: "[01]zero T0C/idle:012 [01]1C/idle T1C/idle:012 [01]2C/idle T2C/idle:012 [01]4C/warm T4C/warm:012 [01]five v1 v3 T8C,T9C, T14C T4C warm\n",
	},
	{
		name: "a loop that keeps a string outside it still prints what it kept",
		source: `module app

fn main() void {
    var i: u32 = 0
    var log: string = ""
    var last: string = "none"
    while i < 4 {
        last = "n" + int_to_str(i)
        log = log + last + ";"
        i = i + 1
    }
    var k: u32 = 0
    while str_contains(log + "", int_to_str(k)) {
        k = k + 1
    }
    print(log + " " + last + " " + int_to_str(k) + "\n")
}
`,
		want: "n0;n1;n2;n3; n3 4\n",
	},
	{
		name: "numbers printed without building a string",
		source: `module app

fn main() void {
    let a: u32 = 4000000000
    let b: i64 = -9000000000
    print(int_to_str(a))
    print(" ")
    print(i64_to_str(b))
    print(" ")
    print(int_to_str(u32(7)))
    print("\n")
}
`,
		want: "4000000000 -9000000000 7\n",
	},
	{
		name: "an unsigned value plus a constant expression over a module constant",
		source: `module app

const SECURE_STORE_EVERY: u32 = 16

fn main() void {
    let up: u32 = 4000000000
    let next: u32 = up + 2 * SECURE_STORE_EVERY
    let back: u32 = SECURE_STORE_EVERY * 3 - 1 + up
    let small: u8 = 250
    let wrapped: u8 = small + 2 * 3
    print(int_to_str(next) + " " + int_to_str(back) + " " + int_to_str(u32(wrapped)) + "\n")
}
`,
		want: "4000000032 4000000047 0\n",
	},
	// Ptr[T] is one type, spelled `T*` in C and `unsafe.Pointer` in Go. Each
	// operation on one is written so both backends read it: a conversion from a
	// number and back, the null pointer, an element read and written through the
	// pointer, an offset, and the slice_empty a Slice starts from.
	{
		name: "pointers: casts, to_u64, index and slice_empty",
		source: `module app

import std.core.ptr
import std.core.slice

// deep grows the stack, which Go does by moving it: an address held only as a
// number would then name where a variable used to be.
fn deep(n: u32) u32 {
    var pad: u32[32] = [0; 32]
    pad[31] = n
    if n == 0 {
        return pad[31]
    }
    return deep(n - 1) + pad[31] - n + 1
}

fn main() void {
    let values: u32[4] = [10, 20, 30, 40]
    let p: Ptr[u32] = Ptr[u32](&values[0])
    print(int_to_str(index[u32](p, 2)))
    let z: Ptr[u8] = Ptr[u8](0)
    print(" " + int_to_str(u32(to_u64[u8](z))))
    let address: u64 = to_u64[u32](p)
    print(" " + int_to_str(deep(2000)))
    let q: Ptr[u32] = from_u64[u32](address)
    print(" " + int_to_str(index[u32](q, 3)))
    if to_u64[u32](q) == to_u64[u32](p) {
        print(" same")
    }
    let e: Slice[u32] = slice_empty[u32]()
    print(" " + int_to_str(e.len))
    if to_u64[u32](e.ptr) == 0 {
        print(" empty")
    }
}
`,
		want: "30 0 2000 40 same 0 empty",
	},
	{
		name: "pointers: stores through a pointer, a struct pointer, offsets",
		source: `module app

import std.core.ptr
import std.core.slice
import std.core.maybe_uninit

struct Point {
    x: u32
    y: u32
}

fn bump(p: Ptr[Point]) void {
    p[0].x = p[0].x + 1
    p[0].y = 7
}

fn main() void {
    var storage: u32[4] = [0, 0, 0, 0]
    let a: Ptr[u32] = Ptr[u32](&storage[0])
    a[2] = 42
    print(int_to_str(index(a, 2)) + " " + int_to_str(storage[2]))
    var pt: Point = Point{ 1, 2 }
    bump(&pt)
    print(" " + int_to_str(pt.x) + "," + int_to_str(pt.y))
    var bytes: u8[4] = [97, 98, 99, 100]
    let s: Slice[u8] = Slice[u8]{ Ptr[u8](&bytes[0]), 4 }
    let tail: Ptr[u8] = s.ptr + 2
    let back: Ptr[u8] = tail - 1
    print(" " + int_to_str(u32(tail[0])) + "," + int_to_str(u32(back[0])) + "," + int_to_str(u32(*(s.ptr + 3))))
    let v: u32 = 9
    let m: MaybeUninit[u32] = MaybeUninit[u32]{ Ptr[u32](&v) }
    print(" " + int_to_str(assume_init(m)))
    let p: Ptr[u32] = &storage[1]
    p[0] = 5
    print(" " + int_to_str(*p) + " " + int_to_str(storage[1]))
}
`,
		want: "42 42 2,7 99,98,100 9 5 5",
	},
	// The descriptor calls of std.os.sys and std.os.fs move bytes through a
	// Ptr[u8], which on both backends is the program's own memory: the bytes
	// written are an array's, and the bytes read land in another. The program
	// runs in a directory of its own, so the file it makes is its own too.
	{
		name: "std.os: descriptor open, write, close and read through Ptr[u8]",
		source: `module app

import std.os.sys
import std.os.fs

fn main() void {
    // "rho-parity-fd.txt" and the byte that ends a C string
    let path: u8[18] = [114, 104, 111, 45, 112, 97, 114, 105, 116, 121, 45, 102, 100, 46, 116, 120, 116, 0]
    let text: u8[5] = [104, 101, 108, 108, 111]
    // O_WRONLY | O_CREAT | O_TRUNC
    let opened: Result[u32, u32] = open(&path[0], 577)
    if !opened.is_ok {
        print("open failed")
        return
    }
    let wrote: u32 = write(opened.ok, &text[0], 5)
    close(opened.ok)
    var back: u8[8] = [0, 0, 0, 0, 0, 0, 0, 0]
    let reopened: Result[u32, u32] = open(&path[0], 0)
    let got: u32 = read(reopened.ok, &back[0], 8)
    close(reopened.ok)
    print("wrote=" + int_to_str(wrote) + " read=" + int_to_str(got) + " bytes=")
    var i: u32 = 0
    while i < got {
        print(int_to_str(u32(back[i])) + ",")
        i = i + 1
    }
    print(" bad=" + int_to_str(read(9999, &back[0], 8)))
}
`,
		want: "wrote=5 read=5 bytes=104,101,108,108,111, bad=4294967295",
	},
	{
		name: "std.os: a string path and a write to standard output",
		source: `module app

import std.os.sys
import std.os.fs

fn main() void {
    let fd: i32 = rho_fd_open("/dev/null", 1)
    let line: u8[3] = [111, 107, 10]
    if fd >= 0 {
        let n: u32 = write(u32(fd), &line[0], 3)
        close(u32(fd))
        if n == 3 {
            write(1, &line[0], 3)
        }
    }
    write(1, Ptr[u8](0), 0)
}
`,
		want: "ok\n",
	},
	// std.strings.string declares `String = Slice[u8]` and uses Slice[String],
	// so the C program needs the alias before the specialisation built over it:
	// the types are emitted in the order they refer to one another.
	{
		name: "std.strings.string: an alias used by a specialisation, and its len",
		source: `module app

import std.core.ptr
import std.core.slice
import std.core.option
import std.strings.string

fn main() void {
    // "ab,cd,e"
    var bytes: u8[7] = [97, 98, 44, 99, 100, 44, 101]
    let s: String = String{ Ptr[u8](&bytes[0]), 7 }
    // len on a String is std.strings.string's len, not the built-in, which
    // measures strings and arrays and has nothing to say about a struct.
    print(int_to_str(len(s)) + " ")
    var parts: String[4] = [slice_empty[u8](), slice_empty[u8](), slice_empty[u8](), slice_empty[u8]()]
    let out: Slice[String] = Slice[String]{ Ptr[String](&parts[0]), 4 }
    let n: u32 = split(s, 44, out)
    print(int_to_str(n))
    var i: u32 = 0
    while i < n {
        print(" " + int_to_str(len(parts[i])))
        i = i + 1
    }
    let ab: String = String{ Ptr[u8](&bytes[0]), 2 }
    if eq(parts[0], ab) {
        print(" eq")
    }
    if starts_with(s, ab) {
        print(" starts")
    }
    let e: String = String{ Ptr[u8](&bytes[6]), 1 }
    if ends_with(s, e) {
        print(" ends")
    }
    let at: Option[u32] = find(s, parts[1])
    if at.is_some {
        print(" at=" + int_to_str(at.value))
    }
}
`,
		want: "7 3 2 2 1 eq starts ends at=3",
	},
	// A program that only ever writes `String` still gets the Slice[u8] the
	// alias names, and len on it is std.strings.string's len.
	{
		name: "std.strings.string: len on a String, with nothing else naming Slice[u8]",
		source: `module app

import std.strings.string

fn main() void {
    var bytes: u8[5] = [104, 101, 108, 108, 111]
    let s: String = String{ Ptr[u8](&bytes[0]), 5 }
    let head: String = String{ Ptr[u8](&bytes[0]), 2 }
    print(int_to_str(len(s)) + " " + int_to_str(len(head)))
    if starts_with(s, head) {
        print(" starts")
    }
}
`,
		want: "5 2 starts",
	},
	// The built-in len: a string is measured by the runtime on both backends,
	// and a fixed-size array's length is the one its type carries.
	{
		name: "len: a string and a fixed-size array",
		source: `module app

fn label() string {
    return "parity"
}

fn main() void {
    let word: string = "hello"
    let empty: string = ""
    let values: u32[4] = [1, 2, 3, 4]
    var grid: u8[3][2] = [[1, 2], [3, 4], [5, 6]]
    print(int_to_str(len(word)) + " " + int_to_str(len(empty)) + " " + int_to_str(len(label())))
    print(" " + int_to_str(len(values)) + " " + int_to_str(len(grid)))
    var total: u32 = 0
    var i: u32 = 0
    while i < len(values) {
        total = total + values[i]
        i = i + 1
    }
    print(" " + int_to_str(total))
}
`,
		want: "5 0 6 4 3 10",
	},
}
