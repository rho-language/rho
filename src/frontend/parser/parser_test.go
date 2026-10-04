//
// @file parser_test.go
// @brief Rho Compiler - Parser subsystem tests.
//
// This file contains deterministic unit tests for the Rho parser.
// Tests validate correct construction of the AST for module declarations,
// imports, struct declarations, and function declarations according to
// the Rho specification.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package parser

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Module Declaration                                                  */
/* -------------------------------------------------------------------------- */

func TestParseModule(t *testing.T) {
	src := "module lighting"
	lx := lexer.New(src)
	p := New(lx)

	mod := p.parseModuleDecl()

	if mod.Name != "lighting" {
		t.Fatalf("expected module name 'lighting', got %q", mod.Name)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Import List                                                         */
/* -------------------------------------------------------------------------- */

func TestParseImports(t *testing.T) {
	src := `
        module lighting
        import gpio
        import config
    `
	lx := lexer.New(src)
	p := New(lx)

	_ = p.parseModuleDecl()
	imports := p.parseImports()

	if len(imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(imports))
	}

	if imports[0].Name != "gpio" {
		t.Fatalf("expected first import 'gpio', got %q", imports[0].Name)
	}
	if imports[1].Name != "config" {
		t.Fatalf("expected second import 'config', got %q", imports[1].Name)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Struct Declaration                                                  */
/* -------------------------------------------------------------------------- */

func TestParseStruct(t *testing.T) {
	src := `
        module lighting
        struct Light {
            pin: u8
            enabled: bool
        }
    `
	lx := lexer.New(src)
	p := New(lx)

	p.parseModuleDecl()
	p.parseImports()

	decls := p.parseTopLevelDecls()
	if len(decls) != 1 {
		t.Fatalf("expected 1 top-level decl, got %d", len(decls))
	}

	s, ok := decls[0].(ast.StructDecl)
	if !ok {
		t.Fatalf("expected StructDecl, got %T", decls[0])
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
/*  Tests: Function Declaration                                                */
/* -------------------------------------------------------------------------- */

func TestParseFunction(t *testing.T) {
	src := `
        module lighting

        fn set(pin: u8, value: bool) -> bool {
            return value
        }
    `
	lx := lexer.New(src)
	p := New(lx)

	p.parseModuleDecl()
	p.parseImports()

	decls := p.parseTopLevelDecls()
	if len(decls) != 1 {
		t.Fatalf("expected 1 top-level decl, got %d", len(decls))
	}

	fn, ok := decls[0].(ast.FuncDecl)
	if !ok {
		t.Fatalf("expected FuncDecl, got %T", decls[0])
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
}
