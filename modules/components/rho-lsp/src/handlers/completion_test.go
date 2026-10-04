//
// @file completion_test.go
// @brief Tests for Rho LSP Completion Handler.
//
// Validates:
// - keywords appear in completion
// - local symbols appear in completion
// - global symbols appear in completion
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

func TestHandleCompletionKeywords(t *testing.T) {
	ws := state.NewWorkspace()
	ws.OpenDocument("file:///test.rho", "module x")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      30,
		Method:  "textDocument/completion",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///test.rho",
			},
			"position": map[string]any{
				"line":      0,
				"character": 0,
			},
		},
	}

	resp := HandleCompletion(req, ws)
	if resp == nil {
		t.Fatalf("expected response, got nil")
	}

	data, _ := json.Marshal(resp.Result)
	out := string(data)

	if !strings.Contains(out, "module") {
		t.Fatalf("missing keyword completion: %s", out)
	}
}

func TestHandleCompletionLocalSymbols(t *testing.T) {
	ws := state.NewWorkspace()

	// Document with a local symbol
	ws.OpenDocument("file:///test.rho", "module x\nfn test() void {}")
	ws.Recompute("file:///test.rho")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      31,
		Method:  "textDocument/completion",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///test.rho",
			},
			"position": map[string]any{
				"line":      1,
				"character": 3,
			},
		},
	}

	resp := HandleCompletion(req, ws)
	data, _ := json.Marshal(resp.Result)
	out := string(data)

	if !strings.Contains(out, "test") {
		t.Fatalf("missing local symbol completion: %s", out)
	}
}

func TestHandleCompletionGlobalSymbols(t *testing.T) {
	ws := state.NewWorkspace()

	// Add global symbol manually
	ws.GlobalSymbols.All["global_fn"] = true

	ws.OpenDocument("file:///test.rho", "module x")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      32,
		Method:  "textDocument/completion",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///test.rho",
			},
			"position": map[string]any{
				"line":      0,
				"character": 0,
			},
		},
	}

	resp := HandleCompletion(req, ws)
	data, _ := json.Marshal(resp.Result)
	out := string(data)

	if !strings.Contains(out, "global_fn") {
		t.Fatalf("missing global symbol completion: %s", out)
	}
}
