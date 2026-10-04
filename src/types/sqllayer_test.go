//
// @file sqllayer_test.go
// @brief Tests for the language parts a database surface needs.
//
// A generated query layer is written in a particular shape — a row struct per
// query, a slice of them written by the caller, a result carrying a count or an
// error, and a handle the language cannot name — and each of those exercises
// something the language had to be able to say. These are the cases that had to
// work for such a program to compile, and each of them failed at least once.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package types

import (
	"testing"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

// A signed 64-bit integer is what a database column is, and a value that came
// from outside the program is not a width the program chose.
func TestASignedSixtyFourBitIntegerIsAType(t *testing.T) {
	tc := New()
	typ, ok := tc.resolveType("i64")
	if !ok {
		t.Fatal("i64 is not a type the language knows")
	}
	if typ.Kind != ir.TypeI64 {
		t.Errorf("i64 has kind %v", typ.Kind)
	}
	if !numeric(typ) {
		t.Error("i64 is not a number, so arithmetic on it would be refused")
	}
	// A value that came from a database is signed, and a program that widens one
	// to an unsigned has said something it did not mean.
	if !assignable(typ, ir.Type{Kind: ir.TypeI32, Name: "i32"}) {
		t.Error("an i32 is not assignable to an i64, though every i32 is one")
	}
}

// A generic application that appears only in a signature is still an
// application, and a generated function keeps every one of its types there.
func TestAnApplicationInASignatureIsExpanded(t *testing.T) {
	tc := New()
	tc.genericStructs["Slice"] = ast.StructDecl{
		Name:       "Slice",
		TypeParams: []string{"T"},
		Fields:     []ast.Field{{Name: "len", Type: "u32"}},
	}
	tc.types["CustomerRow"] = ir.Type{Kind: ir.TypeStruct, Name: "CustomerRow"}
	tc.fields["CustomerRow"] = map[string]ir.Type{"id": {Kind: ir.TypeI64, Name: "i64"}}

	// `rows: Slice[CustomerRow]` in a parameter position: no body, no literal.
	tc.scanTypeApplication("Slice[CustomerRow]")
	if len(tc.instantiationQueue) == 0 {
		t.Fatal("an application written in a signature was never recorded, so nothing is emitted for it")
	}
	found := false
	for _, in := range tc.instantiationQueue {
		if in.Name == "Slice" && len(in.TypeArgs) == 1 && in.TypeArgs[0].Name == "CustomerRow" {
			found = true
		}
	}
	if !found {
		t.Errorf("the recorded application is %+v, want Slice applied to CustomerRow", tc.instantiationQueue)
	}
}
