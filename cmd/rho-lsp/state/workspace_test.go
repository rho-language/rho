//
// @file workspace_test.go
// @brief Tests for Rho LSP Workspace Manager.
//
// Validates:
// - document open/update
// - global symbol indexing
// - recompute orchestration
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
	"testing"
)

func TestWorkspaceOpenDocument(t *testing.T) {
	ws := NewWorkspace()

	ws.OpenDocument("file:///test.rho", "module x")

	if ws.GetDocument("file:///test.rho") == nil {
		t.Fatalf("document not stored in workspace")
	}
}

func TestWorkspaceAppliesRangeChangeAndVersion(t *testing.T) {
	ws := NewWorkspace()
	ws.OpenDocument("file:///edit.rho", "module old")
	ws.ApplyChanges("file:///edit.rho", 2, []protocol.TextDocumentContentChangeEvent{{Range: &protocol.Range{Start: protocol.Position{Line: 0, Character: 7}, End: protocol.Position{Line: 0, Character: 10}}, Text: "new"}})
	doc := ws.GetDocument("file:///edit.rho")
	if doc.Text != "module new" || doc.Version != 2 {
		t.Fatalf("unexpected document state: %+v", doc)
	}
}

func TestWorkspaceCloseRemovesSymbols(t *testing.T) {
	ws := NewWorkspace()
	ws.OpenDocument("file:///symbols.rho", "module x\nfn test() void {}")
	if !ws.GlobalSymbols.All["test"] {
		t.Fatal("expected indexed symbol")
	}
	ws.CloseDocument("file:///symbols.rho")
	if ws.GlobalSymbols.All["test"] {
		t.Fatal("stale symbol remains")
	}
}

func TestWorkspaceUpdateDocument(t *testing.T) {
	ws := NewWorkspace()

	ws.OpenDocument("file:///test.rho", "module x")
	ws.UpdateDocument("file:///test.rho", "module y")

	doc := ws.GetDocument("file:///test.rho")
	if doc.Text != "module y" {
		t.Fatalf("document text not updated: %s", doc.Text)
	}
}

func TestWorkspaceGlobalSymbols(t *testing.T) {
	ws := NewWorkspace()

	ws.OpenDocument("file:///a.rho", "module a\nfn test() void {}")
	ws.OpenDocument("file:///b.rho", "module b\ntest()")

	ws.Recompute("file:///a.rho")
	ws.Recompute("file:///b.rho")

	if !ws.GlobalSymbols.All["test"] {
		t.Fatalf("expected global symbol 'test'")
	}

	if len(ws.GlobalSymbols.References["test"]) == 0 {
		t.Fatalf("expected global references for 'test'")
	}
}

func TestWorkspaceRecompute(t *testing.T) {
	ws := NewWorkspace()

	ws.OpenDocument("file:///test.rho", "module x")
	diags := ws.Recompute("file:///test.rho")

	if diags == nil {
		t.Fatalf("expected diagnostics array, got nil")
	}
}
