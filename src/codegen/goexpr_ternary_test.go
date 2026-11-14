//
// @file goexpr_ternary_test.go
// @brief An if expression is a C conditional, which the Go backend rewrites.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package codegen

import (
	"strings"
	"testing"
)

// An if expression is lowered to `(cond ? then : else)`, which C has and Go
// does not. The Go translator rewrites it to the generic select the runtime
// carries, including when one is nested inside another.
func TestToGoExprRewritesConditional(t *testing.T) {
	cases := map[string]string{
		"(a > b ? x : y)":                        "rhoSelect(a > b, x, y)",
		"(a ? (b ? x : y) : z)":                  "rhoSelect(a, rhoSelect(b, x, y), z)",
		"rho_concat(\"a\", (c ? \"b\" : \"c\"))": "(\"a\" + rhoSelect(c, \"b\", \"c\"))",
	}
	for in, want := range cases {
		if got := ToGoExpr(in); got != want {
			t.Errorf("ToGoExpr(%q) = %q, want %q", in, got, want)
		}
	}
}

// An if expression is lowered to a C conditional and rewritten as rhoSelect;
// the same characters inside a string literal are text and stay as they are.
func TestTernaryRewriteLeavesStringLiteralsAlone(t *testing.T) {
	if got := rewriteTernaries("(ready ? 1 : 2)"); got != "rhoSelect(ready, 1, 2)" {
		t.Fatalf("a conditional was rewritten as %q", got)
	}
	script := `("t.add(ok>=0?'yes':'no')" + x)`
	if got := rewriteTernaries(script); got != script {
		t.Fatalf("a string literal was rewritten: %q", got)
	}
	mixed := `(flag ? "a (b ? c : d)" : "e")`
	if got := rewriteTernaries(mixed); got != `rhoSelect(flag, "a (b ? c : d)", "e")` {
		t.Fatalf("a conditional around string literals was rewritten as %q", got)
	}
}

// A typed conditional whose condition is a call ending in one name, `f(o)`, keeps
// the call: the translator that runs after the rewrite read `(o) {` as a
// compound literal and wrote `fo`.
func TestATypedConditionalKeepsACallAsItsCondition(t *testing.T) {
	got := ToGoExpr(`(is_none_string(o) ? (const char*)("yes") : (const char*)("no"))`)
	if want := `isNoneString(o)`; !strings.Contains(got, want) {
		t.Fatalf("the condition lost its call: %q, want it to contain %q", got, want)
	}
}
