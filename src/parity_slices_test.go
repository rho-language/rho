//
// @file parity_slices_test.go
// @brief Fixed arrays, Slice[T] and [T]: one meaning on both backends.
//
// Slice[T] is the idiomatic sequence (types/slices.go): `s.len`, `len(s)`,
// `s[i]` checked against the length, `s[a..b]` checked as `0 <= a <= b <= len`,
// `for x in s`, `s = append(s, v)` into the fixed array the slice was cut from,
// and `Slice.from_raw(p, n)`. A fixed array converts to one, and to a `[T]`.
// Each is compared on C (with AddressSanitizer and UBSan) and Go, failures
// included: the same standard error and the same status.
//
// The refusals are checked too — what a C build cannot do on a `[T]`, and what
// append cannot honour — so a program is told what to write instead.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	parityCases = append(parityCases, sliceParityCases...)
}

var sliceParityCases = []parityCase{
	{
		name: "arrays: a length named by a constant, in a local, a parameter and a struct field",
		want: "4 11 25 | 9 0\n",
		source: `module app

const N: u32 = 4

struct Box {
    items: u32[N]
    pair: u8[2]
}

fn total(xs: Slice[u32]) u32 {
    var t: u32 = 0
    for x in xs {
        t = t + x
    }
    return t
}

fn first(xs: u32[N]) u32 {
    return xs[0]
}

fn main() void {
    var store: u32[N]
    var s = store[..0]
    s = append(s, 5)
    s = append(s, 6)
    var b: Box
    b.items[1] = 9
    print(int_to_str(len(store)) + " " + int_to_str(total(s)) + " " + int_to_str(total(store) + first(store) + total(b.items)))
    print(" | " + int_to_str(b.items[1]) + " " + int_to_str(b.pair[1]) + "\n")
}
`,
	},
	{
		name: "slices: len, index, cuts of cuts, and writes through a cut",
		want: "6 6 3 2 3 | 4 4 2 | 99 99 | 0 6\n",
		source: `module app

fn main() void {
    var raw: u32[6] = [1, 2, 3, 4, 5, 6]
    let all: Slice[u32] = raw
    let mid = all[1..4]
    let inner = mid[1..]
    let none = mid[3..]
    print(int_to_str(all.len) + " " + int_to_str(len(all)) + " " + int_to_str(mid.len) + " " + int_to_str(inner.len) + " " + int_to_str(inner[0]) + " | ")
    print(int_to_str(inner[1]) + " " + int_to_str(raw[..4][3]) + " " + int_to_str(all[..2].len) + " | ")
    mid[0] = 99
    print(int_to_str(raw[1]) + " " + int_to_str(all[1]) + " | ")
    print(int_to_str(none.len) + " " + int_to_str(all[..].len) + "\n")
}
`,
	},
	{
		name: "slices: for over a Slice, a cut, a reverse walk and a step",
		want: "21 | 9 | 6 5 4 3 2 1 | 1 3 5 | 0\n",
		source: `module app

fn total(s: Slice[u32]) u32 {
    var sum: u32 = 0
    for x in s {
        sum = sum + x
    }
    return sum
}

fn main() void {
    var raw: u32[6] = [1, 2, 3, 4, 5, 6]
    print(int_to_str(total(raw)) + " | " + int_to_str(total(raw[1..4])) + " | ")
    let s: Slice[u32] = raw
    for x in reverse s {
        print(int_to_str(x) + " ")
    }
    print("| ")
    for x in s step 2 {
        print(int_to_str(x) + " ")
    }
    print("| " + int_to_str(total(s[3..3])) + "\n")
}
`,
	},
	{
		name: "slices: append grows into the fixed array, and a cut back from the start",
		want: "3 60 20 | 0 | 2 7 8 | 7\n",
		source: `module app

fn total(s: Slice[u32]) u32 {
    var sum: u32 = 0
    for x in s {
        sum = sum + x
    }
    return sum
}

fn main() void {
    var buf: u32[4]
    var s = buf[..0]
    s = append(s, 10)
    s = append(s, 20, 30)
    print(int_to_str(s.len) + " " + int_to_str(total(s)) + " " + int_to_str(buf[1]) + " | ")
    s = s[..0]
    print(int_to_str(s.len) + " | ")
    s = append(s, 7)
    s = append(s, 8)
    print(int_to_str(s.len) + " " + int_to_str(buf[0]) + " " + int_to_str(buf[1]) + " | ")
    s = buf[..1]
    print(int_to_str(total(s)) + "\n")
}
`,
	},
	{
		name: "slices: Slice.from_raw over a pointer and a [T], and a fixed array decaying to [T]",
		want: "9 4 | 2 3 | 6 | 15\n",
		source: `module app

fn first(items: [u32]) u32 {
    return items[0]
}

fn main() void {
    var raw: u32[5] = [1, 2, 3, 4, 5]
    let p = Slice.from_raw(&raw[3], 2)
    print(int_to_str(p[0] + p[1]) + " " + int_to_str(p[0]) + " | ")
    let q = Slice[u32].from_raw(Ptr[u32](&raw[1]), 2)
    print(int_to_str(q[0]) + " " + int_to_str(q[1]) + " | ")
    print(int_to_str(first(raw) + first(raw) + q[1] + 1) + " | ")
    let bare: [u32] = [4, 5, 6]
    let r = Slice.from_raw(bare, 3)
    var sum: u32 = 0
    for x in r {
        sum = sum + x
    }
    print(int_to_str(sum) + "\n")
}
`,
	},
	{
		name: "slices: slices of structs and of enums",
		want: "65 55 2 | on - t21\n",
		source: `module app

struct Reading {
    id: u32
    value: i32
}

enum Event {
    Temp(i32)
    Switch(bool)
    Missing
}

fn describe(e: Event) string {
    match e {
        Temp(t) => { return "t" + int_to_str(u32(t)) }
        Switch(on) => { if on { return "on" } else { return "off" } }
        Missing => { return "-" }
    }
}

fn sum_values(rs: Slice[Reading]) i32 {
    var total: i32 = 0
    for r in rs {
        total = total + r.value
    }
    return total
}

fn main() void {
    var readings: Reading[3] = [Reading{ 1, 10 }, Reading{ 2, 20 }, Reading{ 3, 30 }]
    let rs: Slice[Reading] = readings
    rs[1].value = 25
    print(int_to_str(u32(sum_values(rs))) + " " + int_to_str(u32(sum_values(rs[1..]))) + " " + int_to_str(readings[1].id) + " | ")
    var events: Event[3] = [Event.Temp(21), Event.Switch(true), Event.Missing]
    let es: Slice[Event] = events[..]
    for e in es[1..3] {
        print(describe(e) + " ")
    }
    print(describe(es[0]) + "\n")
}
`,
	},
	{
		name: "slices: strings built in loops are kept in a slice, and a literal of strings",
		want: "r1,r2,r3,w0,w1, 5 | bc,d, | r3r2r1 | x1x2\n",
		source: `module app

fn join(words: Slice[string]) string {
    var out: string = ""
    for w in words {
        out = out + w + ","
    }
    return out
}

fn fill(out: Slice[string], prefix: string) void {
    var i: u32 = 0
    while i < out.len {
        out[i] = prefix + int_to_str(i + 1)
        i = i + 1
    }
}

fn main() void {
    var ids: u32[3] = [1, 2, 3]
    var store: string[8]
    var names = store[..0]
    for id in ids {
        names = append(names, "r" + int_to_str(id))
    }
    for k in ids[..2] {
        names = append(names, "w" + int_to_str(k - 1))
    }
    print(join(names) + " " + int_to_str(names.len) + " | ")
    let words: Slice[string] = ["a", "b" + "c", "d"]
    print(join(words[1..]) + " | ")
    for w in reverse names[..3] {
        print(w)
    }
    var two: string[2]
    fill(two, "x")
    print(" | " + two[0] + two[1] + "\n")
}
`,
	},
	{
		name: "slices: std.strings.string over a String, split into a Slice of Strings",
		want: "3 ab|cd|e\n",
		source: `module app

import std.strings.string

fn show(s: String) void {
    for b in s {
        print(str_from_byte(b))
    }
}

fn main() void {
    // "ab,cd,e"
    var bytes: u8[7] = [97, 98, 44, 99, 100, 44, 101]
    let s: String = bytes
    var parts: String[4]
    let n: u32 = split(s, 44, parts)
    print(int_to_str(n) + " ")
    let found: Slice[String] = parts
    for part in found[..n] {
        show(part)
        if part.len != 1 {
            print("|")
        }
    }
    print("\n")
}
`,
	},
	{
		name: "slices: a generic over Slice[T], a Slice field, and a module-level array",
		want: "3 6\n26 13\n3 2\n",
		source: `module app

let TABLE: u32[4] = [5, 6, 7, 8]

struct Holder {
    items: Slice[u32]
    count: u32
}

fn last[T](s: Slice[T]) T {
    var out: T = s[0]
    for x in s[1..] {
        out = x
    }
    return out
}

fn sum(s: Slice[u32]) u32 {
    var total: u32 = 0
    for x in s {
        total = total + x
    }
    return total
}

fn main() void {
    var buf: u32[3] = [1, 2, 3]
    let h = Holder{ buf, 3 }
    print(int_to_str(h.items.len) + " " + int_to_str(sum(h.items)) + "\n")
    let t: Slice[u32] = TABLE
    print(int_to_str(sum(t)) + " " + int_to_str(sum(TABLE[1..3])) + "\n")
    print(int_to_str(last(buf)) + " " + int_to_str(last(h.items[..2])) + "\n")
}
`,
	},
	{
		name: "slices: append of structs, enums and Options",
		want: "8\n5\n7\n",
		source: `module app

struct P {
    x: u32
    y: u32
}

enum E {
    A(u32)
    B
}

fn main() void {
    var pb: P[3]
    var ps = pb[..0]
    ps = append(ps, P{ 1, 2 }, P{ 3, 4 })
    var eb: E[2]
    var es = eb[..0]
    es = append(es, E.A(5))
    es = append(es, E.B)
    var ob: Option[u32][2]
    var os = ob[..0]
    os = append(os, Some(7))
    print(int_to_str(ps[1].x + ps.len + es.len + os.len) + "\n")
    match es[0] {
        A(v) => { print(int_to_str(v) + "\n") }
        B => { print("b\n") }
    }
    if os[0].is_some { print(int_to_str(os[0].value) + "\n") }
}
`,
	},
	{
		name:        "slices: an index past the length stops the program",
		wantFailure: true,
		want:        "3\n",
		wantStderr:  "rho: index 3 out of range [0,3)\n",
		source: `module app

fn main() void {
    var raw: u32[4] = [1, 2, 3, 4]
    let s: Slice[u32] = raw[..3]
    var i: u32 = 2
    print(int_to_str(s[i]) + "\n")
    i = 3
    print(int_to_str(s[i]))
}
`,
	},
	{
		name:        "slices: a negative index into a Slice stops the program",
		wantFailure: true,
		wantStderr:  "rho: index -1 out of range [0,4)\n",
		source: `module app

fn main() void {
    var raw: u32[4] = [1, 2, 3, 4]
    let s: Slice[u32] = raw
    var i: i32 = -1
    s[i] = 7
}
`,
	},
	{
		name:        "slices: a cut past the length stops the program",
		wantFailure: true,
		want:        "ok\n",
		wantStderr:  "rho: slice [1..5] out of range [0,3]\n",
		source: `module app

fn main() void {
    var raw: u32[4] = [1, 2, 3, 4]
    let s: Slice[u32] = raw[..3]
    var hi: u32 = 3
    print(if s[1..hi].len == 2 { "ok\n" } else { "no\n" })
    hi = 5
    let t = s[1..hi]
    print(int_to_str(t.len))
}
`,
	},
	{
		name:        "slices: a cut that starts after it ends stops the program",
		wantFailure: true,
		wantStderr:  "rho: slice [3..2] out of range [0,4]\n",
		source: `module app

fn main() void {
    var raw: u32[4] = [1, 2, 3, 4]
    var lo: u32 = 3
    let t = raw[lo..2]
    print(int_to_str(t.len))
}
`,
	},
	{
		name:        "slices: a cut of a cut is checked against the inner length",
		wantFailure: true,
		want:        "2\n",
		wantStderr:  "rho: slice [0..3] out of range [0,2]\n",
		source: `module app

fn main() void {
    var raw: u32[6] = [1, 2, 3, 4, 5, 6]
    let outer = raw[1..5]
    let inner = outer[1..3]
    print(int_to_str(inner.len) + "\n")
    var n: i32 = 3
    let again = inner[..n]
    print(int_to_str(again.len))
}
`,
	},
	{
		name:        "slices: append past the capacity of the fixed array stops the program",
		wantFailure: true,
		want:        "full\n",
		wantStderr:  "rho: append past capacity 2\n",
		source: `module app

fn main() void {
    var buf: u8[2]
    var s = buf[..0]
    s = append(s, 1)
    s = append(s, 2)
    print("full\n")
    s = append(s, 3)
}
`,
	},
	{
		name: "a local declared without a value is zero on both backends",
		want: "0 0 f [] 0 0 [] eq eq2 circle 0 red 0 0 none\n",
		source: `module app

struct Point {
    x: i32
    y: i32
    name: string
}

enum Shape {
    Circle(u32)
    Square(u32)
    Empty
}

enum Color {
    Red
    Green
}

fn shape_name(s: Shape) string {
    match s {
        Circle(r) => { return "circle " + int_to_str(r) }
        Square(w) => { return "square " + int_to_str(w) }
        Empty => { return "empty" }
    }
}

fn main() void {
    var x: u32
    var f: f64
    var b: bool
    var s: string
    var p: Point
    var sh: Shape
    var c: Color
    var arr: u32[3]
    var names: string[2]
    var sl: Slice[u32]
    var o: Option[u32]
    print(int_to_str(x) + " " + int_to_str(u32(f)) + " ")
    if b { print("t ") } else { print("f ") }
    print("[" + s + "] " + int_to_str(u32(p.x)) + " " + int_to_str(u32(p.y)) + " [" + p.name + "] ")
    if p.name == "" { print("eq ") }
    if names[1] == s { print("eq2 ") }
    print(shape_name(sh) + " ")
    if c == Color.Red { print("red ") }
    print(int_to_str(arr[0] + arr[1] + arr[2]) + " " + int_to_str(sl.len) + " ")
    if o.is_some { print("some\n") } else { print("none\n") }
}
`,
	},
}

// buildOneFile builds a one-file program for the given targets and returns
// the build's error, if any.
func buildOneFile(t *testing.T, source string, targets ...string) error {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: slices\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: c\n  targets:\n"
	for _, target := range targets {
		manifest += "    - " + target + "\n"
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := GenerateAllTargets(entry, "", nil, nil, nil)
	return err
}

// What a Slice cannot honour is refused, on every target, with what to write
// instead: append on a slice whose capacity is unknown, append written as an
// expression, a cut of a [T] or a string, a Slice returned that points at the
// function's own storage, and a constant bound outside a fixed array.
func TestSliceOperationsThatCannotBeHonouredAreRefused(t *testing.T) {
	for _, c := range []struct {
		name, source string
		want         []string
	}{
		{"append to a parameter", `module app
fn grow(s: Slice[u32]) Slice[u32] {
    s = append(s, 1)
    return s
}
fn main() void {}
`, []string{":3:9:", "append cannot grow s", "was not cut from one in this function", "var s = buf[..0]"}},
		{"append as an expression", `module app
fn main() void {
    var buf: u32[4]
    var s = buf[..0]
    let t = append(s, 1)
}
`, []string{":5:13:", "s = append(s, v)"}},
		{"append after the slice was pointed elsewhere", `module app
fn main() void {
    var buf: u32[4]
    var s = buf[..0]
    var other: u32[2]
    s = other[..]
    s = append(s, 1)
}
`, []string{":7:9:", "assigned something other than", "line 6"}},
		{"append on a slice cut from the middle", `module app
fn main() void {
    var buf: u32[4]
    var s = buf[1..1]
    s = append(s, 1)
}
`, []string{"append cannot grow s", "was not cut from one"}},
		{"a cut of a bare pointer", `module app
fn first(p: [u8]) void {
    let s = p[0..2]
}
fn main() void {}
`, []string{":3:13:", "bare pointer with no length", "Slice.from_raw(p, n)"}},
		{"a cut of a string", `module app
fn main() void {
    let s: string = "hello"
    let t = s[1..3]
}
`, []string{"str_slice"}},
		{"a returned cut of a local array", `module app
fn make() Slice[u32] {
    var buf: u32[4]
    return buf[..2]
}
fn main() void {}
`, []string{":4:12:", "cannot return a Slice that points at the fixed array buf"}},
		{"a returned slice that grew into a local array", `module app
fn make() Slice[u32] {
    var buf: u32[4]
    var s = buf[..0]
    s = append(s, 1)
    return s
}
fn main() void {}
`, []string{"the fixed array buf (through s)"}},
		{"a returned array literal", `module app
fn make() Slice[u32] {
    return [1, 2, 3]
}
fn main() void {}
`, []string{"an array literal"}},
		{"a constant bound past a fixed array", `module app
fn main() void {
    var buf: u32[4]
    let s = buf[2..5]
}
`, []string{":4:13:", "slice bound 5 out of range [0,4]"}},
		{"a constant cut that starts after it ends", `module app
fn main() void {
    var buf: u32[4]
    let s = buf[3..1]
}
`, []string{"slice [3..1] starts after it ends"}},
		{"Slice.from_raw of a number", `module app
fn main() void {
    let n: u32 = 4
    let s = Slice.from_raw(n, 2)
}
`, []string{"Slice.from_raw takes the pointer to the first element"}},
		{"indexing a call that answers a Slice", `module app
fn make(b: Slice[u32]) Slice[u32] { return b }
fn main() void {
    var buf: u32[4]
    let x = make(buf)[1]
}
`, []string{":5:13:", "index a Slice through a name"}},
		{"assigning to a cut", `module app
fn main() void {
    var buf: u32[4]
    var s = buf[..0]
    s[0..1] = s
}
`, []string{"a sub-slice is a value, not storage"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := buildOneFile(t, c.source, "c", "go")
			if err == nil {
				t.Fatalf("expected the build to be refused:\n%s", c.source)
			}
			for _, want := range c.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal should mention %q, got:\n%v", want, err)
				}
			}
		})
	}
}

// A `[T]` has no length, so what needs one is refused on every backend with
// the Slice to use instead.
func TestLengthOperationsOnABarePointerAreRefused(t *testing.T) {
	source := `module app

fn count(xs: [u32]) u32 {
    var n: u32 = 0
    for x in xs {
        n = n + x
    }
    return n + len(xs)
}

fn main() void {
    var xs: [u32] = [1, 2, 3]
    xs = append(xs, 4)
    print(int_to_str(count(xs)))
}
`
	for _, target := range []string{"c", "go"} {
		err := buildOneFile(t, source, target)
		if err == nil {
			t.Fatalf("expected the %s build to be refused", target)
		}
		for _, want := range []string{
			":5:5:", "for x in xs: xs is [u32]",
			":8:16:", "len(xs): xs is [u32]",
			":13:10:", "append(xs, ...): xs is [u32]",
			"use a Slice[T]", "Slice.from_raw(p, n)",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the %s refusal should mention %q, got:\n%v", target, want, err)
			}
		}
	}
}
