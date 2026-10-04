//
// @file did_open_test.go
// @brief Tests for Rho LSP DidOpen Handler.
//
// Validates:
// - document is added to workspace
// - diagnostics are returned
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rho/rho-lsp/src/protocol"
	"github.com/rho/rho-lsp/src/state"
)

func TestHandleDidOpen(t *testing.T) {
	ws := state.NewWorkspace()

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      10,
		Method:  "textDocument/didOpen",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri":  "file:///test.rho",
				"text": "module x",
			},
		},
	}

	resp := HandleDidOpen(req, ws)
	if resp == nil {
		t.Fatalf("expected response, got nil")
	}

	data, _ := json.Marshal(resp)
	out := string(data)

	if !strings.Contains(out, `"diagnostics"`) {
		t.Fatalf("missing diagnostics: %s", out)
	}

	// Workspace should contain the document
	if ws.GetDocument("file:///test.rho") == nil {
		t.Fatalf("document not added to workspace")
	}
}
