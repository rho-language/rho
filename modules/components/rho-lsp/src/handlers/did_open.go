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

	"github.com/rho/rho-lsp/src/protocol"
	"github.com/rho/rho-lsp/src/state"
)

/* -------------------------------------------------------------------------- */
/*  HandleDidOpen                                                             */
/* -------------------------------------------------------------------------- */

func HandleDidOpen(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.DidOpenTextDocumentParams
	_ = json.Unmarshal(raw, &params)

	doc := params.TextDocument
	normalizedPath := state.NormalizeURI(doc.URI)

	// Add document to workspace using normalized path key
	ws.OpenDocument(normalizedPath, doc.Text)

	// Parse + analyze using normalized path key
	diags := ws.Recompute(normalizedPath)

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