//
// @file parity_std_collections_test.go
// @brief std.collections on both backends.
//
// Array, BitSet and Map hold a pointer to memory the caller owns, so a write
// through one is a write to the caller's array. Array and Map take a count as a
// value parameter (`Array[u32, 4]`), which the compiler substitutes into the
// body as the number it is.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

func init() {
	parityCases = append(parityCases, stdCollectionsParityCases...)
}

var stdCollectionsParityCases = []parityCase{
	// A struct literal of a type with a value argument, `Array[u32, 4]{ ... }`, did not
	// parse; N in the generic body was emitted as the name `N`.
	{
		name: "std.collections.array: get, set and as_slice through Array[T, N]",
		source: `module app

import std.core.ptr
import std.core.slice
import std.collections.array

fn main() void {
    var storage: u32[4] = [0, 0, 0, 0]
    let a: Array[u32, 4] = Array[u32, 4]{ Ptr[u32](&storage[0]) }
    var i: u32 = 0
    while i < 4 {
        set(a, i, (i + 1) * 10)
        i = i + 1
    }
    // set writes through the pointer, so the caller's array holds it.
    print(int_to_str(get(a, 0)) + " " + int_to_str(get(a, 3)) + " " + int_to_str(storage[2]))
    // as_slice carries the count the type was written with.
    let s: Slice[u32] = as_slice(a)
    var sum: u32 = 0
    var j: u32 = 0
    while j < s.len {
        sum = sum + s.ptr[j]
        j = j + 1
    }
    print(" len=" + int_to_str(s.len) + " sum=" + int_to_str(sum))
    var bytes: u8[2] = [7, 9]
    let b: Array[u8, 2] = Array[u8, 2]{ Ptr[u8](&bytes[0]) }
    print(" " + int_to_str(u32(get(b, 1))) + " " + int_to_str(as_slice(b).len))
}
`,
		want: "10 40 30 len=4 sum=100 9 2",
	},
	// BitSet[N] held its words by value, so set and clear changed a copy; and
	// `BitSet[2]` reads as an array of two BitSets. It is a pointer and a count now.
	{
		name: "std.collections.bitset: set, clear and get over the caller's words",
		source: `module app

import std.core.ptr
import std.collections.bitset

fn show(b: BitSet, i: u32) void {
    print(if get(b, i) { " +" } else { " -" })
    print(int_to_str(i))
}

fn main() void {
    var words: u64[2] = [0, 0]
    let b: BitSet = bitset(&words[0], 2)
    print("cap=" + int_to_str(capacity(b)))
    set(b, 0)
    set(b, 3)
    set(b, 63)
    set(b, 64)
    set(b, 127)
    // Past the last word: ignored, and nothing is written past the array.
    set(b, 128)
    show(b, 0)
    show(b, 3)
    show(b, 4)
    show(b, 63)
    show(b, 64)
    show(b, 127)
    show(b, 128)
    clear(b, 3)
    clear(b, 500)
    show(b, 3)
    // The words the caller owns are the set: bit 63 and bit 0 of each.
    print(" w0=" + i64_to_str(i64(words[0] >> 32)) + ":" + i64_to_str(i64(words[0] & 0xffffffff)))
    print(" w1=" + i64_to_str(i64(words[1] >> 32)) + ":" + i64_to_str(i64(words[1] & 0xffffffff)))
}
`,
		want: "cap=128 +0 +3 -4 +63 +64 +127 -128 -3 w0=2147483648:1 w1=2147483648:1",
	},
	// An array of a generic struct (`Entry[u32, u32][4]`) was read as an application
	// with a third argument, and the field `Ptr[Entry[K, V]]` was emitted unmangled.
	{
		name: "std.collections.map: insert, probing, a full table and get",
		source: `module app

import std.core.ptr
import std.core.option
import std.collections.map

fn found(m: Map[u32, u32, 4], key: u32) string {
    let v: Option[u32] = get(m, key)
    if v.is_some {
        return int_to_str(v.value)
    }
    return "none"
}

fn main() void {
    var slots: Entry[u32, u32][4] = [Entry[u32, u32]{ false, 0, 0 }, Entry[u32, u32]{ false, 0, 0 }, Entry[u32, u32]{ false, 0, 0 }, Entry[u32, u32]{ false, 0, 0 }]
    let m: Map[u32, u32, 4] = Map[u32, u32, 4]{ Ptr[Entry[u32, u32]](&slots[0]) }
    print(if insert(m, 7, 70) { "i7 " } else { "-i7 " })
    print(if insert(m, 11, 110) { "i11 " } else { "-i11 " })
    print(if insert(m, 3, 30) { "i3 " } else { "-i3 " })
    print(if insert(m, 15, 150) { "i15 " } else { "-i15 " })
    // Every slot is taken.
    print(if insert(m, 99, 990) { "i99 " } else { "-i99 " })
    print(found(m, 7) + " " + found(m, 11) + " " + found(m, 3) + " " + found(m, 15) + " " + found(m, 99))
    var used: u32 = 0
    var i: u32 = 0
    while i < 4 {
        if slots[i].used {
            used = used + 1
        }
        i = i + 1
    }
    print(" used=" + int_to_str(used) + " h=" + int_to_str(hash_u32(3)) + " " + int_to_str(hash_u32(0)))
}
`,
		want: "i7 i11 i3 i15 -i99 70 110 30 150 none used=4 h=3668339987 0",
	},
	// Regression for the compiler fixes std.collections needed, without the library.
	{
		name: "generics: arrays of a specialisation, a value parameter, a pointer field",
		source: `module app

struct Pair[A, B] {
    a: A
    b: B
}

struct Ring[T, N] {
    items: Ptr[Pair[T, T]]
}

fn last_b[T, N](r: Ring[T, N]) T {
    return r.items[N - 1].b
}

fn count[T, N](r: Ring[T, N]) u32 {
    return N * 2
}

fn main() void {
    var ps: Pair[u32, i32][2] = [Pair[u32, i32]{ 1, -1 }, Pair[u32, i32]{ 2, -2 }]
    ps[1].a = 20
    print(int_to_str(ps[0].a + ps[1].a) + " " + i64_to_str(i64(ps[0].b + ps[1].b)) + " " + int_to_str(len(ps)))
    var qs: Pair[u32, u32][3] = [Pair[u32, u32]{ 5, 0 }, Pair[u32, u32]{ 0, 0 }, Pair[u32, u32]{ 0, 7 }]
    let r: Ring[u32, 3] = Ring[u32, 3]{ Ptr[Pair[u32, u32]](&qs[0]) }
    print(" " + int_to_str(r.items[0].a + last_b(r)) + " " + int_to_str(count(r)))
}
`,
		want: "21 -3 2 12 6",
	},
}
