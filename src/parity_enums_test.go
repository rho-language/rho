//
// @file parity_enums_test.go
// @brief Enums on both backends: plain ones, ones that carry data, generic ones.
//
// An enum is a tagged union in C and a struct of variants in Go, built and taken
// apart by the same helpers, so a program must print the same on both. The cases
// cover every kind of value a variant carries — a number, a string, a struct, a
// plain enum, another enum — strings built in a loop and kept or released by the
// C arena, generic enums, Option and Result as the enums they now are, and the
// checked conversion of a number into a plain enum, which stops the program the
// same way on both when the number is none of its values.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

func init() {
	parityCases = append(parityCases, enumParityCases...)
}

var enumParityCases = []parityCase{
	{
		name: "enums: a plain enum is its numbers, written, counted and negative",
		source: `module app

enum Level { Low = -2, Mid, High = 10, Top }

enum Mode { Off, On }

fn name(l: Level) string {
    match l {
        Level.Low => { return "low" }
        Level.Mid => { return "mid" }
        Level.High => { return "high" }
        Top => { return "top" }
    }
}

fn main() void {
    let levels: Level[4] = [Level.Low, Level.Mid, Level.High, Level.Top]
    for l in levels {
        print(name(l) + "=" + i64_to_str(i64(l)) + " ")
    }
    let back: Level = Level(11)
    if back == Level.Top && back != Level.Low {
        print("back ")
    }
    var m = Mode.Off
    if m == Mode.Off {
        m = Mode.On
    }
    print(int_to_str(u32(m)) + " " + name(Level(-1)))
}
`,
		want: "low=-2 mid=-1 high=10 top=11 back 1 mid",
	},
	{
		name: "enums: every kind of value a variant carries",
		source: `module app

struct Point { x: i32, y: i32 }

enum Color { Red = 1, Green = 2 }

enum Value {
    Count(u32)
    Signed(i64)
    Ratio(f64)
    Flag(bool)
    Text(string)
    At(Point)
    Paint(Color)
    Pair(u8, string)
    Box { w: u16, h: u16 }
    Nothing
}

fn show(v: Value) string {
    match v {
        Value.Count(n) => { return "count " + int_to_str(n) }
        Value.Signed(n) => { return "signed " + i64_to_str(n) }
        Value.Ratio(r) => { return "ratio " + int_to_str(u32(r * 100.0)) }
        Value.Flag(true) => { return "flag on" }
        Value.Flag(false) => { return "flag off" }
        Value.Text("") => { return "empty text" }
        Value.Text(s) => { return "text " + s }
        Value.At(p) => { return "at " + i64_to_str(i64(p.x)) + "," + i64_to_str(i64(p.y)) }
        Value.Paint(Color.Red) => { return "red" }
        Value.Paint(c) => { return "paint " + int_to_str(u32(c)) }
        Value.Pair(n, s) => { return "pair " + int_to_str(u32(n)) + s }
        Value.Box { w, h } => { return "box " + int_to_str(u32(w) * u32(h)) }
        Value.Nothing => { return "nothing" }
    }
}

fn main() void {
    let values: Value[13] = [
        Value.Count(7), Value.Signed(-9), Value.Ratio(0.25), Value.Flag(true), Value.Flag(false),
        Value.Text(""), Value.Text("hi"), Value.At(Point{ 3, -4 }), Value.Paint(Color.Red),
        Value.Paint(Color.Green), Value.Pair(2, "x"), Value.Box { 3, 5 }, Value.Nothing
    ]
    for v in values {
        print(show(v) + "; ")
    }
}
`,
		want: "count 7; signed -9; ratio 25; flag on; flag off; empty text; text hi; at 3,-4; red; paint 2; pair 2x; box 15; nothing; ",
	},
	{
		name: "enums: strings built in a loop, released or kept by the arena",
		source: `module app

enum Event {
    Reading { sensor: string, value: i32 }
    Fault(string)
    Idle
}

fn make(i: u32) Event {
    if i % 3 == 0 {
        return Event.Reading { "t" + int_to_str(i), i32(i) * 10 }
    }
    if i % 3 == 1 {
        return Event.Fault("code " + int_to_str(i))
    }
    return Event.Idle
}

fn label(e: Event) string {
    match e {
        Event.Reading { sensor, value } => { return sensor + "=" + i64_to_str(i64(value)) }
        Event.Fault(why) => { return "fault " + why }
        Event.Idle => { return "idle" }
    }
}

fn main() void {
    var i: u32 = 0
    while i < 200 {
        let e = make(i)
        if i % 50 == 0 {
            print(label(e) + " ")
        }
        i = i + 1
    }
    var last = Event.Idle
    var kept = ""
    var j: u32 = 0
    while j < 7 {
        last = make(j)
        match make(j + 1) {
            Event.Fault(why) => { kept = why }
            _ => {}
        }
        j = j + 1
    }
    print("| " + label(last) + " | " + kept)
}
`,
		want: "t0=0 idle fault code 100 t150=1500 | t6=60 | code 7",
	},
	{
		name: "enums: generic enums and enums inside enums",
		source: `module app

enum Pair[T] {
    Both(T, T)
    One(T)
    Neither
}

enum Shape {
    Circle(u32)
    Square(u32)
}

enum Slot {
    Filled(Pair[Shape])
    Empty
}

fn size(s: Shape) u32 {
    match s {
        Circle(r) => { return 3 * r * r }
        Square(a) => { return a * a }
    }
}

fn total(slot: Slot) u32 {
    match slot {
        Slot.Filled(Pair.Both(a, b)) => { return size(a) + size(b) }
        Slot.Filled(Pair.One(Shape.Circle(r))) => { return 1000 + r }
        Slot.Filled(Pair.One(s)) => { return size(s) }
        Slot.Filled(Pair.Neither) => { return 1 }
        Slot.Empty => { return 0 }
    }
}

fn first(p: Pair[string]) string {
    match p {
        Pair.Both(a, _) => { return a }
        Pair.One(a) => { return a }
        Pair.Neither => { return "-" }
    }
}

fn main() void {
    let slots: Slot[5] = [
        Slot.Filled(Pair.Both(Shape.Circle(1), Shape.Square(2))),
        Slot.Filled(Pair.One(Shape.Circle(5))),
        Slot.Filled(Pair.One(Shape.Square(3))),
        Slot.Filled(Pair[Shape].Neither),
        Slot.Empty
    ]
    for s in slots {
        print(int_to_str(total(s)) + " ")
    }
    let p: Pair[u32] = Pair.Both(4, 5)
    if let Pair.Both(a, b) = p {
        print(int_to_str(a * b) + " ")
    }
    print(first(Pair.Both("a" + "b", "c")) + first(Pair[string].Neither))
}
`,
		want: "7 1005 9 1 0 20 ab-",
	},
	{
		name: "enums: a value is copied whole, in a struct, an array and a call",
		source: `module app

enum Light {
    Off
    Dim(u8)
    Named(string)
}

struct Room { id: u32, light: Light }

fn brighter(l: Light) Light {
    match l {
        Light.Dim(n) => { return Light.Dim(n + 1) }
        _ => { return l }
    }
}

fn show(l: Light) string {
    match l {
        Light.Off => { return "off" }
        Light.Dim(n) => { return "dim" + int_to_str(u32(n)) }
        Light.Named(s) => { return s }
    }
}

fn main() void {
    var room = Room{ 1, Light.Dim(3) }
    let before = room
    room.light = brighter(room.light)
    var lights: Light[2] = [Light.Off, Light.Named("lamp")]
    let copy = lights[1]
    lights[1] = Light.Dim(9)
    print(show(before.light) + " " + show(room.light) + " " + show(copy) + " " + show(lights[1]) + " " + show(lights[0]))
}
`,
		want: "dim3 dim4 lamp dim9 off",
	},
	{
		name: "enums: Option and Result are enums, and read as their structs did",
		source: `module app

fn find(values: [u32], count: u32, wanted: u32) Option[u32] {
    var i: u32 = 0
    while i < count {
        if values[i] == wanted {
            return Some(i)
        }
        i = i + 1
    }
    return None[u32]()
}

fn parse(text: string) Result[u32, string] {
    if text == "" {
        return Err[u32, string]("empty")
    }
    return Result.Ok(u32(strlen(text)))
}

type MaybeCount = Option[u32]
type Outcome = Result[u32, u32]

fn main() void {
    let values: u32[3] = [10, 20, 30]
    let hit = find(values, 3, 20)
    if hit.is_some {
        print("at " + int_to_str(hit.value) + " ")
    }
    match find(values, 3, 99) {
        Some(i) => print("unexpected " + int_to_str(i))
        None => print("missing ")
    }
    let r = parse("hello")
    print(int_to_str(r.ok) + (if r.is_ok { " ok " } else { " no " }))
    let e = parse("")
    print("[" + e.err + "] " + int_to_str(e.ok) + " ")
    match e {
        Ok(n) => print(int_to_str(n))
        Err(message) => print("failed " + message + " ")
    }
    let literal: Result[u32, string] = Result[u32, string]{ true, 5, "" }
    let refused: Result[u32, string] = Result[u32, string]{ false, 0, "no" }
    print(int_to_str(literal.ok) + refused.err + " ")
    let none: Option[string] = Option.None
    print("[" + none.value + "] ")
    let zero: Option[u32] = default(MaybeCount)
    let failed: Result[u32, u32] = default(Outcome)
    if !zero.is_some && !failed.is_ok {
        print("zero is none and err ")
    }
    let nested: Option[Option[u32]] = Option.Some(Option.Some(4))
    if let Some(Some(x)) = nested {
        print("nested " + int_to_str(x))
    }
}
`,
		want: "at 1 missing 5 ok [empty] 0 failed empty 5no [] zero is none and err nested 4",
	},
	// Option is loaded from std only when a program names it, and a program may
	// name it only inside another application or by a variant.
	{
		name: "enums: Option named only inside an application, and enum constants",
		source: `module app

struct Box[T] {
    item: T
    count: u32
}

enum Mode { Off = 1, On = 2 }

const DEFAULT_MODE: Mode = Mode.On

fn main() void {
    let b = Box[Option[u32]]{ Some[u32](7), 1 }
    match b.item {
        Some(n) => print(int_to_str(n) + " ")
        None => print("none ")
    }
    let empty = Option[string].None
    if !empty.is_some && DEFAULT_MODE == Mode.On {
        print(int_to_str(u32(DEFAULT_MODE)))
    }
}
`,
		want: "7 2",
	},
	{
		name: "enums: a number that is no value of a plain enum stops the program",
		source: `module app

enum Mode { Off = 1, On = 2 }

fn main() void {
    var n: u32 = 1
    let ok = Mode(n)
    print("ok=" + int_to_str(u32(ok)) + " ")
    n = 5
    let bad = Mode(n)
    print("after " + int_to_str(u32(bad)))
}
`,
		wantFailure: true,
		want:        "ok=1 ",
	},
}
