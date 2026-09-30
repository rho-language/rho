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

	"github.com/rho/rho-lsp/src/protocol"
	"github.com/rho/rho-lsp/src/state"
)

/* -------------------------------------------------------------------------- */
/*  HandleReferences                                                          */
/* -------------------------------------------------------------------------- */

func HandleReferences(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.ReferenceParams
	_ = json.Unmarshal(raw, &params)

	normalizedPath := state.NormalizeURI(params.TextDocument.URI)
	pos := params.Position

	doc := ws.GetDocument(normalizedPath)
	if doc == nil {
		return emptyReferences(req)
	}

	// Find AST node at cursor
	node, found := doc.NodeAt(pos.Line, pos.Character)
	if !found {
		return emptyReferences(req)
	}

	name := node.Name
	if name == "" {
		return emptyReferences(req)
	}

	// Collect references
	refs := []protocol.Location{}
	roleKey := string(node.Role) + ":" + name
	if roleRefs := doc.Symbols.RoleRefs[roleKey]; len(roleRefs) > 0 {
		refs = append(refs, roleRefs...)
	}

	// Local references.
	//
	// Every one of them: a guard here that stopped after the first made "find
	// references" answer with one reference, which is a number that looks like an
	// answer and is not one.
	refs = append(refs, doc.Symbols.LocalRefs[name]...)

	// Global references
	for _, loc := range ws.GlobalSymbols.References[roleKey] {
		refs = append(refs, loc)
	}

	// A declaration is recorded as a declaration and as a reference to itself,
	// which is how the symbol table knows both. Asked for references, that means
	// every one of them is answered twice.
	refs = dedupeLocations(refs)
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
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

func emptyReferences(req protocol.Request) *protocol.Response {
	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  []protocol.Location{},
	}
}

// dedupeLocations is the same place once.
func dedupeLocations(locations []protocol.Location) []protocol.Location {
	seen := make(map[protocol.Location]bool, len(locations))
	out := make([]protocol.Location, 0, len(locations))
	for _, location := range locations {
		if seen[location] {
			continue
		}
		seen[location] = true
		out = append(out, location)
	}
	return out
}
