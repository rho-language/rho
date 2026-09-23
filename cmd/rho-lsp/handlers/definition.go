//
// @file definition.go
// @brief Rho LSP Definition Handler - Enterprise v1.
//
// Implements the `textDocument/definition` request.
// Finds the declaration location of a symbol at the cursor position.
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
/*  HandleDefinition                                                           */
/* -------------------------------------------------------------------------- */

func HandleDefinition(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.DefinitionParams
	_ = json.Unmarshal(raw, &params)

	uri := params.TextDocument.URI
	pos := params.Position

	doc := ws.GetDocument(uri)
	if doc == nil {
		return emptyDefinition(req)
	}

	// Find AST node at cursor
	node := doc.NodeAt(pos.Line, pos.Character)
	if node == nil {
		return emptyDefinition(req)
	}

	name := node.Name()
	if name == "" {
		return emptyDefinition(req)
	}

	// Check local symbols
	if loc, ok := doc.Symbols.LocalLocations[name]; ok {
		return definitionResponse(req, loc)
	}

	// Check global symbols
	if loc, ok := ws.GlobalSymbols.Locations[name]; ok {
		return definitionResponse(req, loc)
	}

	return emptyDefinition(req)
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func emptyDefinition(req protocol.Request) *protocol.Response {
	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  json.RawMessage("null"),
	}
}

func definitionResponse(req protocol.Request, loc protocol.Location) *protocol.Response {
	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  loc,
	}
}
