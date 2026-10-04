//
// @file ast_test.go
// @brief Rho Compiler - AST subsystem tests.
//
// This file contains deterministic unit tests for the Rho AST structures.
// Tests validate correct construction and integrity of AST nodes such as
// modules, imports, struct declarations, and function declarations.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package ast

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Module Node                                                        */
/* -------------------------------------------------------------------------- */

func TestModuleNode(t *testing.T) {
	mod := Module{
		Name: "lighting",
		Imports: []Import{
			{Name: "gpio"},
			{Name: "config"},
		},
		Decls: []Decl{},
	}

	if mod.Name != "lighting" {
		t.Fatalf("expected module name 'lighting', got %q", mod.Name)
	}

	if len(mod.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(mod.Imports))
	}

	if mod.Imports[0].Name != "gpio" {
		t.Fatalf("expected first import 'gpio', got %q", mod.Imports[0].Name)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Struct Declaration                                                 */
/* -------------------------------------------------------------------------- */

func TestStructDecl(t *testing.T) {
	s := StructDecl{
		Name: "Light",
		Fields: []Field{
			{Name: "pin", Type: "u8"},
			{Name: "enabled", Type: "bool"},
		},
	}

	if s.Name != "Light" {
		t.Fatalf("expected struct name 'Light', got %q", s.Name)
	}

	if len(s.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(s.Fields))
	}

	if s.Fields[0].Name != "pin" || s.Fields[0].Type != "u8" {
		t.Fatalf("unexpected field 0: %+v", s.Fields[0])
	}

	if s.Fields[1].Name != "enabled" || s.Fields[1].Type != "bool" {
		t.Fatalf("unexpected field 1: %+v", s.Fields[1])
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Function Declaration                                               */
/* -------------------------------------------------------------------------- */

func TestFuncDecl(t *testing.T) {
	fn := FuncDecl{
		Name: "set",
		Params: []Param{
			{Name: "pin", Type: "u8"},
			{Name: "value", Type: "bool"},
		},
		ReturnType: "bool",
		Body:       []Stmt{}, // Rho 0.1: empty
	}

	if fn.Name != "set" {
		t.Fatalf("expected function name 'set', got %q", fn.Name)
	}

	if len(fn.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(fn.Params))
	}

	if fn.Params[0].Name != "pin" || fn.Params[0].Type != "u8" {
		t.Fatalf("unexpected param 0: %+v", fn.Params[0])
	}

	if fn.Params[1].Name != "value" || fn.Params[1].Type != "bool" {
		t.Fatalf("unexpected param 1: %+v", fn.Params[1])
	}

	if fn.ReturnType != "bool" {
		t.Fatalf("expected return type 'bool', got %q", fn.ReturnType)
	}

	if len(fn.Body) != 0 {
		t.Fatalf("expected empty body for Rho 0.1, got %d statements", len(fn.Body))
	}
}
