//
// @file parity_closures_test.go
// @brief Function values and closures: one meaning on both backends.
//
// A function type `fn(A) -> R`, a named function passed and returned, a value
// kept in a struct field, an array and a Slice, and closures capturing by value
// (types/closures.go). C builds each with AddressSanitizer, which is what says
// whether an environment kept in the string arena outlives what it captured:
// the loops below build enough strings to reuse a released chunk.
//
// The refusals are checked too, each at its file, line and column, with what
// to write instead.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"strings"
	"testing"
)

func init() {
	parityCases = append(parityCases, closureParityCases...)
}

var closureParityCases = []parityCase{
	{
		name: "closures: a named function passed, stored in a struct, returned and called",
		want: "42 | 11 | 7 | h10 | 12\n",
		source: `module app

struct Handler {
    name: string
    run: fn(i32) -> i32
}

fn twice(x: i32) -> i32 {
    return x * 2
}

fn apply(f: fn(i32) -> i32, v: i32) -> i32 {
    return f(v)
}

fn make_adder(k: i32) -> fn(i32) -> i32 {
    return fn(x: i32) -> i32 { return x + k }
}

fn pick(first: bool) -> fn(i32) -> i32 {
    if first {
        return twice
    }
    return make_adder(1)
}

fn main() void {
    print(int_to_str(apply(twice, 21) as u32) + " | ")
    var k: i32 = 10
    let add = fn(x: i32) -> i32 { return x + k }
    k = 100
    print(int_to_str(add(1) as u32) + " | ")
    let a3 = make_adder(3)
    print(int_to_str(a3(4) as u32) + " | ")
    let h = Handler{"h", twice}
    print(h.name + int_to_str(h.run(5) as u32) + " | ")
    print(int_to_str(pick(true)(5) as u32 + pick(false)(1) as u32) + "\n")
}
`,
	},
	{
		name: "closures: capturing a struct, an enum, a string and another closure, by value",
		want: "34 | ab | 50 | 42 | void\n",
		source: `module app

struct Point {
    x: i32
    y: i32
}

enum Shape {
    Circle(i32)
    Rect { w: i32, h: i32 }
}

fn area(s: Shape) -> i32 {
    match s {
        Shape.Circle(r) => { return 3 * r * r }
        Shape.Rect { w, h } => { return w * h }
    }
}

fn main() void {
    var p = Point{3, 8}
    var s = Shape.Rect { 2, 5 }
    let g = fn() -> i32 { return p.x * p.y + area(s) }
    p = Point{0, 0}
    s = Shape.Circle(9)
    print(int_to_str(g() as u32) + " | ")
    var word = "a" + "b"
    let w = fn() -> string { return word }
    word = "changed"
    print(w() + " | ")
    let outer = fn(a: i32) -> fn(i32) -> i32 {
        return fn(b: i32) -> i32 { return a * 10 + b }
    }
    let five = outer(5)
    print(int_to_str(five(0) as u32) + " | ")
    let call = fn(f: fn(i32) -> i32) -> i32 { return f(2) }
    print(int_to_str(call(outer(4)) as u32) + " | ")
    let say = fn(m: string) { print(m) }
    say("void\n")
}
`,
	},
	{
		name: "closures: arrays, a Slice and an Option of functions, a generic higher-order function",
		want: "149 | 6 | 5 | 20 | 6 none | 33\n",
		source: `module app

fn map_into[T, U](s: Slice[T], out: Slice[U], f: fn(T) -> U) void {
    var i: u32 = 0
    while i < s.len {
        out[i] = f(s[i])
        i = i + 1
    }
}

fn twice[T](x: T, f: fn(T) -> T) -> T {
    let g = fn(y: T) -> T { return f(f(y)) }
    return g(x)
}

fn sq(x: u32) -> string {
    return int_to_str(x * x)
}

fn inc(x: u32) -> u32 {
    return x + 1
}

fn compose(f: fn(u32) -> u32, g: fn(u32) -> u32) -> fn(u32) -> u32 {
    return fn(x: u32) -> u32 { return g(f(x)) }
}

fn pick(n: u32) -> Option[fn(u32) -> u32] {
    if n == 0 {
        return None()
    }
    return Some(fn(x: u32) -> u32 { return x + n })
}

fn show(o: Option[fn(u32) -> u32]) -> string {
    match o {
        Some(g) => { return int_to_str(g(1)) }
        None => { return "none" }
    }
}

fn main() void {
    var raw: u32[3] = [1, 2, 3]
    var outs: string[3] = ["", "", ""]
    map_into(raw, outs, sq)
    print(outs[0] + outs[1] + outs[2] + " | ")
    var doubled: u32[3]
    map_into(raw, doubled, fn(x: u32) -> u32 { return x * 2 })
    print(int_to_str(doubled[2]) + " | ")
    let three: u32 = 3
    print(int_to_str(twice(three, inc)) + " | ")
    let h = compose(inc, fn(x: u32) -> u32 { return x * 10 })
    print(int_to_str(h(1)) + " | ")
    print(show(pick(5)) + " " + show(pick(0)) + " | ")
    var table: (fn(u32) -> u32)[2] = [inc, h]
    let s: Slice[fn(u32) -> u32] = table
    var acc: u32 = 0
    for g in s {
        acc = acc + g(2)
    }
    print(int_to_str(acc) + "\n")
}
`,
	},
	{
		name: "closures: made in loops, capturing strings an iteration built, kept and not kept",
		want: "item0!item199! | 13890 | 36 | 5\n",
		source: `module app

fn countdown(n: u32) -> u32 {
    if n == 0 {
        return 0
    }
    let f: fn(u32) -> u32 = countdown
    return 1 + f(n - 1)
}

fn main() void {
    var keep: (fn() -> string)[200]
    var i: u32 = 0
    while i < 200 {
        let label = "item" + int_to_str(i)
        keep[i] = fn() -> string { return label + "!" }
        i = i + 1
    }
    print(keep[0]() + keep[199]() + " | ")
    var n: u32 = 0
    var total: u32 = 0
    while n < 2500 {
        let word = "w" + int_to_str(n)
        let check = fn(m: u32) -> u32 { return len(word + "-") + m }
        total = total + check(0)
        n = n + 1
    }
    print(int_to_str(total) + " | ")
    var raw: u32[3] = [1, 2, 3]
    var sum: u32 = 0
    for x in raw {
        let t = fn(y: u32) -> u32 { return x + y }
        sum = sum + t(10)
    }
    print(int_to_str(sum) + " | ")
    print(int_to_str(countdown(5)) + "\n")
}
`,
	},
	{
		name:        "closures: calling a function value that holds none stops the program",
		want:        "before\n",
		wantFailure: true,
		wantStderr:  "rho: call of a function value that holds no function\n",
		source: `module app

struct Hooks {
    on_tick: fn(u32) -> u32
}

fn main() void {
    var hooks: Hooks
    print("before\n")
    print(int_to_str(hooks.on_tick(1)))
}
`,
	},
}

// What a closure cannot do, and what is not a function, is refused where it is
// written, with what to write instead.
func TestClosuresThatCannotBeHonouredAreRefused(t *testing.T) {
	for _, c := range []struct {
		name, source string
		want         []string
	}{
		{"assigning to a capture", `module app
fn main() void {
    var n: u32 = 1
    let f = fn() { n = 2 }
    f()
}
`, []string{":4:20:", "a closure cannot assign to n, which it captured", "var n_copy = n"}},
		{"taking the address of a capture", `module app
fn main() void {
    var n: u32 = 1
    let f = fn() -> u32 { let p = &n
        return 1 }
}
`, []string{":4:35:", "cannot take the address of n"}},
		{"capturing a Slice", `module app
fn main() void {
    var raw: u32[2] = [1, 2]
    let s: Slice[u32] = raw
    let f = fn() -> u32 { return s[0] }
}
`, []string{":5:27:", "cannot capture s (Slice[u32]), which holds a Slice", "pass s as a parameter of type Slice[u32]"}},
		{"capturing a struct holding a pointer", `module app
struct Box { p: Ptr[u32] }
fn main() void {
    var n: u32 = 1
    let b = Box{&n}
    let f = fn() -> u32 { return 1 + b.p as u32 }
}
`, []string{":6:27:", "cannot capture b (Box), which holds a pointer"}},
		{"capturing a fixed array", `module app
fn main() void {
    var raw: u32[2] = [1, 2]
    let f = fn() -> u32 { return raw[0] }
}
`, []string{":4:27:", "cannot capture raw, the fixed array u32[2]", "wrap it in a struct"}},
		{"calling a number", `module app
fn main() void {
    let x: u32 = 3
    print(int_to_str(x(2)))
}
`, []string{":4:22:", "x is a u32, not a function"}},
		{"too many arguments", `module app
fn main() void {
    let f = fn(a: u32) -> u32 { return a }
    print(int_to_str(f(1, 2)))
}
`, []string{":4:22:", "f is fn(u32) -> u32 and takes 1 argument, and this call passes 2"}},
		{"an argument of the wrong type", `module app
fn main() void {
    let f = fn(a: u32) -> u32 { return a }
    print(int_to_str(f("x")))
}
`, []string{":4:24:", "argument 1 of f has type string, want u32"}},
		{"a literal of another signature", `module app
fn main() void {
    let f: fn(u32) -> u32 = fn(a: i32) -> i32 { return a }
}
`, []string{":3:29:", "cannot use a fn(i32) -> i32 as fn(u32) -> u32"}},
		{"comparing function values", `module app
fn main() void {
    let f = fn(a: u32) -> u32 { return a }
    if f == f { print("x") }
}
`, []string{":4:8:", "function values cannot be compared with =="}},
		{"a built-in as a value", `module app
fn main() void {
    let f = print
}
`, []string{":3:13:", "print is built into the language and is not a function value"}},
		{"printing a function value", `module app
fn main() void {
    let f = fn(a: u32) -> u32 { return a }
    print(f)
}
`, []string{":4:11:", "print writes a string, and f is a function value"}},
		{"a literal that can reach its end", `module app
fn main() void {
    let f = fn(a: u32) -> u32 { if a > 1 { return a } }
}
`, []string{"function literal: missing return"}},
		{"a generic function named where no function type says which", `module app
fn pick(n: u32) -> Option[u32] {
    if n == 0 {
        return None
    }
    return Some(n)
}
fn main() void {}
`, []string{":4:16:", "the generic function None is not a value here", "None(...)"}},
		{"a function type returning a fixed array", `module app
fn main() void {
    var fs: fn() -> u32[4]
}
`, []string{":3:21:", "a function type cannot return the fixed array u32[4]", "(fn(...) -> u32)[4]"}},
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
