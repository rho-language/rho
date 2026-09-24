//
// @file parser_test.go
// @brief Tests for Rho Parser - Enterprise v1.
//
// Validates:
// - module declarations
// - function declarations
// - identifier references
// - diagnostics
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package analysis

import (
	"testing"
)

func TestParseModuleDecl(t *testing.T) {
	tokens := []Token{
		{Kind: "keyword", Text: "module", Line: 0, Column: 0},
		{Kind: "identifier", Text: "x", Line: 0, Column: 7},
	}

	ast, diags := Parse(tokens)

	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if len(ast.Nodes) == 0 {
		t.Fatalf("expected module decl node")
	}

	if ast.Nodes[0].Name() != "x" {
		t.Fatalf("expected module name 'x'")
	}
}

func TestParseFnDecl(t *testing.T) {
	tokens := []Token{
		{Kind: "keyword", Text: "fn", Line: 1, Column: 0},
		{Kind: "identifier", Text: "test", Line: 1, Column: 3},
	}

	ast, _ := Parse(tokens)

	if len(ast.Nodes) == 0 {
		t.Fatalf("expected fn decl node")
	}

	if ast.Nodes[0].Name() != "test" {
		t.Fatalf("expected fn name 'test'")
	}
}

func TestParseIdentifierRef(t *testing.T) {
	tokens := []Token{
		{Kind: "identifier", Text: "value", Line: 2, Column: 1},
	}

	ast, _ := Parse(tokens)

	if len(ast.Nodes) != 1 {
		t.Fatalf("expected one ref node")
	}

	if !ast.Nodes[0].IsRef() {
		t.Fatalf("expected ref node")
	}
}

func TestParseDiagnostics(t *testing.T) {
	tokens := []Token{
		{Kind: "keyword", Text: "module", Line: 0, Column: 0},
		// missing identifier
	}

	_, diags := Parse(tokens)

	if len(diags) == 0 {
		t.Fatalf("expected diagnostic for missing module name")
	}
}
