//
// @file references_test.go
// @brief Tests for Rho LSP References Handler.
//
// Validates:
// - local references are returned
// - global references are returned
// - empty list for unknown symbols
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

func TestReferencesLocal(t *testing.T) {
	ws := state.NewWorkspace()

	// Document with a local symbol and references
	ws.OpenDocument("file:///test.rho", "module x\nfn test() void {}\ntest()")
	ws.Recompute("file:///test.rho")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      70,
		Method:  "textDocument/references",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///test.rho",
			},
			"position": map[string]any{
				"line":      1,
				"character": 4,
			},
		},
	}

	resp := HandleReferences(req, ws)
	data, _ := json.Marshal(resp.Result)

	var out []protocol.Location
	_ = json.Unmarshal(data, &out)

	if len(out) == 0 {
		t.Fatalf("expected local references, got none")
	}
}

func TestReferencesGlobal(t *testing.T) {
	ws := state.NewWorkspace()

	// Add global symbol references
	ws.GlobalSymbols.References["global_fn"] = []protocol.Location{
		{
			URI: "file:///a.rho",
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 10},
			},
		},
	}

	ws.OpenDocument("file:///test.rho", "module x\nglobal_fn")
	ws.Recompute("file:///test.rho")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      71,
		Method:  "textDocument/references",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///test.rho",
			},
			"position": map[string]any{
				"line":      1,
				"character": 2,
			},
		},
	}

	resp := HandleReferences(req, ws)
	data, _ := json.Marshal(resp.Result)

	var out []protocol.Location
	_ = json.Unmarshal(data, &out)

	if len(out) == 0 {
		t.Fatalf("expected global references, got none")
	}
}

func TestReferencesEmpty(t *testing.T) {
	ws := state.NewWorkspace()

	ws.OpenDocument("file:///test.rho", "module x")
	ws.Recompute("file:///test.rho")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      72,
		Method:  "textDocument/references",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///test.rho",
			},
			"position": map[string]any{
				"line":      10,
				"character": 10,
			},
		},
	}

	resp := HandleReferences(req, ws)
	data, _ := json.Marshal(resp.Result)

	var out []protocol.Location
	_ = json.Unmarshal(data, &out)

	if len(out) != 0 {
		t.Fatalf("expected empty references, got: %v", out)
	}
}
