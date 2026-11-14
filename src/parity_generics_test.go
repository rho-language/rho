//
// @file parity_generics_test.go
// @brief Constrained generics, inference and generic values: one meaning on
// both backends.
//
// A constraint on a type parameter (`[T: Number]`, types/constraints.go) lets a
// generic body use the operators every type it may become supports, and each
// specialisation lowers them exactly as the same code at the concrete type: a
// `+` on an i8 wraps and a `/` by zero stops the program, on C under ASan and
// UBSan as on Go. Type arguments are deduced through Slices, fixed arrays,
// Options, function types and nested generic structs, or written, and a
// generic function named with its arguments is a value.
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
	parityCases = append(parityCases, genericsParityCases...)
}

var genericsParityCases = []parityCase{
	{
		name: "generics: Number on every integer width and both floats, wrapping as the concrete type does",
		want: "-56 -736 -2147483648 -9223372036854775808 | 144 64 0 0 | 3 4 | 30 | -2\n",
		source: `module app

fn add[T: Number](a: T, b: T) -> T {
    return a + b
}

fn mul_sub[T: Number](a: T, b: T) -> T {
    return a * b - a
}

fn neg[T: Number](a: T) -> T {
    return -a
}

fn main() void {
    let a8: i8 = 100
    let a16: i16 = 32400
    let a32: i32 = 2147483647
    let a64: i64 = 9223372036854775807
    print(i64_to_str(add(a8, a8) as i64) + " " + i64_to_str(add(a16, a16) as i64) + " ")
    print(i64_to_str(add(a32, 1) as i64) + " " + i64_to_str(add(a64, 1)) + " | ")
    let u8v: u8 = 200
    let u16v: u16 = 65500
    let u32v: u32 = 4294967295
    let u64v: u64 = 18446744073709551615
    print(int_to_str(add(u8v, u8v) as u32) + " " + int_to_str(add(u16v, 100) as u32) + " ")
    print(int_to_str(add(u32v, 1)) + " " + i64_to_str(add(u64v, 1) as i64) + " | ")
    let f: f32 = 1.5
    let d: f64 = 2.25
    print(i64_to_str(add(f, f) as i64) + " " + i64_to_str((add(d, d) * 2.0) as i64 / 2) + " | ")
    print(i64_to_str(mul_sub(6, 6) as i64) + " | ")
    let m: i8 = -128
    print(i64_to_str(neg(m) as i64 + neg(2) as i64 + 128) + "\n")
}
`,
	},
	{
		name: "generics: Ordered on numbers and strings, Equal on bool, strings and a plain enum",
		want: "9 -3 2.5 pear apple | same diff bool mode | lt le\n",
		source: `module app

enum Mode { Idle, Busy }

fn largest[T: Ordered](a: T, b: T) -> T {
    if a > b {
        return a
    }
    return b
}

fn smallest[T: Ordered](a: T, b: T) -> T {
    if a <= b {
        return a
    }
    return b
}

fn same[T: Equal](a: T, b: T) -> bool {
    return a == b
}

fn differ[T: Ordered + Equal](a: T, b: T) -> bool {
    return a != b
}

fn main() void {
    let small: u8 = 9
    print(int_to_str(largest(small, 3) as u32) + " " + i64_to_str(smallest(-3, 4) as i64) + " ")
    let f: f64 = largest(2.5, 1.0)
    print(if f == 2.5 { "2.5 " } else { "? " })
    print(largest("pear", "apple") + " " + smallest("pear", "apple") + " | ")
    if same("x", "x") { print("same ") }
    if differ("x", "y") { print("diff ") }
    if same(true, true) && !same(true, false) { print("bool ") }
    if same(Mode.Busy, Mode.Busy) && !same(Mode.Idle, Mode.Busy) { print("mode") }
    let a: string = "abc"
    let b: string = "abd"
    print(" | ")
    if a < b { print("lt ") }
    if b <= "abd" { print("le\n") }
}
`,
	},
	{
		name: "generics: Integer remainder and bitwise operators, conversions to and from T",
		want: "3 6 5 | 44 7 | 122\n",
		source: `module app

fn rem[T: Integer](a: T, b: T) -> T {
    return a % b
}

fn bits[T: Integer](a: T, b: T) -> T {
    return (a & b) | (a ^ b)
}

fn narrow[T: Integer](v: u32) -> T {
    return T(v)
}

fn widen[T: Number](v: T) -> u32 {
    return v as u32
}

fn scaled[T: Number](v: T) -> T {
    return v * T(10) / T(2) as T
}

fn main() void {
    let a: u16 = 15
    let b: u16 = 4
    print(int_to_str(rem(a, b) as u32) + " " + int_to_str(bits(a, b) as u32 - 9) + " " + int_to_str(rem(-7, 6) as u32 + 6) + " | ")
    let n: u8 = narrow(300)
    print(int_to_str(n as u32) + " " + int_to_str(widen(7.9)) + " | ")
    let s: u8 = 50
    print(int_to_str(scaled(s) as u32) + "\n")
}
`,
	},
	{
		name:        "generics: a division by zero inside a generic body stops the program as the concrete one does",
		want:        "3 ",
		wantFailure: true,
		wantStderr:  "rho: division by zero\n",
		source: `module app

fn ratio[T: Number](a: T, b: T) -> T {
    return a / b
}

fn main() void {
    let a: i64 = 7
    let b: i64 = 2
    print(i64_to_str(ratio(a, b)) + " ")
    let zero: u8 = 0
    let one: u8 = 1
    print(int_to_str(ratio(one, zero) as u32))
}
`,
	},
	{
		name: "generics: deduction through Slice, a fixed array, Option, function types, nested generic structs and the expected type",
		want: "10 | 4 9 | a -5 | 42 | 7 none ok5 | 5 5\n",
		source: `module app

struct Pair[A, B] {
    first: A
    second: B
}

struct Box[T] {
    inner: Pair[T, u32]
}

fn total[T: Number](xs: Slice[T]) -> T {
    var t: T = T(0)
    for x in xs {
        t = t + x
    }
    return t
}

fn or_else[T](o: Option[T], fallback: T) -> T {
    match o {
        Some(v) => { return v }
        None => { return fallback }
    }
}

fn first_of[A, B](p: Pair[A, B]) -> A {
    return p.first
}

fn unbox[T](b: Box[T]) -> T {
    return b.inner.first
}

fn compose[A, B, C](f: fn(A) -> B, g: fn(B) -> C, x: A) -> C {
    return g(f(x))
}

fn make[T: Number]() -> T {
    return T(7)
}

fn biggest[T: Ordered](a: T, b: T) -> T {
    if a > b {
        return a
    }
    return b
}

fn twice(n: u32) -> u32 {
    return n * 2
}

fn label(n: u32) -> string {
    return int_to_str(n)
}

fn main() void {
    var raw: u32[4] = [1, 2, 3, 4]
    print(int_to_str(total(raw)) + " | ")
    let o: Option[u8] = Some[u8](4)
    let e: Option[u8] = None()
    print(int_to_str(or_else(o, 9) as u32) + " " + int_to_str(or_else(e, 9) as u32) + " | ")
    let p = Pair[string, u32]{"a", 1}
    let b = Box[i64]{Pair[i64, u32]{-5, 2}}
    print(first_of(p) + " " + i64_to_str(unbox(b)) + " | ")
    print(compose(twice, label, 21) + " | ")
    let v: u64 = make()
    let r: Result[u32, string] = Ok(5)
    print(i64_to_str(v as i64) + " " + if is_none(e) { "none " } else { "some " })
    match r {
        Ok(n) => print("ok" + int_to_str(n) + " | ")
        Err(m) => print(m)
    }
    let c: u8 = 3
    print(int_to_str(biggest(5, c) as u32) + " " + int_to_str(biggest(c, 5) as u32) + "\n")
}
`,
	},
	{
		name: "generics: explicit type arguments, and a generic function named with them as a value",
		want: "7 8 s 42 | 3:6\n",
		source: `module app

fn id[T](x: T) -> T {
    return x
}

fn pick_first[A, B](a: A, b: B) -> A {
    return a
}

fn doubled[T: Number](x: T) -> T {
    return x + x
}

fn apply(f: fn(u32) -> u32, v: u32) -> u32 {
    return f(v)
}

fn map_all[T, U](s: Slice[T], out: Slice[U], f: fn(T) -> U) -> u32 {
    var i: u32 = 0
    while i < s.len {
        out[i] = f(s[i])
        i = i + 1
    }
    return i
}

fn main() void {
    let g = id[u32]
    print(int_to_str(g(7)) + " " + int_to_str(apply(id[u32], 8)) + " ")
    let p = pick_first[string, u32]
    print(p("s", 1) + " " + int_to_str(apply(doubled[u32], 21)) + " | ")
    var src: u32[3] = [1, 2, 3]
    var dst: u32[3] = [0, 0, 0]
    let n = map_all(src, dst, doubled[u32])
    print(int_to_str(n) + ":" + int_to_str(dst[2]) + "\n")
}
`,
	},
	{
		name: "generics: generic enums built, matched and returned by generic functions",
		want: "12 12 0 | at2 none | zero 3\n",
		source: `module app

enum Shape[T] {
    Circle(T)
    Rect(T, T)
    Empty
}

fn area[T: Number](s: Shape[T]) -> T {
    match s {
        Circle(r) => { return r * r * T(3) }
        Rect(w, h) => { return w * h }
        Empty => { return T(0) }
    }
}

fn square_of[T: Number](side: T) -> Shape[T] {
    return Shape[T].Rect(side, side)
}

fn find[T: Equal](xs: Slice[T], x: T) -> Option[u32] {
    var i: u32 = 0
    for v in xs {
        if v == x {
            return Some(i)
        }
        i = i + 1
    }
    return None[u32]()
}

fn checked_div[T: Integer](a: T, b: T) -> Result[T, string] {
    if b == 0 {
        return Err[T, string]("zero")
    }
    return Ok[T, string](a / b)
}

fn main() void {
    let s: Shape[u32] = Shape.Rect(3, 4)
    let none: Shape[f64] = Shape[f64].Empty
    print(int_to_str(area(s)) + " " + i64_to_str(area(Shape[i64].Circle(2))) + " " + i64_to_str(area(none) as i64) + " | ")
    var words: string[3] = ["a", "b", "c"]
    match find(words, "c") {
        Some(i) => print("at" + int_to_str(i) + " ")
        None => print("none ")
    }
    if let Some(i) = find(words, "z") {
        print("at" + int_to_str(i))
    } else {
        print("none | ")
    }
    match checked_div(7, 0) {
        Ok(v) => print(int_to_str(v as u32))
        Err(m) => print(m + " ")
    }
    let side: u16 = 9
    match square_of(side) {
        Rect(w, h) => print(int_to_str((w / 3) as u32) + "\n")
        Circle(r) => print("circle\n")
        Empty => print("empty\n")
    }
}
`,
	},
	{
		name: "generics: a constrained generic calls another, takes a closure and makes one in its body",
		want: "15 | 6 | -126 4\n",
		source: `module app

fn add[T: Number](a: T, b: T) -> T {
    return a + b
}

fn sum3[T: Integer](a: T, b: T, c: T) -> T {
    return add(add(a, b), c)
}

fn apply_twice[T: Number](x: T, f: fn(T) -> T) -> T {
    return f(f(x))
}

fn add_all[T: Number](s: Slice[T], k: T) -> T {
    let f = fn(v: T) -> T { return v + k }
    var t: T = T(0)
    for x in s {
        t = t + f(x)
    }
    return t
}

fn main() void {
    let a: u8 = 4
    print(int_to_str(sum3(a, 5, 6) as u32) + " | ")
    print(int_to_str(apply_twice(4, fn(v: i32) -> i32 { return v - 1 }) as u32 + 4) + " | ")
    var small: i8[2] = [100, 20]
    var halves: f64[2] = [0.5, 1.5]
    print(i64_to_str(add_all(small, 5) as i64) + " " + i64_to_str(add_all(halves, 1.0) as i64) + "\n")
}
`,
	},
}

func TestGenericsThatCannotBeHonouredAreRefused(t *testing.T) {
	for _, c := range []struct {
		name, source string
		want         []string
	}{
		{"an operator on an unconstrained type parameter", `module app
fn add[T](a: T, b: T) -> T {
    return a + b
}
fn main() void {}
`, []string{":3:12:", "operator + is not defined on T, a type parameter with no constraint", "[T: Number]"}},
		{"ordering an unconstrained type parameter", `module app
fn less[T](a: T, b: T) -> bool {
    return a < b
}
fn main() void {}
`, []string{":3:12:", "operator < is not defined on T", "[T: Ordered]"}},
		{"arithmetic on a parameter that is only Ordered", `module app
fn add[T: Ordered](a: T, b: T) -> T {
    return a + b
}
fn main() void {}
`, []string{":3:12:", "a type parameter with Ordered: it needs Number", "[T: Ordered + Number]"}},
		{"remainder on a Number", `module app
fn rem[T: Number](a: T, b: T) -> T {
    return a % b
}
fn main() void {}
`, []string{":3:12:", "it needs Integer"}},
		{"a type argument that does not satisfy the constraint", `module app
fn add[T: Number](a: T, b: T) -> T {
    return a + b
}
fn main() void {
    print(add("a", "b"))
}
`, []string{":6:11:", "add needs its type parameter T to satisfy Number, and string does not"}},
		{"a struct where Equal is wanted", `module app
struct P { x: u32 }
fn same[T: Equal](a: T, b: T) -> bool {
    return a == b
}
fn main() void {
    let p = P{1}
    if same(p, p) { print("x") }
}
`, []string{":8:8:", "same needs its type parameter T to satisfy Equal, and P does not"}},
		{"an explicit type argument that does not satisfy the constraint", `module app
fn id[T: Integer](x: T) -> T {
    return x
}
fn main() void {
    let x = id[f64](1.5)
}
`, []string{":6:13:", "id needs its type parameter T to satisfy Integer, and f64 does not"}},
		{"a generic value whose type argument does not satisfy the constraint", `module app
fn id[T: Integer](x: T) -> T {
    return x
}
fn main() void {
    let f = id[f64]
}
`, []string{":6:13:", "satisfy Integer, and f64 does not"}},
		{"a generic value deduced from a function type that does not satisfy the constraint", `module app
fn id[T: Integer](x: T) -> T {
    return x
}
fn main() void {
    let f: fn(f64) -> f64 = id
}
`, []string{":6:29:", "satisfy Integer, and f64 does not"}},
		{"passing an unconstrained parameter on to a constrained one", `module app
fn g[U: Number](a: U) -> U {
    return a
}
fn f[T](a: T) -> T {
    return g(a)
}
fn main() void {}
`, []string{":6:12:", "g needs its type parameter U to satisfy Number, and T (no constraint) is not known to", "[T: Number]"}},
		{"an unknown constraint", `module app
fn add[T: Numeric](a: T, b: T) -> T {
    return a
}
fn main() void {}
`, []string{":2:11:", "unknown constraint Numeric on type parameter T of add", "Integer, Float, Number, Ordered, Equal"}},
		{"a constraint on a struct's parameter", `module app
struct Box[T: Number] { v: T }
fn main() void {}
`, []string{":2:15:", "a constraint is written on a function's type parameter"}},
		{"a type parameter that cannot be deduced", `module app
fn make[T]() -> T {
    return default(T)
}
fn main() void {
    let x = make()
}
`, []string{":6:13:", "cannot deduce the type arguments of make from this call: nothing passed says what T is", "make[T](...)"}},
		{"one of two type parameters that cannot be deduced", `module app
fn pair[A, B](a: A) -> A {
    return a
}
fn main() void {
    let x = pair(1)
}
`, []string{":6:13:", "nothing passed says what B is", "pair[A, B](...)"}},
		{"some type arguments written and the rest left out", `module app
fn pair[A, B](a: A, b: B) -> A {
    return a
}
fn main() void {
    let x = pair[u32](1, "x")
}
`, []string{":6:13:", "pair takes 2 type arguments (A, B), and 1 are written, leaving B with no type and no default"}},
		{"a number that does not fit every type the parameter may be", `module app
fn f[T: Number](a: T) -> T {
    return a + 300
}
fn main() void {}
`, []string{":3:16:", "the number 300 does not fit every type T (Number) may be", "T(x)"}},
		{"a fraction meeting a parameter that may be an integer", `module app
fn f[T: Number](a: T) -> T {
    return a * 0.5
}
fn main() void {}
`, []string{":3:16:", "the number 0.5 does not fit every type T (Number) may be"}},
		{"a type parameter beside a concrete type", `module app
fn f[T: Number](a: T, b: u32) -> T {
    return a * b
}
fn main() void {}
`, []string{":3:12:", "T is a type parameter while the other side is u32", "T(value)"}},
		{"two different type parameters", `module app
fn f[T: Number, U: Number](a: T, b: U) -> T {
    return a + b
}
fn main() void {}
`, []string{":3:12:", "T and U are different type parameters"}},
		{"a conversion to a parameter that is not a number", `module app
fn f[T: Equal](a: u32) -> T {
    return T(a)
}
fn main() void {}
`, []string{":3:12:", "cannot convert to T, a type parameter with Equal", "[T: Number]"}},
		{"a number meeting a parameter that is not one", `module app
fn f[T: Equal](a: T) -> bool {
    return a == 0
}
fn main() void {}
`, []string{":3:17:", "a number cannot meet T, a type parameter with Equal"}},
		{"a shift on a type parameter", `module app
fn f[T: Integer](a: T) -> T {
    return a << 1
}
fn main() void {}
`, []string{":3:12:", "operator << is not defined on T", "convert to a type of known width first"}},
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
