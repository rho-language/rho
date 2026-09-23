//
// @file did_open.go
// @brief Rho LSP DidOpen Handler - Enterprise v1.
//
// Implements the `textDocument/didOpen` request.
// Loads the document into the workspace, parses it, and returns diagnostics.
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
/*  HandleDidOpen                                                              */
/* -------------------------------------------------------------------------- */

func HandleDidOpen(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.DidOpenTextDocumentParams
	_ = json.Unmarshal(raw, &params)

	doc := params.TextDocument

	// Add document to workspace
	ws.OpenDocument(doc.URI, doc.Text)

	// Parse + analyze
	diags := ws.Recompute(doc.URI)

	// LSP diagnostics response
	result := map[string]any{
		"diagnostics": diags,
	}

	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}
