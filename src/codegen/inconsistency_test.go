//
// @file inconsistency_test.go
// @brief Tests for the C backend's ordering and Go's unused-local analysis.
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package codegen

import (
	"strings"
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

// An alias and the structs are one ordering: std.strings.string declares
// `String = Slice[u8]` and uses `Slice[String]`, whose field is `String*`, so
// the C program needs Slice_u8, then the String typedef, then Slice_String —
// whatever order the lists arrive in.
func TestOrderedTypeDeclsPutsAliasBeforeItsUsers(t *testing.T) {
	u8 := ir.Type{Kind: ir.TypeU8, Name: "u8"}
	sliceU8 := ir.StructIR{Name: "Slice_u8", Fields: []ir.FieldIR{
		{Name: "ptr", Type: ir.Type{Kind: ir.TypePtr, Name: "Ptr[u8]", Args: []ir.Type{u8}}},
		{Name: "len", Type: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
	}}
	sliceString := ir.StructIR{Name: "Slice_String", Fields: []ir.FieldIR{
		{Name: "ptr", Type: ir.Type{Kind: ir.TypePtr, Name: "Ptr[String]",
			Args: []ir.Type{{Kind: ir.TypeStruct, Name: "String"}}}},
		{Name: "len", Type: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
	}}
	holder := ir.StructIR{Name: "Holder", Fields: []ir.FieldIR{
		{Name: "text", Type: ir.Type{Kind: ir.TypeStruct, Name: "Text"}},
	}}
	aliases := []ir.AliasIR{
		// An alias of an alias comes after the one it names.
		{Name: "Text", Target: "String", CTarget: "String"},
		{Name: "String", Target: "Slice[u8]", CTarget: "Slice_u8"},
		// An alias with no C spelling is not declared at all.
		{Name: "Nothing", Target: "?"},
	}
	got := orderedTypeDecls([]ir.StructIR{holder, sliceString, sliceU8}, aliases)
	position := map[string]int{}
	for i, d := range got {
		position[d.name] = i
	}
	if len(got) != 5 {
		t.Fatalf("want 5 declarations, got %d: %v", len(got), position)
	}
	if _, declared := position["Nothing"]; declared {
		t.Errorf("an alias without a C spelling should not be declared")
	}
	before := [][2]string{
		{"Slice_u8", "String"},
		{"String", "Slice_String"},
		{"String", "Text"},
		{"Text", "Holder"},
	}
	for _, pair := range before {
		if position[pair[0]] >= position[pair[1]] {
			t.Errorf("%s must come before %s, got order %v", pair[0], pair[1], position)
		}
	}
}

// The order is the one emitted: the module's C declares the String typedef
// before the specialisation whose field names it.
func TestEmitModuleDeclaresAliasBeforeSpecialisation(t *testing.T) {
	u8 := ir.Type{Kind: ir.TypeU8, Name: "u8"}
	m := ir.ModuleIR{
		Name: "app",
		Structs: []ir.StructIR{
			{Name: "Slice_String", Fields: []ir.FieldIR{
				{Name: "ptr", Type: ir.Type{Kind: ir.TypePtr, Name: "Ptr[String]",
					Args: []ir.Type{{Kind: ir.TypeStruct, Name: "String"}}}},
				{Name: "len", Type: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
			}},
			{Name: "Slice_u8", Fields: []ir.FieldIR{
				{Name: "ptr", Type: ir.Type{Kind: ir.TypePtr, Name: "Ptr[u8]", Args: []ir.Type{u8}}},
				{Name: "len", Type: ir.Type{Kind: ir.TypeU32, Name: "u32"}},
			}},
		},
		Aliases: []ir.AliasIR{{Name: "String", Target: "Slice[u8]", CTarget: "Slice_u8"}},
	}
	out := New().EmitModule(m)
	sliceU8 := strings.Index(out, "} Slice_u8;")
	alias := strings.Index(out, "typedef Slice_u8 String;")
	sliceString := strings.Index(out, "} Slice_String;")
	if sliceU8 < 0 || alias < 0 || sliceString < 0 {
		t.Fatalf("missing a declaration in:\n%s", out)
	}
	if !(sliceU8 < alias && alias < sliceString) {
		t.Fatalf("want Slice_u8, then String, then Slice_String; got\n%s", out)
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
