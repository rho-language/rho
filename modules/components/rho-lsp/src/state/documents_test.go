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
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package state

import (
	"strings"
	"testing"

	"github.com/rho/rho-lsp/src/protocol"
	"rho/src"
	"rho/src/types"
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

func TestDocumentIndex(t *testing.T) {
	doc := NewDocument("file:///test.rho", "module x\n\nfn test() void {\n  let a: i32 = 1;\n  return;\n}\n")

	if doc.Index == nil {
		t.Fatal("a well-formed file was not indexed")
	}
	if !doc.Symbols.Local["test"] {
		t.Error("a declared function was not indexed")
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

	node, found := doc.NodeAt(1, 4)
	if !found {
		t.Fatal("expected something at the position, got nothing")
	}
	if node.Name != "test" {
		t.Errorf("found %q, want test", node.Name)
	}
}

// A use of something deprecated is a warning the editor strikes through: the
// compiler's code for it becomes the protocol's Deprecated tag.
func TestADeprecationWarningCarriesTheDeprecatedTag(t *testing.T) {
	published := publishable([]rho.Diagnostic{
		{File: "/p/src/main.rho", Line: 6, Column: 18, Severity: rho.SeverityWarning, Code: types.DeprecatedCode, Message: "old_area is deprecated: use area instead"},
		{File: "/p/src/main.rho", Line: 7, Column: 1, Severity: rho.SeverityWarning, Code: "type", Message: "never runs"},
	}, "/p/src/main.rho", true, "module main\n\n\n\n\n    let a: u32 = lib.old_area(2, 3)\n_ => {}\n")
	if len(published) != 2 {
		t.Fatalf("published %d diagnostics, want 2", len(published))
	}
	if len(published[0].Tags) != 1 || published[0].Tags[0] != protocol.DiagnosticTagDeprecated || published[0].Severity != 2 {
		t.Errorf("the deprecation is %+v, want a warning tagged deprecated", published[0])
	}
	if len(published[1].Tags) != 0 {
		t.Errorf("an ordinary warning was tagged: %+v", published[1])
	}
}
