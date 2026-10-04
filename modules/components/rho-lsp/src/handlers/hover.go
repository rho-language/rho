//
// @file hover.go
// @brief Rho LSP Hover Handler - Enterprise v1.
//
// Implements the `textDocument/hover` request.
// Finds the AST node at the cursor position and returns basic hover info.
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
/*  HandleHover                                                               */
/* -------------------------------------------------------------------------- */

func HandleHover(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.HoverParams
	_ = json.Unmarshal(raw, &params)

	normalizedPath := state.NormalizeURI(params.TextDocument.URI)
	pos := params.Position

	doc := ws.GetDocument(normalizedPath)
	if doc == nil {
		return emptyHover(req)
	}

	// Find AST node at position
	node, found := doc.NodeAt(pos.Line, pos.Character)
	if !found {
		return emptyHover(req)
	}

	// Extract symbol name
	name := node.Name
	if name == "" {
		return emptyHover(req)
	}

	// Check local symbols
	if _, ok := doc.Symbols.Local[name]; ok {
		return hoverResponse(req, node, "local symbol")
	}

	// Check global symbols
	if _, ok := ws.GlobalSymbols.All[name]; ok {
		return hoverResponse(req, node, "global symbol")
	}

	// Fallback: show raw name
	return hoverResponse(req, node, "symbol")
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

func emptyHover(req protocol.Request) *protocol.Response {
	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  json.RawMessage("null"),
	}
}

func hoverResponse(req protocol.Request, node state.NamedPosition, detail string) *protocol.Response {
	name := node.Name
	// MarkupContent requires "kind"; without it clients silently drop the hover.
	contents := map[string]any{
		"kind":  "plaintext",
		"value": name + " (" + detail + ")",
	}
	start := protocol.Position{Line: node.Line, Character: node.Column}
	end := protocol.Position{Line: node.Line, Character: node.Column + len([]rune(name))}

	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result: protocol.Hover{
			Contents: contents,
			Range:    protocol.Range{Start: start, End: end},
		},
	}
}
