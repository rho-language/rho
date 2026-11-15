//
// @file arena_test.go
// @brief Where the C backend releases strings, and which loops it cannot.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"strings"
	"testing"

	"rho/src/ir"
)

// lowerArena checks and lowers a program, and returns its IR and the loops it
// reports.
func lowerArena(t *testing.T, source string) (ir.ModuleIR, []ArenaEscape) {
	t.Helper()
	module := parseSource(t, source)
	tc := New()
	if err := tc.CheckModule(*module); err != nil {
		t.Fatalf("check: %v", err)
	}
	lowered := tc.ToIR(*module)
	return lowered, tc.ArenaEscapes()
}

// arenaOps counts the arena's instructions in a body, at any depth, and the
// instructions that carry a mark of their own.
func arenaOps(body []ir.Instr) (marks, releases, carried int) {
	for _, instr := range body {
		switch instr.Op {
		case ir.OpArenaMark:
			marks++
		case ir.OpArenaRelease:
			releases++
		}
		if instr.Arena != "" {
			carried++
		}
		m, r, c := arenaOps(instr.Then)
		marks, releases, carried = marks+m, releases+r, carried+c
		m, r, c = arenaOps(instr.Else)
		marks, releases, carried = marks+m, releases+r, carried+c
	}
	return marks, releases, carried
}

// A loop that builds no string is lowered as it always was: a firmware that
// toggles a pin carries no arena.
func TestALoopThatBuildsNoStringHasNoArena(t *testing.T) {
	module, escapes := lowerArena(t, `module main

extern fn toggle(pin: u32) void

fn main() void {
    var i: u32 = 0
    while true {
        toggle(13)
        print("tick\n")
        print(int_to_str(i))
        i = i + 1
    }
}
`)
	if marks, releases, carried := arenaOps(findFunc(t, module, "main").Body); marks+releases+carried != 0 {
		t.Errorf("expected no arena instructions, got %d marks, %d releases, %d carried", marks, releases, carried)
	}
	if len(escapes) != 0 {
		t.Errorf("expected nothing reported, got %v", escapes)
	}
}

// A loop that builds strings and keeps none outside itself is released every
// iteration and after it, and before a `continue`.
func TestALoopThatKeepsNothingIsReleasedEachIteration(t *testing.T) {
	module, escapes := lowerArena(t, `module main

fn main() void {
    var i: u32 = 0
    var state: string = "idle"
    while i < 10 {
        let line = "n=" + int_to_str(i)
        i = i + 1
        if i == 3 {
            state = "three"
            continue
        }
        print(line + state)
    }
}
`)
	body := findFunc(t, module, "main").Body
	var loop ir.Instr
	loopAt := -1
	for index, instr := range body {
		if instr.Op == ir.OpWhile {
			loop, loopAt = instr, index
		}
	}
	if loopAt < 1 || body[loopAt-1].Op != ir.OpArenaMark || loopAt+1 >= len(body) || body[loopAt+1].Op != ir.OpArenaRelease {
		t.Fatalf("expected a mark before the loop and a release after it: %+v", body)
	}
	mark := body[loopAt-1].Dest
	if last := loop.Then[len(loop.Then)-1]; last.Op != ir.OpArenaRelease || last.Dest != mark {
		t.Errorf("expected the body to end by releasing to %s, got %+v", mark, last)
	}
	// The continue releases first.
	found := false
	for _, instr := range loop.Then {
		if instr.Op == ir.OpIf {
			for index, inner := range instr.Then {
				if inner.Op == ir.OpContinue && index > 0 && instr.Then[index-1].Op == ir.OpArenaRelease && instr.Then[index-1].Dest == mark {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected the continue to release to the loop's mark first")
	}
	// The kept line carries a statement mark of its own.
	if _, _, carried := arenaOps(loop.Then); carried == 0 {
		t.Error("expected the let to keep its string against a statement mark")
	}
	if len(escapes) != 0 {
		t.Errorf("a literal stored outside the loop keeps nothing alive, got %v", escapes)
	}
}

// A loop that replaces a string in a variable declared outside it is released
// every iteration except for that string: the release at the end of the body,
// before a `continue` and after the loop all keep it, and nothing is reported.
func TestALoopThatReplacesAStringOutsideItKeepsIt(t *testing.T) {
	module, escapes := lowerArena(t, `module main

fn main() void {
    var last: string = ""
    var i: u32 = 0
    while true {
        last = "t=" + int_to_str(i)
        i = i + 1
        if i % 3 == 0 {
            continue
        }
        print(last)
    }
}
`)
	if len(escapes) != 0 {
		t.Fatalf("a replaced string is kept, not reported: %+v", escapes)
	}
	body := findFunc(t, module, "main").Body
	keeps := 0
	var visit func([]ir.Instr)
	visit = func(instrs []ir.Instr) {
		for _, instr := range instrs {
			if instr.Op == ir.OpArenaRelease {
				if len(instr.Args) != 1 || instr.Args[0] != "(void*)&(last)" {
					t.Errorf("expected every release of the loop to keep last, got %+v", instr)
				}
				keeps++
			}
			visit(instr.Then)
			visit(instr.Else)
		}
	}
	visit(body)
	if keeps != 3 {
		t.Errorf("expected three releases that keep last (end of body, continue, after), got %d", keeps)
	}
}

// A variable that holds what a kept variable holds keeps it too, and a struct
// of strings replaced as a whole keeps each of its strings.
func TestAValueCopiedOutOfAKeptVariableIsKeptAndAStructKeepsItsFields(t *testing.T) {
	module, escapes := lowerArena(t, `module main

struct Status {
    text: string
    code: u32
    detail: string
}

fn main() void {
    var last: string = ""
    var previous: string = ""
    var status = Status{ "", 0, "" }
    var i: u32 = 0
    while i < 10 {
        previous = last
        last = "t=" + int_to_str(i)
        status = Status{ last, i, "d" + int_to_str(i) }
        i = i + 1
    }
    print(previous + last + status.text + status.detail)
}
`)
	if len(escapes) != 0 {
		t.Fatalf("expected nothing reported, got %+v", escapes)
	}
	var keep []string
	for _, instr := range findFunc(t, module, "main").Body {
		if instr.Op == ir.OpArenaRelease && keep == nil {
			keep = instr.Args
		}
	}
	got := strings.Join(keep, " ")
	for _, want := range []string{"&(last)", "&(previous)", "&((status).text)", "&((status).detail)"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected the loop to keep %s, it keeps %s", want, got)
		}
	}
	if strings.Contains(got, "code") {
		t.Errorf("a number is not kept: %s", got)
	}
}

// A string stored where no name reaches it as a whole — an element of a Slice
// over storage declared outside the loop — stops the loop being released, and the loop is
// reported with the variable. So does a function value replaced: its
// environment is not a string the runtime can measure.
func TestALoopThatKeepsAStringOutsideItIsReported(t *testing.T) {
	module, escapes := lowerArena(t, `module main

fn main() void {
    var store: string[4]
    var history = store[..]
    while true {
        history[0] = "t=" + int_to_str(7)
        print(history[0])
    }
}
`)
	body := findFunc(t, module, "main").Body
	for _, instr := range body {
		if instr.Op == ir.OpArenaMark {
			t.Errorf("a loop that keeps a string outside it must not be released, got %+v", body)
		}
	}
	if len(escapes) != 1 || escapes[0].Variable != "history" || escapes[0].Line != 6 {
		t.Fatalf("expected the loop at line 6 reported for history, got %+v", escapes)
	}
	if !strings.Contains(escapes[0].String(), `"history"`) {
		t.Errorf("the report should name the variable: %s", escapes[0])
	}

	for name, source := range map[string]string{
		"a function value": `module main

fn main() void {
    var f: fn() -> u32 = fn() -> u32 { return 0 }
    var i: u32 = 0
    while true {
        let s = "t=" + int_to_str(i)
        f = fn() -> u32 { return strlen(s) }
        i = i + 1
    }
}
`,
	} {
		_, escapes := lowerArena(t, source)
		if len(escapes) != 1 {
			t.Errorf("%s replaced in a loop is not kept and should be reported, got %+v", name, escapes)
		}
	}
}

// A string kept in a variable an enclosing loop declares lives until that
// loop's iteration ends, and that loop is released: nothing is reported, the
// outer loop is reset and the inner one is not.
func TestAnInnerLoopKeepingIntoTheOuterLoopsVariableIsNotReported(t *testing.T) {
	module, escapes := lowerArena(t, `module main

fn main() void {
    while true {
        var row: string = ""
        var j: u32 = 0
        while j < 4 {
            row = row + int_to_str(j)
            j = j + 1
        }
        print(row)
    }
}
`)
	if len(escapes) != 0 {
		t.Fatalf("expected nothing reported, got %+v", escapes)
	}
	body := findFunc(t, module, "main").Body
	if len(body) == 0 || body[0].Op != ir.OpArenaMark {
		t.Fatalf("expected the outer loop to be released, got %+v", body)
	}
}

// Taking the address of a value declared outside the loop that holds a string
// lets a callee store one there.
func TestAnAddressOfAnOuterValueStopsTheLoopBeingReleased(t *testing.T) {
	_, escapes := lowerArena(t, `module main

struct Reading {
    name: string
    value: u32
}

fn fill(r: Ptr[Reading], i: u32) void {
}

fn main() void {
    var reading = Reading{ "none", 0 }
    var i: u32 = 0
    while true {
        fill(&reading, i)
        print("x" + int_to_str(i))
        i = i + 1
    }
}
`)
	if len(escapes) != 1 || escapes[0].Variable != "reading" {
		t.Fatalf("expected the loop reported for reading, got %+v", escapes)
	}
}

// A bounded loop that builds a string a function returns is not reported when
// the caller keeps the result: the statement that keeps it releases the rest.
// The same loop that can never end is.
func TestABuilderLoopIsReportedOnlyWhenItsGarbageIsNeverReleased(t *testing.T) {
	_, escapes := lowerArena(t, `module main

fn joined(count: u32) string {
    var out: string = ""
    var k: u32 = 0
    while k < count {
        out = out + int_to_str(k)
        k = k + 1
    }
    return out
}

fn forever() void {
    var store: string[2]
    var seen = store[..]
    while true {
        seen[1] = "x" + int_to_str(1)
    }
}

fn unused() void {
    var store: string[2]
    var seen = store[..]
    while true {
        seen[1] = "x" + int_to_str(1)
    }
}

fn main() void {
    let all = joined(4)
    print(all)
    forever()
}
`)
	if len(escapes) != 1 || escapes[0].Function != "forever" {
		t.Fatalf("expected only forever's loop reported, got %+v", escapes)
	}
}

// A `for` loop is bounded by its array, and is not reported.
func TestAForLoopIsNotReported(t *testing.T) {
	_, escapes := lowerArena(t, `module main

fn main() void {
    let values: u32[3] = [1, 2, 3]
    var out: string = ""
    for value in values {
        out = out + int_to_str(value)
    }
    print(out)
}
`)
	if len(escapes) != 0 {
		t.Fatalf("expected nothing reported, got %+v", escapes)
	}
}

// `print(int_to_str(n))` and `print(i64_to_str(n))` print the number without
// building the string.
func TestPrintingANumberBuildsNoString(t *testing.T) {
	module, _ := lowerArena(t, `module main

fn main() void {
    let a: u32 = 7
    let b: i64 = -7
    print(int_to_str(a))
    print(i64_to_str(b))
}
`)
	text := ""
	for _, instr := range findFunc(t, module, "main").Body {
		text += strings.Join(instr.Args, " ") + "\n"
	}
	if !strings.Contains(text, "rho_print_u32(a)") || !strings.Contains(text, "rho_print_i64(b)") {
		t.Errorf("expected the numbers printed directly, got:\n%s", text)
	}
	if strings.Contains(text, "to_str") {
		t.Errorf("expected no string built, got:\n%s", text)
	}
}

// An enum replaced in a loop keeps the strings of whichever variant it holds:
// each payload's slot is guarded by the tag.
func TestAnEnumReplacedInALoopKeepsItsVariantsStrings(t *testing.T) {
	module, escapes := lowerArena(t, `module main

enum Note {
    Text(string),
    Pair { a: string, b: u32 },
    Empty,
}

fn main() void {
    var last = Note.Empty
    var i: u32 = 0
    while true {
        if i % 2 == 0 {
            last = Note.Text("t=" + int_to_str(i))
        } else {
            last = Note.Pair { "p" + int_to_str(i), i }
        }
        i = i + 1
    }
}
`)
	if len(escapes) != 0 {
		t.Fatalf("expected nothing reported, got %+v", escapes)
	}
	var keep []string
	for _, instr := range findFunc(t, module, "main").Body {
		if instr.Op == ir.OpArenaRelease {
			keep = instr.Args
		}
	}
	if len(keep) != 2 || !strings.Contains(keep[0], "(last).tag == 0 ?") || !strings.Contains(keep[1], "(last).tag == 1 ?") {
		t.Fatalf("expected a guarded slot per string payload, got %v", keep)
	}
}

// A fixed array declared outside the loop is storage of its own: a loop that
// replaces its elements keeps every element's string, up to the most a loop
// keeps. One larger than that is not released, and is reported.
func TestAFixedArrayOutsideTheLoopKeepsItsElements(t *testing.T) {
	module, escapes := lowerArena(t, `module main

fn main() void {
    var history: string[4]
    var i: u32 = 0
    while true {
        history[i % 4] = "t=" + int_to_str(i)
        i = i + 1
    }
}
`)
	if len(escapes) != 0 {
		t.Fatalf("expected nothing reported, got %+v", escapes)
	}
	var keep []string
	for _, instr := range findFunc(t, module, "main").Body {
		if instr.Op == ir.OpArenaRelease {
			keep = instr.Args
		}
	}
	if len(keep) != 4 || keep[3] != "(void*)&((history)[3])" {
		t.Fatalf("expected each element kept, got %v", keep)
	}

	_, escapes = lowerArena(t, `module main

fn main() void {
    var history: string[64]
    var i: u32 = 0
    while true {
        history[i % 64] = "t=" + int_to_str(i)
        i = i + 1
    }
}
`)
	if len(escapes) != 1 || escapes[0].Variable != "history" {
		t.Fatalf("an array of more strings than a loop keeps is reported, got %+v", escapes)
	}
}

// A value a loop binds once from what was there before it — `let key = s[i]` —
// is as old as what it was read from: storing it back keeps nothing alive, so
// sorting strings in place builds none and is released.
func TestALetOfAnOlderValueIsOlder(t *testing.T) {
	_, escapes := lowerArena(t, `module main

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
        print("moved " + key)
        i = i + 1
    }
}

fn reassigned(s: Slice[string]) void {
    var i: u32 = 0
    while true {
        let key = s[0]
        key = "k" + int_to_str(i)
        s[1] = key
        i = i + 1
    }
}

fn main() void {
    var names: string[3] = ["b", "a", "c"]
    sort_names(names[..])
    reassigned(names[..])
}
`)
	if len(escapes) != 1 || escapes[0].Function != "reassigned" {
		t.Fatalf("expected only the loop that stores a reassigned let reported, got %+v", escapes)
	}
}

// A loop that builds no string keeps none, whatever it stores outside itself;
// and a struct of an older string and numbers worked out in the iteration is
// an older value.
func TestALoopThatBuildsNoStringIsNotReportedForWhatItStores(t *testing.T) {
	_, escapes := lowerArena(t, `module main

struct Group {
    key: string
    count: u32
}

fn add(groups: Slice[Group], used: u32, key: string) u32 {
    var i: u32 = 0
    while i < used {
        if groups[i].key == key {
            groups[i] = Group{ key, groups[i].count + 1 }
            return used
        }
        i = i + 1
    }
    return used
}

fn spin(groups: Slice[Group], key: string) void {
    while true {
        groups[0] = Group{ key, groups[0].count * 2 }
        print("tick" + int_to_str(groups[0].count))
    }
}

fn main() void {
    var store: Group[2]
    let used = add(store[..], 1, "k")
    print(int_to_str(used))
    spin(store[..], "k")
}
`)
	if len(escapes) != 0 {
		t.Fatalf("expected nothing reported, got %+v", escapes)
	}
}
