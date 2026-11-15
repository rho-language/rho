//
// @file parity_arena_test.go
// @brief Loops that keep their latest strings in variables declared outside them.
//
// A loop that replaces a string held outside it — the last reading, the status
// text, an accumulator — is released every iteration except for the strings
// those variables hold, which are moved down to the loop's mark
// (rho_arena_keep_many). C builds each program with AddressSanitizer, which
// poisons what is released: a kept string that was not moved, or a variable
// left pointing at where its string used to be, is a report and not a value
// that happens to still be there. Each loop runs long enough to reuse what it
// released many times over.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

func init() {
	parityCases = append(parityCases, arenaKeepParityCases...)
}

var arenaKeepParityCases = []parityCase{
	{
		name: "arena: the last value of a long loop, kept in a variable declared outside it",
		want: "node4=399dC | node3=398dC | 99999\n",
		source: `module app

fn describe(t: u32, label: string) string {
    return label + "=" + int_to_str(t) + "dC"
}

fn main() void {
    var last: string = ""
    var previous: string = ""
    var n: u32 = 0
    while n < 100000 {
        let line = describe(n % 600, "node" + int_to_str(n % 5))
        previous = last
        last = line
        n = n + 1
    }
    print(last + " | " + previous + " | " + int_to_str(n - 1) + "\n")
}
`,
	},
	{
		name: "arena: two variables holding one string, one holding its tail, then parted",
		want: "b-29999 b-29999 29999 | b-29998\n",
		source: `module app

fn main() void {
    var first: string = ""
    var second: string = ""
    var tail: string = ""
    var older: string = ""
    var i: u32 = 0
    while i < 30000 {
        let line = "b-" + int_to_str(i)
        older = first
        first = line
        second = line
        tail = str_trim_prefix(line, "b-")
        if i % 2 == 0 {
            older = "a-" + int_to_str(i)
        }
        i = i + 1
    }
    print(first + " " + second + " " + tail + " | " + older + "\n")
}
`,
	},
	{
		name: "arena: an accumulator grows by what it keeps and nothing else",
		want: "70000 0123456789 9 3456\n",
		source: `module app

fn main() void {
    var acc: string = ""
    var i: u32 = 0
    while i < 14000 {
        let piece = int_to_str((i * 5) % 10) + int_to_str((i * 5 + 1) % 10) + "" + int_to_str((i * 5 + 2) % 10) + int_to_str((i * 5 + 3) % 10) + int_to_str((i * 5 + 4) % 10)
        acc = acc + piece
        i = i + 1
    }
    print(int_to_str(strlen(acc)) + " " + str_slice(acc, 0, 10) + " " + str_slice(acc, 69999, 1) + " " + str_slice(acc, 3, 4) + "\n")
}
`,
	},
	{
		name: "arena: a kept string survives break, continue, a false condition and a return from inside the loop",
		want: "broke at k7 | skipped to k9 | ended at k4 | found k12\n",
		source: `module app

fn label(i: u32) string {
    return "k" + int_to_str(i)
}

fn find(target: u32) string {
    var last: string = "none"
    var i: u32 = 0
    while i < 1000 {
        last = label(i)
        if i == target {
            return "found " + last
        }
        i = i + 1
    }
    return last
}

fn main() void {
    var last: string = ""
    var i: u32 = 0
    while true {
        last = label(i)
        if i == 7 {
            break
        }
        i = i + 1
    }
    print("broke at " + last + " | ")

    var kept: string = ""
    var j: u32 = 0
    while j < 10 {
        j = j + 1
        if j % 2 == 0 {
            continue
        }
        kept = label(j)
        if j > 8 {
            continue
        }
        let noise = "noise" + label(j)
        print(str_slice(noise, 0, 0))
    }
    print("skipped to " + kept + " | ")

    var ended: string = ""
    var k: u32 = 0
    while k < 5 {
        ended = label(k) + ""
        k = k + 1
    }
    print("ended at " + ended + " | ")
    print(find(12) + "\n")
}
`,
	},
	{
		name: "arena: nested loops, each keeping its own variables and the outer one's",
		want: "row9=0,1,2,3 | best=row9:3 | 40 | rows=10\n",
		source: `module app

fn main() void {
    var best: string = ""
    var lastRow: string = ""
    var cells: u32 = 0
    var r: u32 = 0
    while r < 10 {
        var row: string = "row" + int_to_str(r) + "="
        var c: u32 = 0
        while c < 4 {
            if c > 0 {
                row = row + ","
            }
            row = row + int_to_str(c)
            best = "row" + int_to_str(r) + ":" + int_to_str(c)
            cells = cells + 1
            var inner: u32 = 0
            while inner < 3 {
                let scratch = "s" + int_to_str(inner)
                if inner == 2 {
                    break
                }
                print(str_slice(scratch, 0, 0))
                inner = inner + 1
            }
            c = c + 1
        }
        lastRow = row
        r = r + 1
    }
    print(lastRow + " | best=" + best + " | " + int_to_str(cells) + " | rows=" + int_to_str(r) + "\n")
}
`,
	},
	{
		name: "arena: a struct of strings replaced every iteration keeps each of them",
		want: "s4999 4999 d4999 | s4998\n",
		source: `module app

struct Status {
    text: string
    code: u32
    detail: string
}

fn status(i: u32) Status {
    return Status{ "s" + int_to_str(i), i, "d" + int_to_str(i) }
}

fn main() void {
    var now = Status{ "", 0, "" }
    var before: string = ""
    var i: u32 = 0
    while i < 5000 {
        before = now.text
        now = status(i)
        i = i + 1
    }
    print(now.text + " " + int_to_str(now.code) + " " + now.detail + " | " + before + "\n")
}
`,
	},
	{
		name: "arena: a field of an outer struct replaced, and a for loop keeping a running total",
		want: "x=9 | 0123456789\n",
		source: `module app

struct Holder {
    name: string
    count: u32
}

fn main() void {
    var h = Holder{ "", 0 }
    var i: u32 = 0
    while i < 10 {
        h.name = "x=" + int_to_str(i)
        h.count = h.count + 1
        i = i + 1
    }
    let values: u32[10] = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9]
    var digits: string = ""
    for value in values {
        digits = digits + int_to_str(value)
    }
    print(h.name + " | " + digits + "\n")
}
`,
	},
	{
		name: "arena: an enum and an Option replaced every iteration keep whichever variant they hold",
		want: "t29997=299970 | fault code 29998 | e29995 | none\n",
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
    var last = Event.Idle
    var reading = Event.Idle
    var fault = Event.Idle
    var err: Option[string] = Option.None
    var cleared: Option[string] = Option.None
    var i: u32 = 0
    while i < 30000 {
        last = make(i)
        match last {
            Event.Reading { sensor, value } => { reading = last }
            Event.Fault(why) => { fault = last }
            _ => {}
        }
        if i % 5 == 0 {
            err = Option.Some("e" + int_to_str(i))
            cleared = Option.Some("c" + int_to_str(i))
        }
        if i % 5 == 1 {
            cleared = Option.None
        }
        i = i + 1
    }
    var c = "none"
    if cleared.is_some {
        c = cleared.value
    }
    print(label(reading) + " | " + label(fault) + " | " + err.value + " | " + c + "\n")
}
`,
	},
	{
		name: "arena: a ring of the last readings in a fixed array, and sorting strings in place",
		want: "r49996 r49997 r49998 r49999 | a b c d\n",
		source: `module app

fn sort_names(s: Slice[string]) void {
    var i: u32 = 1
    while i < s.len {
        let key = s[i]
        var j: u32 = i
        while j > 0 && key < s[j - 1] {
            s[j] = s[j - 1]
            j = j - 1
        }
        s[j] = key
        i = i + 1
    }
}

fn main() void {
    var ring: string[4]
    var i: u32 = 0
    var slot: u32 = 0
    while i < 50000 {
        ring[slot] = "r" + int_to_str(i)
        slot = slot + 1
        if slot == 4 {
            slot = 0
        }
        i = i + 1
    }
    var names: string[4] = ["d", "b", "c", "a"]
    sort_names(names[..])
    print(ring[0] + " " + ring[1] + " " + ring[2] + " " + ring[3] + " | " + names[0] + " " + names[1] + " " + names[2] + " " + names[3] + "\n")
}
`,
	},
}
