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
// SPDX-License-Identifier: MIT
//

package types

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"testing"

	"rho/src/frontend/ast"
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
