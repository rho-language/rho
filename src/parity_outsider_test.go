package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The findings of the outsider test (examples/csv-report/FINDINGS.md): each
// behaviour a program written from the documentation alone ran into, run on
// both backends.

func init() {
	parityCases = append(parityCases, outsiderParityCases...)
}

var outsiderParityCases = []parityCase{
	{
		// FINDINGS #1: the element of an array field is the struct, and the
		// length follows the field's name in C.
		name: "a struct field that is an array of structs, enums or a specialisation",
		want: "5 7 1 9 4 6 3\n",
		source: `module app

const N: u32 = 3

struct P { x: i32 }
enum Color { Red, Green }
enum Shape {
    Dot
    Circle(i32)
}
struct Box[T] { v: T }
struct H { p: P[4], q: P[N], c: Color[2], b: Box[i32][2], s: Shape[2] }
struct G[T] { a: P[2], t: T[3] }

fn show(v: i32) void {
    print(i64_to_str(i64(v)) + " ")
}

fn main() void {
    var h: H
    h.p[1].x = 5
    h.q[2].x = 7
    h.c[1] = Color.Green
    h.b[1].v = 9
    h.s[1] = Shape.Circle(3)
    show(h.p[1].x)
    show(h.q[2].x)
    if h.c[1] == Color.Green {
        show(1)
    }
    show(h.b[1].v)
    var g: G[i32]
    g.a[1].x = 4
    g.t[2] = 6
    show(g.a[1].x)
    show(g.t[2])
    match h.s[1] {
        Circle(r) => { print(i64_to_str(i64(r)) + "\n") }
        Dot => { print("dot\n") }
    }
}
`,
	},
	{
		// FINDINGS #3: str_at, str_slice and len give the same answers on C,
		// where a long string's length is remembered, as on Go — at the edges,
		// and for a string built where a released one was.
		name: "str_at, str_slice and len at the edges, and over reused arena memory",
		want: "104 0 0 | abc  ab c  | 120 120 0 | 160 160 0 | 200 200 0 | 70 x 0 |\n",
		source: `module app

fn repeat(s: string, n: u32) string {
    var out: string = ""
    var i: u32 = 0
    while i < n {
        out = out + s
        i = i + 1
    }
    return out
}

fn main() void {
    let a: string = repeat("abcd", 26)
    print(int_to_str(len(a)) + " " + int_to_str(u32(str_at(a, 104))) + " " + int_to_str(u32(str_at(a, 4294967295))) + " | ")
    print(str_slice("abc", 0, 99) + " " + str_slice("abc", 3, 1) + " " + str_slice("abc", 0, 2) + " " + str_slice("abc", 2, 4294967295) + " " + str_slice("", 0, 0) + " | ")
    var j: u32 = 0
    while j < 3 {
        let t: string = repeat("0123456789", 12 + j * 4)
        print(int_to_str(len(t)) + " " + int_to_str(len(str_slice(t, 0, 1000))) + " " + int_to_str(u32(str_at(t, len(t)))) + " | ")
        j = j + 1
    }
    var k: string = repeat("y", 70)
    let n: u32 = len(k)
    k = repeat("x", 3)
    print(int_to_str(n) + " " + str_slice(k, 2, 1) + " " + int_to_str(u32(str_at(k, 3))) + " |\n")
}
`,
	},

	{
		// FINDINGS #4: a C program reads its command line, and the answers are
		// Go's: rho_arg(0) is the first argument after the program's name.
		name: "the command line: rho_arg, arg_count and get_args",
		args: []string{"a b", "-v", "", "last"},
		want: "4 [a b][-v][][last][] [a b -v  last]\n",
		source: `module app

extern fn rho_arg(index: i32) string

fn main() void {
    let n: u32 = arg_count()
    print(int_to_str(n) + " ")
    var i: i32 = 0
    while i <= i32(n) {
        print("[" + rho_arg(i) + "]")
        i = i + 1
    }
    print(rho_arg(-1) + " [" + get_args() + "]\n")
}
`,
	},

	{
		// FINDINGS #8: a qualified call of a generic function deduces as an
		// unqualified one does, and takes explicit type arguments.
		name: "a qualified generic call: deduced from a closure, and with explicit type arguments",
		want: "cba abc a 7\n",
		source: `module app

import std.collections.slices
import std.core.cmp

struct G { k: string, n: u32 }

fn main() void {
    var arr: G[3] = [G { "b", 2 }, G { "a", 3 }, G { "c", 1 }]
    var xs: Slice[G] = arr[..]
    slices.sort_by(xs, fn(a: G, b: G) -> bool { return a.n < b.n })
    print(xs[0].k + xs[1].k + xs[2].k + " ")
    slices.sort_by[G](xs, fn(a: G, b: G) -> bool { return a.k < b.k })
    print(xs[0].k + xs[1].k + xs[2].k + " ")
    sort_by(xs, fn(a: G, b: G) -> bool { return a.n > b.n })
    print(xs[0].k + " " + int_to_str(cmp.max[u32](7, 3)) + "\n")
}
`,
	},
	{
		// FINDINGS #9 and #10: a line that starts with `(` starts a statement,
		// and a field is reached through a pointer to a struct as p.f, read
		// and written, as (*p).f is.
		name: "a line starting with ( is a new statement; p.f through a Ptr to a struct",
		want: "7 h! 12 7 h!\n",
		source: `module app

struct St { n: u32, name: string, inner: In }
struct In { v: i32 }

fn bump(s: Ptr[St]) void {
    s.n = s.n + 1
    s.inner.v = s.inner.v * 2
    s.name = s.name + "!"
}

fn read(s: Ptr[St]) string {
    return s.name
}

fn main() void {
    var x: St = St { 1, "a", In { 3 } }
    let p: Ptr[St] = &x
    bump(p)
    var header: string = "h"
    (*p).name = header
    bump(&x)
    p.n += 4
    let f: fn(Ptr[St]) -> u32 = fn(q: Ptr[St]) -> u32 {
        return q.n
    }
    print(int_to_str(x.n) + " " + x.name + " " + i64_to_str(i64(p.inner.v)) + " " + int_to_str(f(p)) + " " + read(p) + "\n")
}
`,
	},
	{
		// A struct literal is an expression wherever one is delimited: a call's
		// argument, an array element, an operand, a return, and inside
		// parentheses in a condition.
		name: "struct literals as call arguments, elements, operands and in conditions",
		want: "8364 7 ok ok2 23\n",
		source: `module app

import std.strings.utf8
import std.strings.string
import std.core.option

struct P { x: i32, y: i32 }
struct Box[T] { v: T }

fn sum(p: P) i32 {
    return p.x + p.y
}

fn mk() P {
    return P { 1, 2 }
}

fn main() void {
    var b: u8[3] = [226, 130, 172]
    print(int_to_str(value(first_codepoint(String{ Ptr[u8](&b[0]), 3 }))) + " ")
    print(i64_to_str(i64(sum(P { 3, 4 }))) + " ")
    let ps: P[2] = [P { 1, 1 }, P { 2, 2 }]
    let v: i32 = sum(mk()) + sum(P{5, 5}) + Box[i32]{7}.v
    if sum(P { 1, 1 }) == 2 {
        print("ok ")
    }
    if (P { 1, 2 }).x == 1 {
        print("ok2 ")
    }
    print(i64_to_str(i64(v + ps[1].x + ps[0].y)) + "\n")
}
`,
	},
	{
		// A checked % / << in an index on the left of an assignment, in a
		// struct field, a generic body and a narrow type, and a conversion in
		// an index, which was read as a type argument.
		name: "checked operators inside an index that is assigned to",
		want: "24 -3 4\n",
		source: `module app

struct R { a: u32[4], n: u32 }
struct Q[T] { v: T[4] }

fn pick[T](q: Q[T], i: u32) T {
    return q.v[i % 4]
}

fn main() void {
    var ring: u32[4]
    var i: u32 = 0
    var r: R
    var q: Q[u32]
    while i < 10 {
        ring[i % 4] = i
        r.a[i / 3 % 4] = i
        r.a[(i << 1) % 4] = r.a[i % 4] + 1
        r.n = i % 3
        q.v[i % 4] = i >> 1
        i = i + 1
    }
    let k: i32 = -7
    var arr: i32[3]
    arr[u32(k % 3 + 2)] = k / 2
    var small: u8[4]
    var b: u8 = 9
    small[b % 4] = b / 2
    print(int_to_str(ring[1] + ring[2] + r.a[1] + r.n + pick(q, 5)) + " " + i64_to_str(i64(arr[1])) + " " + int_to_str(u32(small[1])) + "\n")
}
`,
	},
	{
		// FINDINGS #17: rho_read_text answers "" for a missing file and an
		// empty one alike; rho_path_exists tells them apart, on C too.
		name: "rho_path_exists tells a missing file from an empty one",
		want: "true true false |  | \n",
		source: `module app

extern fn rho_read_text(path: string) string
extern fn rho_write_text(path: string, text: string) bool
extern fn rho_path_exists(path: string) bool

fn yes(b: bool) string {
    if b {
        return "true "
    }
    return "false "
}

fn main() void {
    let wrote: bool = rho_write_text("empty.txt", "")
    print(yes(wrote) + yes(rho_path_exists("empty.txt")) + yes(rho_path_exists("missing.txt")) + "| ")
    print(rho_read_text("empty.txt") + " | " + rho_read_text("missing.txt") + "\n")
}
`,
	},

	{
		// FINDINGS #11: a float as decimal text. Fixed notation, rounded to
		// nearest from the exact binary value with a tie to even, as Go's
		// strconv.FormatFloat(x, 'f', d, 64); NaN, infinities and -0 alike.
		name: "f64_to_str and f32_to_str at the edges",
		want: "3.14 0.12 0.38 -0.00 2 4 -2 100000000000000000000.000 0.1000000015 NaN +Inf -Inf -0 0.00000000000000000000 179769313486231570814527423731704356798070567525844996598917476803157260780028538760589558632766878171540458953514382464234321326889464182768467546703537516986049910576551282076245490090389328944075868508455133942304583236903222948165808559332123348274797826204144723168738177180919299881250404026184124858368 1.00000000000000000000\n",
		source: `module app

fn main() void {
    print(f64_to_str(3.14159, 2) + " " + f64_to_str(0.125, 2) + " " + f64_to_str(0.375, 2) + " " + f64_to_str(-0.001, 2) + " ")
    print(f64_to_str(2.5, 0) + " " + f64_to_str(3.5, 0) + " " + f64_to_str(-2.5, 0) + " " + f64_to_str(1e20, 3) + " " + f32_to_str(f32(0.1), 10) + " ")
    print(f64_to_str(f64_from_bits(9221120237041090560), 2) + " " + f64_to_str(f64_from_bits(9218868437227405312), 1) + " " + f64_to_str(f64_from_bits(18442240474082181120), 1) + " ")
    print(f64_to_str(f64_from_bits(9223372036854775808), 0) + " " + f64_to_str(f64_from_bits(1), 25) + " " + f64_to_str(f64_from_bits(9218868437227405311), 0) + " " + f64_to_str(0.99999999999999999999, 99) + "\n")
}
`,
	},
	{
		// The same over pseudo-random bit patterns and every number of
		// decimals: the two backends must write the same text for each.
		name: "f64_to_str over random values agrees on both backends",
		source: `module app

fn main() void {
    var state: u64 = 88172645463325252
    var i: u32 = 0
    while i < 600 {
        state = state ^ (state << 13)
        state = state ^ (state >> 7)
        state = state ^ (state << 17)
        var bits: u64 = state
        // A third of them small: a number a program prints.
        if i % 3 == 0 {
            bits = (bits & 4503599627370495) | (u64(1013 + (i % 40)) << 52)
        }
        print(f64_to_str(f64_from_bits(bits), i % 21) + "\n")
        print(f64_to_str(f64(i) / 8.0, i % 4) + " " + f32_to_str(f32(i) / 3.0, i % 9) + "\n")
        i = i + 1
    }
}
`,
	},
	{
		// milli_format drops trailing zeros; milli_format_fixed writes a fixed
		// number of decimals, the half away from zero below three.
		name: "milli_format_fixed",
		want: "1.50 -1.01 0.00 12.34500 1 -2 0.0 9223372036854775.807 -9223372036854775.808\n",
		source: `module app

fn main() void {
    print(milli_format_fixed(1500, 2) + " " + milli_format_fixed(-1005, 2) + " " + milli_format_fixed(-4, 2) + " " + milli_format_fixed(12345, 5) + " ")
    print(milli_format_fixed(999, 0) + " " + milli_format_fixed(-1500, 0) + " " + milli_format_fixed(49, 1) + " ")
    print(milli_format_fixed(9223372036854775807, 3) + " " + milli_format_fixed(-9223372036854775807 - 1, 3) + "\n")
}
`,
	},
	{
		// FINDINGS #12: eprint writes to standard error, no prefix and no
		// newline added.
		name:       "eprint writes to standard error",
		want:       "out\n",
		wantStderr: "err1err2\n",
		source: `module app

fn main() void {
    eprint("err1")
    print("out\n")
    eprint("err2\n")
}
`,
	},
	{
		// FINDINGS #13: a string into a caller's buffer and bytes into a
		// string, which ends at a 0 byte on both backends.
		name: "str_to_bytes and str_from_bytes",
		want: "8 hello|hello wo|ab|0|\n",
		source: `module app

fn main() void {
    var buf: u8[8]
    let n: u32 = str_to_bytes("hello world", buf[..])
    print(int_to_str(n) + " " + str_from_bytes(buf[..5]) + "|" + str_from_bytes(buf) + "|")
    var z: u8[4] = [97, 98, 0, 99]
    var empty: u8[1]
    print(str_from_bytes(z) + "|" + int_to_str(str_to_bytes("", empty[..])) + "|" + str_from_bytes(empty[..0]) + "\n")
}
`,
	},
	{
		// A program's own function under a new built-in's name, written
		// before the built-in existed (robohub-pico's elf2uf2 has an
		// arg_count of its own), is still its own.
		name: "a function of the program's own named like a new built-in",
		want: "3 x\n",
		source: `module app

fn arg_count(args: string) u32 {
    return len(args)
}

fn main() void {
    print(int_to_str(arg_count("abc")) + " x\n")
}
`,
	},

}

// buildCOnly generates a program and builds its C, optimised and without the
// sanitizers, for a test about how fast the runtime is rather than whether it
// is right.
func buildCOnly(t *testing.T, source string) string {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(parityManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}
	cDir := dirs[0]
	binary := filepath.Join(t.TempDir(), "program")
	build := exec.Command("gcc", "-std=c11", "-O1", "-o", binary,
		filepath.Join(cDir, "main.c"), filepath.Join(cDir, "app.c"), filepath.Join(cDir, "runtime.c"), "-I", cDir, "-lm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("C build failed: %v\n%s", err, out)
	}
	return binary
}

// FINDINGS #3: str_at, str_slice and len measured the whole string on every
// call on C, so a scan of a long string was quadratic: this one took 78 s. It is
// linear now, as on Go; the bound is generous, and a quadratic scan misses it by
// minutes.
func TestStringScanIsLinearOnC(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}
	binary := buildCOnly(t, `module app

fn main() void {
    var s: string = "abcdefghi\n"
    var k: u32 = 0
    while k < 17 {
        s = s + s
        k = k + 1
    }
    var i: u32 = 0
    var lines: u32 = 0
    var total: u32 = 0
    while i < len(s) {
        if str_at(s, i) == 10 {
            lines = lines + 1
        }
        if i % 100 == 0 {
            total = total + len(str_slice(s, i, 5))
        }
        i = i + 1
    }
    print(int_to_str(len(s)) + " " + int_to_str(lines) + " " + int_to_str(total) + "\n")
}
`)
	command := exec.Command(binary)
	started := time.Now()
	done := make(chan struct{})
	var out []byte
	var runErr error
	go func() {
		out, runErr = command.Output()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		_ = command.Process.Kill()
		t.Fatalf("a scan of a 1.3 MB string took more than 20 s on C; it is linear on Go")
	}
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if got, want := string(out), "1310720 131072 65540\n"; got != want {
		t.Fatalf("printed %q, want %q", got, want)
	}
	t.Logf("scan took %s", time.Since(started))
}

// A Go-only build of a program that calls a runtime function an imported
// module declares extern (std.os.sys's rho_fd_write) or one the program
// declares itself (rho_arg). The C emitter used to rename the calls in the IR
// both backends share, so a build for both targets worked and a Go-only one
// failed with `undefined: stdOsSysRhoFdWrite`.
func TestAGoOnlyBuildCallsTheRuntimesOwnNames(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(parityManifest, "    - c\n", "", 1)
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `module app

import std.os.sys

extern fn rho_arg(index: i32) string

fn main() void {
    var buf: u8[3] = [104, 105, 10]
    let n: u32 = write(1, buf[..])
    print(int_to_str(n) + "[" + rho_arg(0) + "]\n")
}
`
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "program")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dirs[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Go build failed: %v\n%s", err, out)
	}
	result := runBinary(binary, "x")
	if result.failed != "" || result.stdout != "hi\n3[x]\n" {
		t.Fatalf("printed %q, failed %q", result.stdout, result.failed)
	}
}
