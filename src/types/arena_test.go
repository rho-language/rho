//
// @file arena_test.go
// @brief Where the C backend releases strings, and which loops it cannot.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
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

// A string stored outside the loop stops the loop being released, and the loop
// is reported with the variable.
func TestALoopThatKeepsAStringOutsideItIsReported(t *testing.T) {
	module, escapes := lowerArena(t, `module main

fn main() void {
    var last: string = ""
    while true {
        last = "t=" + int_to_str(7)
        print(last)
    }
}
`)
	body := findFunc(t, module, "main").Body
	for _, instr := range body {
		if instr.Op == ir.OpArenaMark {
			t.Errorf("a loop that keeps a string outside it must not be released, got %+v", body)
		}
	}
	if len(escapes) != 1 || escapes[0].Variable != "last" || escapes[0].Line != 5 {
		t.Fatalf("expected the loop at line 5 reported for last, got %+v", escapes)
	}
	if !strings.Contains(escapes[0].String(), `"last"`) {
		t.Errorf("the report should name the variable: %s", escapes[0])
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
    var last: string = ""
    while true {
        last = "x" + int_to_str(1)
    }
}

fn unused() void {
    var last: string = ""
    while true {
        last = "x" + int_to_str(1)
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
