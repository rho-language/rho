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

func TestParseVarAndConst(t *testing.T) {
	source := "module app\nfn main() {\n  var x = 1\n  const y = 2\n  let z = 3\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	mod, err := Parse(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn, ok := mod.Decls[0].(ast.FuncDecl)
	if !ok {
		t.Fatalf("expected function declaration, got %T", mod.Decls[0])
	}
	if len(fn.Body) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(fn.Body))
	}
	for i, want := range []string{"x", "y", "z"} {
		let, ok := fn.Body[i].(ast.LetStmt)
		if !ok {
			t.Fatalf("statement %d: expected let, got %T", i, fn.Body[i])
		}
		if let.Name != want {
			t.Fatalf("statement %d: expected name %q, got %q", i, want, let.Name)
		}
	}
}

func TestParseDynamicArrayType(t *testing.T) {
	source := "module app\nextern fn read_file(path: string) -> [u8]\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	mod, err := Parse(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn, ok := mod.Decls[0].(ast.FuncDecl)
	if !ok {
		t.Fatalf("expected function declaration, got %T", mod.Decls[0])
	}
	if fn.ReturnType != "[u8]" {
		t.Fatalf("expected return type [u8], got %q", fn.ReturnType)
	}
}

func TestParseTopLevelConst(t *testing.T) {
	source := "module app\nconst MAX: u32 = 512\nconst NAME = \"x\"\nfn main() {}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	mod, err := Parse(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(mod.Decls) != 3 {
		t.Fatalf("expected 3 declarations, got %d", len(mod.Decls))
	}
	first, ok := mod.Decls[0].(ast.ConstDecl)
	if !ok {
		t.Fatalf("expected a const declaration, got %T", mod.Decls[0])
	}
	if first.Name != "MAX" || first.Type.Name != "u32" {
		t.Fatalf("unexpected const: %+v", first)
	}
}

func TestParseRepeatArrayLiteral(t *testing.T) {
	source := "module app\nfn main() {\n  var b: u8[8] = [0; 8]\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	mod, err := Parse(tokens)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn, ok := mod.Decls[0].(ast.FuncDecl)
	if !ok {
		t.Fatalf("expected a function, got %T", mod.Decls[0])
	}
	let, ok := fn.Body[0].(ast.LetStmt)
	if !ok {
		t.Fatalf("expected a binding, got %T", fn.Body[0])
	}
	literal, ok := let.Value.(ast.ArrayLiteralExpr)
	if !ok {
		t.Fatalf("expected an array literal, got %T", let.Value)
	}
	if literal.Repeat == nil {
		t.Fatal("expected a repeat count")
	}
}

func TestParseIndexNotTypeApplication(t *testing.T) {
	source := "module app\nfn main() {\n  let i: u32 = 1\n  let x: u32 = data[i + 1]\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	if _, err := Parse(tokens); err != nil {
		t.Fatalf("an arithmetic index should parse: %v", err)
	}
}

func TestParseGenericCall(t *testing.T) {
	source := "module app\nfn twice[T](v: T) T { return v }\nfn main() {\n  let x: u32 = twice[u32](1)\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	if _, err := Parse(tokens); err != nil {
		t.Fatalf("a generic call should parse: %v", err)
	}
}

func TestParseConstBeforeBlock(t *testing.T) {
	// A capitalised constant before a block must not be read as a struct
	// literal whose body is the loop.
	source := "module app\nconst MAX: u32 = 8\nfn main() {\n  let i: u32 = 0\n  while i < MAX {\n    i = i + 1\n  }\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	if _, err := Parse(tokens); err != nil {
		t.Fatalf("a constant before a block should parse: %v", err)
	}
}

func TestParseElseIf(t *testing.T) {
	source := "module app\nfn main() {\n  if a {\n    x = 1\n  } else if b {\n    x = 2\n  } else {\n    x = 3\n  }\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	mod, err := Parse(tokens)
	if err != nil {
		t.Fatalf("an else-if chain should parse: %v", err)
	}
	fn, ok := mod.Decls[0].(ast.FuncDecl)
	if !ok {
		t.Fatalf("expected a function, got %T", mod.Decls[0])
	}
	outer, ok := fn.Body[0].(ast.IfStmt)
	if !ok {
		t.Fatalf("expected an if statement, got %T", fn.Body[0])
	}
	if len(outer.Else) != 1 {
		t.Fatalf("expected one statement in the else, got %d", len(outer.Else))
	}
	inner, ok := outer.Else[0].(ast.IfStmt)
	if !ok {
		t.Fatalf("expected the else to hold an if, got %T", outer.Else[0])
	}
	if len(inner.Else) != 1 {
		t.Fatalf("expected the nested if to keep its else, got %d", len(inner.Else))
	}
}

func TestParseIndexBeforeBlock(t *testing.T) {
	// `a[i] {` is an index and then a block, not an application and a struct
	// literal: `a` is a parameter, which cannot take a type argument.
	source := "module app\nfn f(a: [u32], i: u32) {\n  if a[i] {\n    i = 1\n  }\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	if _, err := Parse(tokens); err != nil {
		t.Fatalf("an indexed condition before a block should parse: %v", err)
	}
}

func TestParseConditionIdentifierBeforeBlock(t *testing.T) {
	// `if id == MESSAGE_ID_TELEMETRY {` is a comparison and then a block, not a
	// comparison and a struct literal. The name is written with a capital
	// because it is a constant, and the parser cannot tell a capitalised
	// constant declared in another file from a type: the `{` after a condition
	// opens the body whatever the name looks like.
	source := "module app\nfn f(id: u32) {\n  if id == MESSAGE_ID_TELEMETRY {\n    id = 1\n  }\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	if _, err := Parse(tokens); err != nil {
		t.Fatalf("a named condition before a block should parse: %v", err)
	}
}

func TestParseGenericLiteralStillParses(t *testing.T) {
	source := "module app\nstruct Box[T] { v: T }\nfn f() {\n  let b: Box[u32] = Box[u32] { 1 }\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}
	if _, err := Parse(tokens); err != nil {
		t.Fatalf("a generic struct literal should still parse: %v", err)
	}
}

func TestParseErrorCarriesPosition(t *testing.T) {
	source := "module app\nfn main() {\n  let x :=\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatalf("lex failed: %v", err)
	}

	_, err = Parse(tokens)
	if err == nil {
		t.Fatal("expected parse error")
	}

	parseErr := AsError(err)
	if parseErr == nil {
		t.Fatalf("expected parser.Error, got %T (%v)", err, err)
	}
	if parseErr.Line != 4 || parseErr.Column != 2 {
		t.Fatalf("expected parse error at 4:2, got %d:%d (%v)", parseErr.Line, parseErr.Column, parseErr)
	}
	if parseErr.Message != "expected expression, got \"}\"" {
		t.Fatalf("unexpected parser message: %q", parseErr.Message)
	}
}
