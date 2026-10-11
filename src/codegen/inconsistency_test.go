//
// @file inconsistency_test.go
// @brief Tests for the C backend's ordering and Go's unused-local analysis.
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"testing"

	"rho/src/ir"
)

// A struct named by another struct's field is emitted first: C needs the type
// defined before it is used, which a nested specialisation — Box[Box[u32]] —
// depends on.
func TestOrderedStructsPutsDependencyFirst(t *testing.T) {
	inner := ir.StructIR{Name: "Box_u32", Fields: []ir.FieldIR{
		{Name: "v", Type: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
	}}
	outer := ir.StructIR{Name: "Box_Box_u32", Fields: []ir.FieldIR{
		{Name: "v", Type: ir.Type{Kind: ir.TypeStruct, Name: "Box_u32"}},
	}}
	got := orderedStructs([]ir.StructIR{outer, inner})
	if len(got) != 2 || got[0].Name != "Box_u32" {
		t.Fatalf("want the inner struct first, got %v", got)
	}
}

// A local whose name also appears inside a string literal is still unused, and
// must be discarded, or Go rejects the generated function.
func TestCollectWordsSkipsStringContents(t *testing.T) {
	words := map[string]bool{}
	collectWords("rho_print(\"x\")", words)
	if words["x"] {
		t.Errorf("a name inside a string literal is not a use")
	}
	if !words["rho_print"] {
		t.Errorf("the call name should be collected")
	}
}
