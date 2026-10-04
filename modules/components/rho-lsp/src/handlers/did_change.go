//
// @file did_change.go
// @brief Rho LSP DidChange Handler - Enterprise v1.
//
// Implements the `textDocument/didChange` request.
// Updates the document text in the workspace, reparses it, and returns diagnostics.
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

	"github.com/rho/rho-lsp/src/protocol"
	"github.com/rho/rho-lsp/src/state"
)

/* -------------------------------------------------------------------------- */
/*  HandleDidChange                                                           */
/* -------------------------------------------------------------------------- */

func HandleDidChange(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.DidChangeTextDocumentParams
	_ = json.Unmarshal(raw, &params)

	normalizedPath := state.NormalizeURI(params.TextDocument.URI)

	if len(params.ContentChanges) == 0 {
		return &protocol.Response{
			JsonRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"diagnostics": []any{}},
		}
	}

	// Full text replacement (LSP v1) using normalized path key
	ws.ApplyChanges(normalizedPath, params.TextDocument.Version, params.ContentChanges)

	// Recompute AST + diagnostics using normalized path key
	diags := ws.Recompute(normalizedPath)

	result := map[string]any{
		"diagnostics": diags,
	}

	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}
