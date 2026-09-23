//
// @file initialize.go
// @brief Rho LSP Initialize Handler - Enterprise v1.
//
// Implements the `initialize` request for the Rho Language Server.
// Returns server capabilities and prepares the workspace.
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
/*  HandleInitialize                                                           */
/* -------------------------------------------------------------------------- */

func HandleInitialize(req protocol.Request, workspaces ...*state.Workspace) *protocol.Response {
	var params protocol.InitializeParams
	raw, _ := json.Marshal(req.Params)
	_ = json.Unmarshal(raw, &params)
	if len(workspaces) > 0 {
		roots := []string{}
		if params.RootURI != "" {
			roots = append(roots, params.RootURI)
		}
		for _, folder := range params.WorkspaceFolders {
			roots = append(roots, folder.URI)
		}
		workspaces[0].SetRoots(roots)
	}
	capabilities := map[string]any{
		"textDocumentSync": map[string]any{
			"openClose": true,
			"change":    1, // Full sync
		},
		"completionProvider": map[string]any{
			"triggerCharacters": []string{"."},
		},
		"hoverProvider":      true,
		"definitionProvider": true,
		"referencesProvider": true,
		"semanticTokensProvider": map[string]any{
			"full": true,
			"legend": map[string]any{
				"tokenTypes": []string{
					"keyword", "type", "function", "variable",
					"field", "literal", "operator",
				},
				"tokenModifiers": []string{"readonly", "static", "deprecated"},
			},
		},
	}

	result := map[string]any{
		"capabilities": capabilities,
	}

	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}
