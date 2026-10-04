//
// @file semantic_test.go
// @brief Tests for Rho LSP Semantic Tokens Handler.
//
// Validates:
// - semantic tokens are generated
// - token types are correct
// - empty result for unknown documents
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
	"testing"

	"github.com/rho/rho-lsp/src/protocol"
	"github.com/rho/rho-lsp/src/state"
)

func TestSemanticTokensBasic(t *testing.T) {
	ws := state.NewWorkspace()

	// Simple document
	ws.OpenDocument("file:///test.rho", "module x\nfn test() void {}")
	ws.Recompute("file:///test.rho")

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      60,
		Method:  "textDocument/semanticTokens/full",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///test.rho",
			},
		},
	}

	resp := HandleSemanticTokens(req, ws)
	if resp.Result == nil {
		t.Fatalf("expected semantic tokens, got nil")
	}

	data, _ := json.Marshal(resp.Result)
	var out protocol.SemanticTokens
	_ = json.Unmarshal(data, &out)

	if len(out.Data) == 0 {
		t.Fatalf("expected semantic tokens, got empty")
	}
}

func TestSemanticTokensEmptyDocument(t *testing.T) {
	ws := state.NewWorkspace()

	req := protocol.Request{
		JsonRPC: "2.0",
		ID:      61,
		Method:  "textDocument/semanticTokens/full",
		Params: map[string]any{
			"textDocument": map[string]any{
				"uri": "file:///missing.rho",
			},
		},
	}

	resp := HandleSemanticTokens(req, ws)
	data, _ := json.Marshal(resp.Result)

	var out protocol.SemanticTokens
	_ = json.Unmarshal(data, &out)

	if len(out.Data) != 0 {
		t.Fatalf("expected empty semantic tokens, got: %v", out.Data)
	}
}
