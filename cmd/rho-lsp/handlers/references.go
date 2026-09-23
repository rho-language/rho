//
// @file references.go
// @brief Rho LSP References Handler - Enterprise v1.
//
// Implements the `textDocument/references` request.
// Finds all references to a symbol across the workspace.
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
/*  HandleReferences                                                           */
/* -------------------------------------------------------------------------- */

func HandleReferences(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.ReferenceParams
	_ = json.Unmarshal(raw, &params)

	uri := params.TextDocument.URI
	pos := params.Position

	doc := ws.GetDocument(uri)
	if doc == nil {
		return emptyReferences(req)
	}

	// Find AST node at cursor
	node := doc.NodeAt(pos.Line, pos.Character)
	if node == nil {
		return emptyReferences(req)
	}

	name := node.Name()
	if name == "" {
		return emptyReferences(req)
	}

	// Collect references
	refs := []protocol.Location{}
	roleKey := node.Role() + ":" + name
	if roleRefs := doc.Symbols.RoleRefs[roleKey]; len(roleRefs) > 0 {
		refs = append(refs, roleRefs...)
	}

	// Local references
	for _, loc := range doc.Symbols.LocalRefs[name] {
		if len(refs) == 0 {
			refs = append(refs, loc)
		}
	}

	// Global references
	for _, loc := range ws.GlobalSymbols.References[roleKey] {
		refs = append(refs, loc)
	}
	if len(refs) == 0 {
		for _, loc := range ws.GlobalSymbols.References[name] {
			refs = append(refs, loc)
		}
	}

	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  refs,
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func emptyReferences(req protocol.Request) *protocol.Response {
	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  []protocol.Location{},
	}
}
