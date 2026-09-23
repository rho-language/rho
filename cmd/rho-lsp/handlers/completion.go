//
// @file completion.go
// @brief Rho LSP Completion Handler - Enterprise v1.
//
// Implements the `textDocument/completion` request.
// Provides basic symbol completion using workspace symbol tables.
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
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  HandleCompletion                                                          */
/* -------------------------------------------------------------------------- */

func HandleCompletion(req protocol.Request, ws *state.Workspace) *protocol.Response {
	raw, _ := json.Marshal(req.Params)

	var params protocol.CompletionParams
	_ = json.Unmarshal(raw, &params)

	normalizedPath := state.NormalizeURI(params.TextDocument.URI)

	doc := ws.GetDocument(normalizedPath)
	if doc == nil {
		return &protocol.Response{
			JsonRPC: "2.0",
			ID:      req.ID,
			Result:  []any{},
		}
	}

	// Collect completion items
	items := []protocol.CompletionItem{}
	lineText := ""
	if params.Position.Line >= 0 {
		lines := strings.Split(doc.Text, "\n")
		if params.Position.Line < len(lines) {
			lineText = lines[params.Position.Line][:minInt(params.Position.Character, len(lines[params.Position.Line]))]
		}
	}
	prefix := completionPrefix(lineText)
	members := strings.HasSuffix(strings.TrimSpace(lineText), ".")

	// Keywords (minimal)
	keywords := []string{
		"module", "fn", "let", "return", "import",
	}

	for _, kw := range keywords {
		if members || (prefix != "" && !strings.HasPrefix(kw, prefix)) {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label: kw,
			Kind:  14, // keyword
		})
	}

	// Local symbols
	for name := range doc.Symbols.Local {
		if members || (prefix != "" && !strings.HasPrefix(name, prefix)) {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label:  name,
			Kind:   6, // variable/function
			Detail: "local symbol",
		})
	}

	// Global symbols
	for name := range ws.GlobalSymbols.All {
		if members || (prefix != "" && !strings.HasPrefix(name, prefix)) {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label:  name,
			Kind:   12, // function/type/module
			Detail: "global symbol",
		})
	}

	return &protocol.Response{
		JsonRPC: "2.0",
		ID:      req.ID,
		Result:  items,
	}
}

func completionPrefix(line string) string {
	index := len(line) - 1
	for index >= 0 && ((line[index] >= 'a' && line[index] <= 'z') || (line[index] >= 'A' && line[index] <= 'Z') || (line[index] >= '0' && line[index] <= '9') || line[index] == '_') {
		index--
	}
	return line[index+1:]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}