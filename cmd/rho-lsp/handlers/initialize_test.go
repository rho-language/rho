//
// @file initialize_test.go
// @brief Tests for Rho LSP Initialize Handler.
//
// Validates:
// - initialize returns capabilities
// - JSON-RPC response is well-formed
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
	"testing"

	"rho/cmd/rho-lsp/protocol"
	"rho/cmd/rho-lsp/state"
)

func TestHandleInitialize(t *testing.T) {
	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  map[string]any{"rootUri": "file:///"},
	}

	resp := HandleInitialize(req)
	if resp == nil {
		t.Fatalf("expected response, got nil")
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	result := decoded["result"].(map[string]any)
	caps := result["capabilities"].(map[string]any)

	if caps["hoverProvider"] != true {
		t.Fatalf("hoverProvider missing or false")
	}

	if caps["definitionProvider"] != true {
		t.Fatalf("definitionProvider missing or false")
	}

	if caps["completionProvider"] == nil {
		t.Fatalf("completionProvider missing")
	}
}

func TestHandleInitializeStoresWorkspaceRoots(t *testing.T) {
	ws := state.NewWorkspace()
	resp := HandleInitialize(protocol.Request{JsonRPC: "2.0", ID: 2, Method: "initialize", Params: map[string]any{"rootUri": "file:///rho"}}, ws)
	if resp == nil || len(ws.Roots) != 1 || ws.Roots[0] != "file:///rho" {
		t.Fatalf("roots were not stored: %#v", ws.Roots)
	}
}
