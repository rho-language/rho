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
// SPDX-License-Identifier: MIT
//

package handlers

import (
	"encoding/json"
	"rho/cmd/rho-lsp/protocol"
	"rho/cmd/rho-lsp/state"
)

/* -------------------------------------------------------------------------- */
/*  HandleDidChange                                                            */
/* -------------------------------------------------------------------------- */

func HandleDidChange(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.DidChangeTextDocumentParams
	_ = json.Unmarshal(raw, &params)

	uri := params.TextDocument.URI

	if len(params.ContentChanges) == 0 {
		return &protocol.Response{
			JsonRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"diagnostics": []any{}},
		}
	}

	// Full text replacement (LSP v1)
	ws.ApplyChanges(uri, params.TextDocument.Version, params.ContentChanges)

	// Recompute AST + diagnostics
	diags := ws.Recompute(uri)

	result := map[string]any{
		"diagnostics": diags,
	}

	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}
