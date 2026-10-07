//
// @file typechecker_test.go
// @brief Rho Compiler - Type Checker tests.
//
// This file contains deterministic unit tests for the Rho type checker.
// Tests validate correct type registration, struct validation, function
// signature validation, error reporting, and AST → IR conversion.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package types

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"testing"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Built-in Types Registration                                         */
/* -------------------------------------------------------------------------- */

func TestBuiltInTypes(t *testing.T) {
	tc := New()

	builtins := []string{"void", "bool", "u8", "u32", "u64", "string"}

	for _, name := range builtins {
		if _, ok := tc.types[name]; !ok {
			t.Fatalf("expected built-in type %q to be registered", name)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Struct Validation                                                   */
/* -------------------------------------------------------------------------- */

func TestStructValidationOK(t *testing.T) {
	tc := New()

	m := ast.Module{
		Name: "lighting",
		Decls: []ast.Decl{
			ast.StructDecl{
				Name: "Light",
				Fields: []ast.Field{
					{Name: "pin", Type: "u8"},
					{Name: "enabled", Type: "bool"},
				},
			},
		},
	}

	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStructValidationUnknownType(t *testing.T) {
	tc := New()

	m := ast.Module{
		Name: "lighting",
		Decls: []ast.Decl{
			ast.StructDecl{
				Name: "Light",
				Fields: []ast.Field{
					{Name: "pin", Type: "u8"},
					{Name: "mode", Type: "UnknownType"},
				},
			},
		},
	}

	if err := tc.CheckModule(m); err == nil {
		t.Fatalf("expected error for unknown type in struct")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Function Validation                                                 */
/* -------------------------------------------------------------------------- */

func TestFuncValidationOK(t *testing.T) {
	tc := New()

	m := ast.Module{
		Name: "lighting",
		Decls: []ast.Decl{
			ast.FuncDecl{
				Name: "set",
				Params: []ast.Param{
					{Name: "pin", Type: "u8"},
					{Name: "value", Type: "bool"},
				},
				ReturnType: "bool",
			},
		},
	}

	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFuncValidationUnknownParamType(t *testing.T) {
	tc := New()

	m := ast.Module{
		Name: "lighting",
		Decls: []ast.Decl{
			ast.FuncDecl{
				Name: "set",
				Params: []ast.Param{
					{Name: "pin", Type: "u8"},
					{Name: "value", Type: "UnknownType"},
				},
				ReturnType: "bool",
			},
		},
	}

	if err := tc.CheckModule(m); err == nil {
		t.Fatalf("expected error for unknown parameter type")
	}
}

func TestFuncValidationUnknownReturnType(t *testing.T) {
	tc := New()

	m := ast.Module{
		Name: "lighting",
		Decls: []ast.Decl{
			ast.FuncDecl{
				Name:       "set",
				Params:     []ast.Param{{Name: "pin", Type: "u8"}},
				ReturnType: "UnknownType",
			},
		},
	}

	if err := tc.CheckModule(m); err == nil {
		t.Fatalf("expected error for unknown return type")
	}
}

func TestFuncBodyRejectsUndefinedName(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{ast.FuncDecl{
		Name: "main", ReturnType: "void",
		Body: []ast.Stmt{ast.ExprStmt{Expr: ast.IdentifierExpr{Name: "missing"}}},
	}}}
	if err := tc.CheckModule(m); err == nil {
		t.Fatal("expected undefined name error")
	}
}

func TestFuncBodyRejectsTypeMismatch(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{ast.FuncDecl{
		Name: "main", ReturnType: "void",
		Body: []ast.Stmt{ast.LetStmt{Name: "value", Type: ast.TypeExpr{Name: "bool"}, Value: ast.NumberExpr{Value: "1"}}},
	}}}
	if err := tc.CheckModule(m); err == nil {
		t.Fatal("expected type mismatch error")
	}
}

func TestFuncBodyRejectsMissingReturn(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{ast.FuncDecl{
		Name: "value", ReturnType: "i32",
		Body: []ast.Stmt{ast.ExprStmt{Expr: ast.NumberExpr{Value: "1"}}},
	}}}
	if err := tc.CheckModule(m); err == nil {
		t.Fatal("expected missing return error")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: AST → IR Conversion                                                 */
/* -------------------------------------------------------------------------- */

func TestToIRConversion(t *testing.T) {
	tc := New()

	m := ast.Module{
		Name: "lighting",
		Decls: []ast.Decl{
			ast.StructDecl{
				Name: "Light",
				Fields: []ast.Field{
					{Name: "pin", Type: "u8"},
				},
			},
			ast.FuncDecl{
				Name:       "init",
				Params:     []ast.Param{},
				ReturnType: "void",
			},
		},
	}

	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	irMod := tc.ToIR(m)
	if len(irMod.Structs) != 1 || len(irMod.Funcs) != 1 {
		t.Fatalf("unexpected IR declaration counts: %+v", irMod)
	}
	if irMod.Structs[0].Name != "Light" || irMod.Funcs[0].Name != "init" {
		t.Fatalf("unexpected IR declarations: %+v", irMod)
	}
}

// A generic struct is a template. Until a concrete application is requested
// there is nothing to emit, because the field type T has no single lowering.
func TestToIRSkipsUninstantiatedGeneric(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.StructDecl{Name: "Box", TypeParams: []string{"T"}, Fields: []ast.Field{{Name: "value", Type: "T", TypeExpr: ast.TypeExpr{Name: "T"}}}},
	}}
	if err := tc.CheckModule(m); err != nil {
		t.Fatal(err)
	}
	irModule := tc.ToIR(m)
	if len(irModule.Structs) != 0 {
		t.Fatalf("an uninstantiated generic must not be emitted, got %+v", irModule.Structs)
	}
}

// A concrete application is monomorphised: the field type parameter is
// replaced and the declaration is renamed to the mangled specialisation.
func TestToIRMonomorphisesGenericStruct(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.StructDecl{Name: "Box", TypeParams: []string{"T"}, Fields: []ast.Field{{Name: "value", Type: "T", TypeExpr: ast.TypeExpr{Name: "T"}}}},
		ast.FuncDecl{
			Name:       "make",
			Params:     []ast.Param{},
			ReturnType: "Box[u32]",
			Body: []ast.Stmt{
				ast.LetStmt{Name: "b", Type: ast.TypeExpr{Name: "Box", Args: []ast.TypeExpr{{Name: "u32"}}}},
				ast.ReturnStmt{Value: ast.IdentifierExpr{Name: "b"}},
			},
		},
	}}
	if err := tc.CheckModule(m); err != nil {
		t.Fatal(err)
	}
	irModule := tc.ToIR(m)
	if len(irModule.Structs) != 1 {
		t.Fatalf("expected one specialisation, got %+v", irModule.Structs)
	}
	specialised := irModule.Structs[0]
	if specialised.Name != "Box_u32" {
		t.Fatalf("expected the mangled name Box_u32, got %q", specialised.Name)
	}
	if field := specialised.Fields[0].Type; field.Name != "u32" {
		t.Fatalf("expected the field type substituted to u32, got %#v", field)
	}
}

func TestGenericFieldSubstitution(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.StructDecl{Name: "Box", TypeParams: []string{"T"}, Fields: []ast.Field{{Name: "value", Type: "T", TypeExpr: ast.TypeExpr{Name: "T"}}}},
		ast.FuncDecl{Name: "read", Params: []ast.Param{{Name: "box", Type: "Box[u32]", TypeExpr: ast.TypeExpr{Name: "Box", Args: []ast.TypeExpr{{Name: "u32"}}}}}, ReturnType: "u32", Body: []ast.Stmt{ast.ReturnStmt{Value: ast.MemberExpr{Object: ast.IdentifierExpr{Name: "box"}, Name: "value"}}}},
	}}
	if err := tc.CheckModule(m); err != nil {
		t.Fatal(err)
	}
}

func TestPtrIndexPassesElementTypeToCall(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.FuncDecl{
			Name: "crc32_byte",
			Params: []ast.Param{
				{Name: "crc", Type: "u32"},
				{Name: "b", Type: "u8"},
			},
			ReturnType: "u32",
			Body: []ast.Stmt{
				ast.ReturnStmt{Value: ast.IdentifierExpr{Name: "crc"}},
			},
		},
		ast.FuncDecl{
			Name: "crc32_buffer",
			Params: []ast.Param{
				{Name: "data", Type: "Ptr[u8]", TypeExpr: ast.TypeExpr{Name: "Ptr", Args: []ast.TypeExpr{{Name: "u8"}}}},
				{Name: "length", Type: "u32"},
			},
			ReturnType: "u32",
			Body: []ast.Stmt{
				ast.LetStmt{Name: "crc", Type: ast.TypeExpr{Name: "u32"}, Value: ast.NumberExpr{Value: "0"}},
				ast.LetStmt{Name: "i", Type: ast.TypeExpr{Name: "u32"}, Value: ast.NumberExpr{Value: "0"}},
				ast.WhileStmt{
					Condition: ast.BinaryExpr{Op: "<", Left: ast.IdentifierExpr{Name: "i"}, Right: ast.IdentifierExpr{Name: "length"}},
					Body: []ast.Stmt{
						ast.AssignStmt{
							Target: ast.IdentifierExpr{Name: "crc"},
							Op:     ":=",
							Value: ast.CallExpr{Callee: ast.IdentifierExpr{Name: "crc32_byte"}, Args: []ast.Expr{
								ast.IdentifierExpr{Name: "crc"},
								ast.IndexExpr{Object: ast.IdentifierExpr{Name: "data"}, Index: ast.IdentifierExpr{Name: "i"}},
							}},
						},
						ast.AssignStmt{
							Target: ast.IdentifierExpr{Name: "i"},
							Op:     ":=",
							Value:  ast.BinaryExpr{Op: "+", Left: ast.IdentifierExpr{Name: "i"}, Right: ast.NumberExpr{Value: "1"}},
						},
					},
				},
				ast.ReturnStmt{Value: ast.IdentifierExpr{Name: "crc"}},
			},
		},
	}}

	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddressOfIndexedElementAssignsToPointer(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.FuncDecl{
			Name:       "main",
			ReturnType: "void",
			Body: []ast.Stmt{
				ast.LetStmt{Name: "buf", Type: ast.TypeExpr{Name: "u8", ArrayLen: intPtr(16)}},
				ast.LetStmt{
					Name: "data",
					Type: ast.TypeExpr{Name: "Ptr", Args: []ast.TypeExpr{{Name: "u8"}}},
					Value: ast.UnaryExpr{
						Op: "&",
						Value: ast.IndexExpr{
							Object: ast.IdentifierExpr{Name: "buf"},
							Index:  ast.NumberExpr{Value: "0"},
						},
					},
				},
			},
		},
	}}

	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStringConcatenation(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.FuncDecl{
			Name:       "main",
			ReturnType: "void",
			Body: []ast.Stmt{
				ast.LetStmt{Name: "a", Value: ast.StringExpr{Value: "hello"}},
				ast.LetStmt{Name: "b", Value: ast.StringExpr{Value: "world"}},
				ast.LetStmt{
					Name: "c",
					Value: ast.BinaryExpr{
						Op:    "+",
						Left:  ast.IdentifierExpr{Name: "a"},
						Right: ast.IdentifierExpr{Name: "b"},
					},
				},
			},
		},
	}}
	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("string concatenation should succeed: %v", err)
	}
}

func TestStringEquality(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.FuncDecl{
			Name:       "main",
			ReturnType: "void",
			Body: []ast.Stmt{
				ast.LetStmt{Name: "a", Value: ast.StringExpr{Value: "hello"}},
				ast.LetStmt{
					Name: "eq",
					Value: ast.BinaryExpr{
						Op:    "==",
						Left:  ast.IdentifierExpr{Name: "a"},
						Right: ast.StringExpr{Value: "hello"},
					},
				},
			},
		},
	}}
	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("string == should succeed: %v", err)
	}
}

func TestStringInequality(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.FuncDecl{
			Name:       "main",
			ReturnType: "void",
			Body: []ast.Stmt{
				ast.LetStmt{Name: "a", Value: ast.StringExpr{Value: "hello"}},
				ast.LetStmt{
					Name: "ne",
					Value: ast.BinaryExpr{
						Op:    "!=",
						Left:  ast.IdentifierExpr{Name: "a"},
						Right: ast.StringExpr{Value: "world"},
					},
				},
			},
		},
	}}
	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("string != should succeed: %v", err)
	}
}

func TestDynamicArrayTypeResolves(t *testing.T) {
	tc := New()
	typ, ok := tc.resolveType("[u8]")
	if !ok {
		t.Fatal("expected [u8] to resolve")
	}
	if len(typ.Args) != 1 || typ.Args[0].Kind != ir.TypeU8 {
		t.Fatalf("expected [u8] with a u8 element, got %+v", typ)
	}
}

func TestLenAcceptsStringAndArray(t *testing.T) {
	signature := intrinsicSignatures["len"]
	if len(signature.params) != 1 {
		t.Fatalf("len should take one argument, got %d", len(signature.params))
	}
	if !assignable(signature.params[0], ir.Type{Kind: ir.TypeString, Name: "string"}) {
		t.Fatal("len should accept a string")
	}
	array := ir.Type{Kind: ir.TypeSlice, Name: "[u8]", Args: []ir.Type{{Kind: ir.TypeU8, Name: "u8"}}}
	if !assignable(signature.params[0], array) {
		t.Fatal("len should accept a dynamic array")
	}
}

func TestElementTypeOfStringIsU8(t *testing.T) {
	elem := elementType(ir.Type{Kind: ir.TypeString, Name: "string"})
	if elem.Kind != ir.TypeU8 {
		t.Fatalf("string element type should be u8, got %v", elem.Kind)
	}
}

func TestModuleConstKeepsDeclaredType(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.ConstDecl{Name: "MAX", Type: ast.TypeExpr{Name: "u32"}, Value: ast.NumberExpr{Value: "512"}},
		ast.FuncDecl{
			Name:       "main",
			ReturnType: "void",
			Body: []ast.Stmt{
				ast.LetStmt{Name: "n", Type: ast.TypeExpr{Name: "u32"}, Value: ast.IdentifierExpr{Name: "MAX"}},
			},
		},
	}}
	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("const should check: %v", err)
	}
	// A `0` literal infers as i32 on its own; the written u32 has to win, or a
	// comparison with a u32 would be promoted to a signed type.
	if got := tc.moduleConstTypes["MAX"]; got.Kind != ir.TypeU32 {
		t.Fatalf("expected the declared u32, got %v", got.Kind)
	}
}

func TestModuleConstVisibleInFunction(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.ConstDecl{Name: "MAGIC", Type: ast.TypeExpr{Name: "u32"}, Value: ast.NumberExpr{Value: "305419896"}},
		ast.FuncDecl{
			Name:       "main",
			ReturnType: "u32",
			Body: []ast.Stmt{
				ast.ReturnStmt{Value: ast.IdentifierExpr{Name: "MAGIC"}},
			},
		},
	}}
	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("a function should see a module constant: %v", err)
	}
}

func TestModuleConstIsInlined(t *testing.T) {
	tc := New()
	m := ast.Module{Decls: []ast.Decl{
		ast.ConstDecl{Name: "MAGIC", Type: ast.TypeExpr{Name: "u32"}, Value: ast.NumberExpr{Value: "42"}},
		ast.FuncDecl{
			Name:       "main",
			ReturnType: "u32",
			Body: []ast.Stmt{
				ast.ReturnStmt{Value: ast.IdentifierExpr{Name: "MAGIC"}},
			},
		},
	}}
	if err := tc.CheckModule(m); err != nil {
		t.Fatalf("const should check: %v", err)
	}
	moduleIR := tc.ToIR(m)
	if len(moduleIR.Funcs) == 0 {
		t.Fatal("expected the function to be lowered")
	}
	body := moduleIR.Funcs[0].Body
	if len(body) == 0 {
		t.Fatal("expected a body")
	}
	if got := body[0].Args; len(got) == 0 || got[0] != "42" {
		t.Fatalf("expected the constant to be inlined as 42, got %v", got)
	}
}

func intPtr(value int) *int { return &value }

/* -------------------------------------------------------------------------- */
/*  Tests: f32 is a type of its own                                            */
/* -------------------------------------------------------------------------- */

// f32 is a 32-bit float, and it is not f64. The two return in different
// registers on an ARM EABI target, so a foreign function declared to return one
// and reached as the other reads the wrong register — a bug that surfaces as a
// wrong number rather than as a link error.
func TestF32IsItsOwnType(t *testing.T) {
	tc := New()
	typ, ok := tc.resolveType("f32")
	if !ok {
		t.Fatal("f32 did not resolve to a type")
	}
	if typ.Kind != ir.TypeF32 {
		t.Fatalf("f32 resolved to kind %v, want TypeF32", typ.Kind)
	}

	f64, ok := tc.resolveType("f64")
	if !ok || f64.Kind != ir.TypeF64 {
		t.Fatal("f64 no longer resolves to TypeF64")
	}
	if typ.Kind == f64.Kind {
		t.Fatal("f32 and f64 resolved to the same kind")
	}
}
