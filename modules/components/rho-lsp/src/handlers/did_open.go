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

	// The original URI is what OpenDocument takes, not a path already reduced to
	// one: it normalises internally and keeps what it was given as the URI that
	// every location in a response is written against. Handing it a normalised
	// path instead stored the path in place of the URI, and a client matching a
	// definition against its own document has nothing to match.
	ws.OpenDocument(doc.URI, doc.Text)

	// Recompute normalises the URI it is given, so the original is passed here
	// too rather than a path derived from it.
	diags := ws.Recompute(doc.URI)

	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"diagnostics": diags,
		},
	}
}
