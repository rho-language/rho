package rho

// What an integer operation means where C leaves it undefined: signed overflow,
// a shift by the width of the type or more, a division by zero, an index past
// the end. Each was a known divergence; Rho now defines it (types/checked.go)
// and both backends must give the defined answer.

func init() {
	parityCases = append(parityCases, semanticsParityCases...)
}

var semanticsParityCases = []parityCase{
	{
		name: "signed overflow wraps, two's complement, at every width",
		want: "-2147483648 2147483647 -2 -2147483648 -2147483648 | -128 32767 -1023 | -9223372036854775808 | 44 1 255 56 | -2147483648\n",
		source: `module app

import std.math.basic

fn show(v: i64) void {
    print(i64_to_str(v) + " ")
}

fn main() void {
    var a: i32 = 2147483647
    let lo: i32 = -2147483647 - 1
    show(i64(a + 1))
    show(i64(lo - 1))
    show(i64(a * 2))
    show(i64(-lo))
    show(i64(abs_i32(lo)))
    print("| ")
    var b: i8 = 127
    show(i64(b + 1))
    var c: i16 = -32768
    show(i64(c - 1))
    var d: i16 = 255
    show(i64(d * d * d * d))
    print("| ")
    var e: i64 = 9223372036854775807
    show(e + 1)
    print("| ")
    var f: u8 = 200
    var g: u8 = 100
    show(i64(f + g))
    var h: u16 = 65535
    show(i64(h * h))
    var z: u8 = 0
    show(i64(z - 1))
    var k: u8 = 200
    show(i64(-k))
    print("| ")
    a += 1
    print(i64_to_str(i64(a)) + "\n")
}
`,
	},
	{
		name: "a shift by the width of the type or more gives 0, or the sign",
		want: "0 0 0 144 0 | -4 -1 0 -2147483648 0 -9223372036854775808 | 0 0 | 0 255\n",
		source: `module app

fn show(v: i64) void {
    print(i64_to_str(v) + " ")
}

fn main() void {
    var s: u32 = 32
    let v: u32 = 1
    show(i64(v << s))
    show(i64(v >> s))
    var b: u8 = 1
    var eight: u32 = 8
    show(i64(b << eight))
    var c: u8 = 200
    var one: u32 = 1
    show(i64(c << one))
    var big: u64 = 64
    var w: u64 = 1
    show(i64(w << big))
    print("| ")
    var neg: i32 = -8
    show(i64(neg >> one))
    var m: i32 = -1
    var forty: u32 = 40
    show(i64(m >> forty))
    var p: i32 = 5
    show(i64(p >> forty))
    var q: i32 = 1
    var thirtyone: u32 = 31
    show(i64(q << thirtyone))
    show(i64(q << s))
    var r: i64 = 1
    var sixtythree: u32 = 63
    show(r << sixtythree)
    print("| ")
    var minus: i32 = -1
    show(i64(v << minus))
    var x: u32 = 7
    x = x << s
    show(i64(x))
    print("| ")
    var n: u8 = 0
    var o: u8 = 255
    show(i64(n >> one))
    print(i64_to_str(i64(o >> u32(0))) + "\n")
}
`,
	},
	{
		name: "the most negative value divided by -1 wraps, and the remainder is 0",
		want: "-2147483648 0 -3 -1 3 1 66 -128\n",
		source: `module app

fn show(v: i64) void {
    print(i64_to_str(v) + " ")
}

fn main() void {
    var lo: i32 = -2147483647 - 1
    var m: i32 = -1
    show(i64(lo / m))
    show(i64(lo % m))
    var a: i32 = -7
    var two: i32 = 2
    show(i64(a / two))
    show(i64(a % two))
    var b: i32 = 7
    show(i64(b / two))
    show(i64(b % two))
    var c: u8 = 200
    var three: u8 = 3
    show(i64(c / three))
    var d: i8 = -128
    var e: i8 = -1
    print(i64_to_str(i64(d / e)) + "\n")
}
`,
	},
	{
		name:        "division by zero stops the program on both backends",
		wantFailure: true,
		want:        "before\n",
		wantStderr:  "rho: division by zero\n",
		source: `module app

fn main() void {
    print("before\n")
    var z: u32 = 0
    print(int_to_str(10 / z))
}
`,
	},
	{
		name:        "a remainder by zero stops the program on both backends",
		wantFailure: true,
		wantStderr:  "rho: division by zero\n",
		source: `module app

fn main() void {
    var z: i64 = 0
    var a: i64 = 10
    print(i64_to_str(a % z))
}
`,
	},
	{
		name:        "an index past the end of a fixed array stops the program",
		wantFailure: true,
		want:        "1\n",
		wantStderr:  "rho: index 4 out of range [0,4)\n",
		source: `module app

fn main() void {
    let v: u32[4] = [1, 2, 3, 4]
    var i: u32 = 0
    print(int_to_str(v[i]) + "\n")
    i = 4
    print(int_to_str(v[i]))
}
`,
	},
	{
		name:        "a negative index stops the program",
		wantFailure: true,
		wantStderr:  "rho: index -1 out of range [0,4)\n",
		source: `module app

fn main() void {
    var v: u32[4] = [1, 2, 3, 4]
    var i: i32 = -1
    v[i] = 9
    print(int_to_str(v[0]))
}
`,
	},
	{
		name:        "an index past the length of a Slice stops the program",
		wantFailure: true,
		want:        "3\n",
		wantStderr:  "rho: index 3 out of range [0,3)\n",
		source: `module app

import std.core.slice

fn at(s: Slice[u8], i: u32) u8 {
    return s.ptr[i]
}

fn main() void {
    var raw: u8[8] = [1, 2, 3, 4, 5, 6, 7, 8]
    let s: Slice[u8] = Slice[u8]{ Ptr[u8](&raw[0]), 3 }
    print(int_to_str(u32(at(s, 2))) + "\n")
    print(int_to_str(u32(at(s, 3))))
}
`,
	},
	{
		name: "indexes in range, through arrays of arrays and a Slice",
		want: "6 3 12\n",
		source: `module app

import std.core.slice

fn main() void {
    var grid: u32[2][3] = [[1, 2, 3], [4, 5, 6]]
    var i: u32 = 1
    var j: u32 = 2
    var raw: u8[4] = [3, 4, 5, 0]
    let s: Slice[u8] = Slice[u8]{ Ptr[u8](&raw[0]), 3 }
    var total: u32 = 0
    var k: u32 = 0
    while k < s.len {
        total = total + u32(s.ptr[k])
        k = k + 1
    }
    print(int_to_str(grid[i][j]) + " " + int_to_str(u32(s.ptr[0])) + " " + int_to_str(total) + "\n")
}
`,
	},
	{
		name:        "abort stops with the same status and message on both backends",
		wantFailure: true,
		want:        "x",
		wantStderr:  "rho: aborted\n",
		source: `module app

fn main() void {
    print("x")
    abort()
}
`,
	},
}
