//
// @file hover_test.go
// @brief Tests for Rho LSP Hover Handler.
//
// Validates:
// - hover returns info for local symbols
// - hover returns info for global symbols
// - hover returns nil for unknown positions
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

func TestHoverLocalSymbol(t *testing.T) {
	ws := state.NewWorkspace()

	// Document with a local symbol
	ws.OpenDocument("file:///test.rho", "module x\nfn test() void {}")
	ws.Recompute("file:///test.rho")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      40,
		Method:  "textDocument/hover",
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

	resp := HandleHover(req, ws)
	data, _ := json.Marshal(resp.Result)
	out := string(data)

	if !strings.Contains(out, "test (local symbol)") {
		t.Fatalf("hover missing local symbol: %s", out)
	}
	if !strings.Contains(out, `"kind":"plaintext"`) {
		t.Fatalf("hover contents missing MarkupContent kind: %s", out)
	}
	if strings.Contains(out, `"character":0},"end":{"line":0,"character":0`) {
		t.Fatalf("hover range was not set to the symbol position: %s", out)
	}
}

func TestHoverGlobalSymbol(t *testing.T) {
	ws := state.NewWorkspace()

	// Add global symbol
	ws.GlobalSymbols.All["global_fn"] = true

	ws.OpenDocument("file:///test.rho", "module x\nglobal_fn")
	ws.Recompute("file:///test.rho")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      41,
		Method:  "textDocument/hover",
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

	resp := HandleHover(req, ws)
	data, _ := json.Marshal(resp.Result)
	out := string(data)

	if !strings.Contains(out, "global_fn (global symbol)") {
		t.Fatalf("hover missing global symbol: %s", out)
	}
}

func TestHoverEmpty(t *testing.T) {
	ws := state.NewWorkspace()

	ws.OpenDocument("file:///test.rho", "module x")
	ws.Recompute("file:///test.rho")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      42,
		Method:  "textDocument/hover",
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

	resp := HandleHover(req, ws)
	if resp.Result != nil {
		t.Fatalf("expected empty hover, got: %+v", resp.Result)
	}
}
