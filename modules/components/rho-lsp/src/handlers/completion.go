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
// SPDX-License-Identifier: Apache-2.0
//

package handlers

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/rho/rho-lsp/src/protocol"
	"github.com/rho/rho-lsp/src/state"
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

	items := []protocol.CompletionItem{}
	lineText := ""
	if params.Position.Line >= 0 {
		lines := strings.Split(doc.Text, "\n")
		if params.Position.Line < len(lines) {
			lineText = lines[params.Position.Line][:minInt(params.Position.Character, len(lines[params.Position.Line]))]
		}
	}
	prefix := completionPrefix(lineText)

	// After a dot, what a caller wants is a member, not a word. The server does
	// not resolve the type before the dot, so it offers every field it knows and
	// names the struct each came from — which is wrong often and useful always,
	// where returning nothing is wrong always and useful never. Advertising the
	// dot and answering it with nothing is the worst of the three.
	if strings.HasSuffix(strings.TrimSpace(lineText), ".") {
		return memberCompletions(ws, doc, req, prefix)
	}
	members := false // member completion has been answered above; nothing here is one

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

// memberCompletions are the fields of every struct the workspace knows, offered
// after a dot.
func memberCompletions(ws *state.Workspace, doc *state.Document, req protocol.Request, prefix string) *protocol.Response {
	items := []protocol.CompletionItem{}
	seen := map[string]bool{}
	for _, entry := range ws.MemberFields() {
		if prefix != "" && !strings.HasPrefix(entry.Name, prefix) {
			continue
		}
		// The same field name on two structs is one completion offered twice, and
		// a list with the same word in it twice is a list a reader has to read
		// twice.
		if seen[entry.Name] {
			continue
		}
		seen[entry.Name] = true
		items = append(items, protocol.CompletionItem{
			Label:  entry.Name,
			Kind:   5, // field
			Detail: "field of " + entry.Struct,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return &protocol.Response{JsonRPC: "2.0", ID: req.ID, Result: items}
}
