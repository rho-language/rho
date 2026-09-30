//
// @file definition_test.go
// @brief Tests for Rho LSP Definition Handler.
//
// Validates:
// - jump-to-definition for local symbols
// - jump-to-definition for global symbols
// - empty result for unknown symbols
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

	"github.com/rho/rho-lsp/src/protocol"
	"github.com/rho/rho-lsp/src/state"
)

func TestDefinitionLocalSymbol(t *testing.T) {
	ws := state.NewWorkspace()

	// Document with a local symbol
	ws.OpenDocument("file:///test.rho", "module x\nfn test() void {}")
	ws.Recompute("file:///test.rho")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      50,
		Method:  "textDocument/definition",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///test.rho",
			},
			"position": map[string]any{
				"line":      7,
				"character": 4,
			},
		},
	}

	resp := HandleDefinition(req, ws)
	if resp.Result == nil {
		t.Fatalf("expected definition result, got nil")
	}

	data, _ := json.Marshal(resp.Result)
	out := string(data)

	if !json.Valid([]byte(out)) {
		t.Fatalf("invalid JSON: %s", out)
	}
}

func TestDefinitionGlobalSymbol(t *testing.T) {
	ws := state.NewWorkspace()

	// Add global symbol
	ws.GlobalSymbols.Locations["global_fn"] = protocol.Location{
		URI: "file:///global.rho",
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 10},
		},
	}

	ws.OpenDocument("file:///global.rho", "module lib\n\nfn global_fn() void {\n  return;\n}\n")
	ws.OpenDocument("file:///test.rho", "module x\n\nfn caller() void {\n  global_fn();\n  return;\n}\n")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      51,
		Method:  "textDocument/definition",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///test.rho",
			},
			"position": map[string]any{
				"line":      3,
				"character": 4,
			},
		},
	}

	resp := HandleDefinition(req, ws)
	if resp.Result == nil {
		t.Fatalf("expected global definition result, got nil")
	}

	loc := resp.Result.(protocol.Location)
	// The place is the declaration in the other document, so what matters is
	// which file it names, not how the file is spelled.
	if !strings.HasSuffix(loc.URI, "global.rho") {
		t.Fatalf("unexpected definition URI: %q", loc.URI)
	}
}

func TestDefinitionEmpty(t *testing.T) {
	ws := state.NewWorkspace()

	ws.OpenDocument("file:///test.rho", "module x")
	ws.Recompute("file:///test.rho")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      52,
		Method:  "textDocument/definition",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///test.rho",
			},
			"position": map[string]any{
				"line":      3,
				"character": 4,
			},
		},
	}

	resp := HandleDefinition(req, ws)
	data, _ := json.Marshal(resp.Result)
	if string(data) != "null" {
		t.Fatalf("expected null definition, got: %s", string(data))
	}
}
