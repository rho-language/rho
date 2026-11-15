package types

import (
	"strings"
	"testing"

	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
)

// warningsOf checks a module and returns what it warned about, each as
// "line:col: message".
func warningsOf(t *testing.T, source string) []string {
	t.Helper()
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatal(err)
	}
	module, err := parser.Parse(tokens)
	if err != nil {
		t.Fatal(err)
	}
	tc := New()
	if err := tc.CheckModule(*module); err != nil {
		t.Fatalf("refused: %v", err)
	}
	out := []string{}
	for _, warning := range tc.Warnings() {
		out = append(out, warning.Position.String()+": "+warning.Message)
	}
	return out
}

// FINDINGS #7: a `let` binds a name that does not change. Assigning to one is
// a deprecation warning this release, at the target, naming the fix.
func TestAssigningToALetIsADeprecationWarning(t *testing.T) {
	got := warningsOf(t, `module app

fn main() void {
    let i: u32 = 0
    var j: u32 = 0
    i = i + 1
    j = j + 1
    while j < 3 {
        i += 2
        j = j + 1
    }
    let f: fn() -> u32 = fn() -> u32 {
        let k: u32 = 1
        k = 2
        return k
    }
    print(int_to_str(i + f()))
}
`)
	want := []string{"6:5:", "9:9:", "14:9:"}
	if len(got) != len(want) {
		t.Fatalf("warnings: %q, want %d at %v", got, len(want), want)
	}
	for i, at := range want {
		if !strings.HasPrefix(got[i], at) || !strings.Contains(got[i], "bound by `let`, is deprecated") ||
			!strings.Contains(got[i], "with `var`") {
			t.Errorf("warning %d is %q, want one at %s naming `var`", i, got[i], at)
		}
	}
}

// What a `let` binds is the name: a field, an element and the address of the
// value it holds are not the name, and a name a `var`, a parameter, a loop or
// a pattern binds may change.
func TestWritingThroughALetIsNotAssigningToIt(t *testing.T) {
	got := warningsOf(t, `module app

struct P { x: u32 }

fn bump(n: u32) u32 {
    n = n + 1
    return n
}

fn main() void {
    let p: P = P { 1 }
    p.x = 2
    let a: u32[3] = [1, 2, 3]
    a[0] = 4
    let q: Ptr[u32] = &a[1]
    var total: u32 = 0
    for v in a {
        v = v + 1
        total = total + v
    }
    if true {
        var i: u32 = 0
        i = 1
        total = total + i
    }
    print(int_to_str(p.x + total + bump(1) + q[0]))
}
`)
	if len(got) != 0 {
		t.Fatalf("warned: %q", got)
	}
}
