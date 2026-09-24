//
// @file semantic_test.go
// @brief Tests for Rho Semantic Analyzer - Enterprise v1.
//
// Validates:
//   - duplicate declarations
//   - undefined references
//   - correct diagnostics
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

func TestSemanticDuplicateDecl(t *testing.T) {
	ast := &Module{
		Nodes: []Node{
			NewDeclNode("x", 0, 0),
			NewDeclNode("x", 1, 0),
		},
	}

	diags := Analyze(ast)

	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	if diags[0].Message != "duplicate declaration: x" {
		t.Fatalf("unexpected diagnostic: %s", diags[0].Message)
	}
}

func TestSemanticUndefinedRef(t *testing.T) {
	ast := &Module{
		Nodes: []Node{
			NewRefNode("missing", 2, 5),
		},
	}

	diags := Analyze(ast)

	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	if diags[0].Message != "undefined symbol: missing" {
		t.Fatalf("unexpected diagnostic: %s", diags[0].Message)
	}
}

func TestSemanticNoDiagnostics(t *testing.T) {
	ast := &Module{
		Nodes: []Node{
			NewDeclNode("x", 0, 0),
			NewRefNode("x", 1, 2),
		},
	}

	diags := Analyze(ast)

	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %d", len(diags))
	}
}
