//
// @file documents_test.go
// @brief Tests for Rho LSP Document Manager.
//
// Validates:
// - document creation
// - tokenization
// - parsing
// - symbol extraction
// - node lookup
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package state

import (
	"rho/cmd/rho-lsp/protocol"
	"strings"
	"testing"
)

func TestDocumentCreate(t *testing.T) {
	doc := NewDocument("file:///test.rho", "module x")

	if doc.URI != "file:///test.rho" {
		t.Fatalf("unexpected URI: %s", doc.URI)
	}
	if doc.Text != "module x" {
		t.Fatalf("unexpected text: %s", doc.Text)
	}
}

func TestOffsetAtUsesUTF16Columns(t *testing.T) {
	text := "module 😀x"
	if got := offsetAt(text, protocol.Position{Line: 0, Character: 9}); got != len("module 😀") {
		t.Fatalf("unexpected UTF-16 offset: %d", got)
	}
}

func TestDocumentTokens(t *testing.T) {
	doc := NewDocument("file:///test.rho", "module x")

	if len(doc.Tokens) == 0 {
		t.Fatalf("expected tokens, got none")
	}
}

func TestDocumentAST(t *testing.T) {
	doc := NewDocument("file:///test.rho", "module x")

	if doc.AST == nil {
		t.Fatalf("expected AST, got nil")
	}
}

func TestDocumentSymbols(t *testing.T) {
	doc := NewDocument("file:///test.rho", "module x\nfn test() void {}")

	if len(doc.Symbols.Local) == 0 {
		t.Fatalf("expected local symbols, got none")
	}
}

func TestDocumentDoesNotFlagOrdinaryIdentifiersAsUndefined(t *testing.T) {
	// The LSP parser has no scope/param/import tracking, so live diagnostics
	// must not report ordinary identifiers (params, locals, calls) as undefined.
	doc := NewDocument("file:///test.rho", "module x\nfn add(a, b) i32 {\n  return a + b\n}")

	for _, diag := range doc.Diags {
		if strings.Contains(diag.Message, "undefined symbol") {
			t.Fatalf("unexpected undefined-symbol diagnostic: %+v", diag)
		}
	}
}

func TestDocumentNodeAt(t *testing.T) {
	doc := NewDocument("file:///test.rho", "module x\nfn test() void {}")

	node := doc.NodeAt(1, 4)
	if node == nil {
		t.Fatalf("expected node at position, got nil")
	}
}
