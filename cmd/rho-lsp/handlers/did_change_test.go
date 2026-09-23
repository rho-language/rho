//
// @file did_change_test.go
// @brief Tests for Rho LSP DidChange Handler.
//
// Validates:
// - document text is updated
// - diagnostics are regenerated
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"rho/cmd/rho-lsp/protocol"
	"rho/cmd/rho-lsp/state"
)

func TestHandleDidChange(t *testing.T) {
	ws := state.NewWorkspace()

	// First open the document
	ws.OpenDocument("file:///test.rho", "module x")

	// Now change it
	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      20,
		Method:  "textDocument/didChange",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri":     "file:///test.rho",
				"version": 2,
			},
			"contentChanges": []map[string]any{
				{"text": "module y"},
			},
		},
	}

	resp := HandleDidChange(req, ws)
	if resp == nil {
		t.Fatalf("expected response, got nil")
	}

	data, _ := json.Marshal(resp)
	out := string(data)

	if !strings.Contains(out, `"diagnostics"`) {
		t.Fatalf("missing diagnostics: %s", out)
	}

	// Workspace should contain updated text
	doc := ws.GetDocument("file:///test.rho")
	if doc == nil {
		t.Fatalf("document missing from workspace")
	}
	if doc.Text != "module y" {
		t.Fatalf("document text not updated: %s", doc.Text)
	}
}
