//
// @file goexpr_ternary_test.go
// @brief An if expression is a C conditional, which the Go backend rewrites.
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import "testing"

// An if expression is lowered to `(cond ? then : else)`, which C has and Go
// does not. The Go translator rewrites it to the generic select the runtime
// carries, including when one is nested inside another.
func TestToGoExprRewritesConditional(t *testing.T) {
	cases := map[string]string{
		"(a > b ? x : y)":                     "rhoSelect(a > b, x, y)",
		"(a ? (b ? x : y) : z)":               "rhoSelect(a, rhoSelect(b, x, y), z)",
		"rho_concat(\"a\", (c ? \"b\" : \"c\"))": "(\"a\" + rhoSelect(c, \"b\", \"c\"))",
	}
	for in, want := range cases {
		if got := ToGoExpr(in); got != want {
			t.Errorf("ToGoExpr(%q) = %q, want %q", in, got, want)
		}
	}
}
