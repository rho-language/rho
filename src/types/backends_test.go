package types

import (
	"strings"
	"testing"

	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
)

func checkSource(t *testing.T, source string) error {
	t.Helper()
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatal(err)
	}
	module, err := parser.Parse(tokens)
	if err != nil {
		t.Fatal(err)
	}
	return New().CheckModule(*module)
}

// A primitive is the compiler's; declaring one again replaced it with an opaque
// type of the same name, and `u32(n)` was then refused as "has type u32, want u32".
func TestAPrimitiveCannotBeDeclaredAgain(t *testing.T) {
	for _, source := range []string{
		"module app\n\ntype u32\n\nfn main() void {}\n",
		"module app\n\nstruct u8 { x: u32 }\n\nfn main() void {}\n",
	} {
		err := checkSource(t, source)
		if err == nil || !strings.Contains(err.Error(), "redeclares the built-in type") {
			t.Errorf("not refused as a redeclared primitive: %v\n%s", err, source)
		}
	}
}

// A built-in only some backends supply is recorded where it is called, and a
// function the program declares under the same name is its own.
func TestABackendOnlyBuiltinIsRecorded(t *testing.T) {
	for _, c := range []struct {
		source string
		want   bool
	}{
		{"module app\n\nfn main() void {\n    print(get_args())\n}\n", true},
		{"module app\n\nfn get_args() string {\n    return \"\"\n}\n\nfn main() void {\n    print(get_args())\n}\n", false},
		{"module app\n\nfn main() void {\n    print(str_trim(\" a \"))\n}\n", false},
	} {
		tokens, _ := lexer.Lex(c.source)
		module, err := parser.Parse(tokens)
		if err != nil {
			t.Fatal(err)
		}
		tc := New()
		if err := tc.CheckModule(*module); err != nil {
			t.Fatal(err)
		}
		lowered := tc.ToIR(*module)
		got := false
		for _, call := range lowered.BackendCalls {
			if call.Name == "get_args" && call.Line == 4 {
				got = true
			}
		}
		if got != c.want {
			t.Errorf("recorded=%v, want %v: %+v\n%s", got, c.want, lowered.BackendCalls, c.source)
		}
	}
}
