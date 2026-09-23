//
// @file workspace.go
// @brief Rho LSP Workspace Manager - Enterprise v1.
//
// Manages:
//   - all open documents
//   - global symbol table
//   - cross-file indexing
//   - parse + analyze orchestration
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package state

import (
	"rho/cmd/rho-lsp/protocol"
	"sync"
)

/* -------------------------------------------------------------------------- */
/*  Workspace                                                                  */
/* -------------------------------------------------------------------------- */

type Workspace struct {
	mu              sync.RWMutex
	Documents       map[string]*Document
	GlobalSymbols   *GlobalSymbolTable
	ExternalSymbols *GlobalSymbolTable
	SemanticTokens  map[string][]uint32
	Roots           []string
}

func NewWorkspace() *Workspace {
	return &Workspace{
		Documents:      map[string]*Document{},
		GlobalSymbols:  NewGlobalSymbolTable(),
		SemanticTokens: map[string][]uint32{},
	}
}

/* -------------------------------------------------------------------------- */
/*  Document Management                                                         */
/* -------------------------------------------------------------------------- */

func (ws *Workspace) OpenDocument(uri, text string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if len(ws.Documents) == 0 && ws.GlobalSymbols != nil {
		ws.ExternalSymbols = ws.GlobalSymbols
	}
	doc := NewDocument(uri, text)
	ws.Documents[uri] = doc
	ws.updateGlobalSymbols()
}

func (ws *Workspace) UpdateDocument(uri, text string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if doc, ok := ws.Documents[uri]; ok {
		doc.Update(text)
		ws.updateGlobalSymbols()
	}
}

func (ws *Workspace) ApplyChanges(uri string, version int, changes []protocol.TextDocumentContentChangeEvent) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if doc, ok := ws.Documents[uri]; ok {
		for _, change := range changes {
			doc.ApplyChange(change)
		}
		doc.Version = version
		ws.updateGlobalSymbols()
	}
}

func (ws *Workspace) CloseDocument(uri string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	delete(ws.Documents, uri)
	ws.updateGlobalSymbols()
}

func (ws *Workspace) GetDocument(uri string) *Document {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.Documents[uri]
}

func (ws *Workspace) SetRoots(roots []string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.Roots = append([]string{}, roots...)
}

/* -------------------------------------------------------------------------- */
/*  Recompute                                                                   */
/* -------------------------------------------------------------------------- */

func (ws *Workspace) Recompute(uri string) []any {
	doc := ws.Documents[uri]
	if doc == nil {
		return []any{}
	}

	doc.Recompute()
	ws.updateGlobalSymbols()

	// Convert diagnostics to LSP format
	out := []any{}
	for _, d := range doc.Diags {
		out = append(out, d)
	}

	return out
}

/* -------------------------------------------------------------------------- */
/*  Global Symbol Indexing                                                      */
/* -------------------------------------------------------------------------- */

func (ws *Workspace) updateGlobalSymbols() {
	ws.GlobalSymbols = NewGlobalSymbolTable()
	currentNames := map[string]bool{}
	for _, doc := range ws.Documents {
		for name := range doc.Symbols.Local {
			currentNames[name] = true
		}
	}
	if ws.ExternalSymbols != nil {
		for name, value := range ws.ExternalSymbols.All {
			if !currentNames[name] {
				ws.GlobalSymbols.All[name] = value
			}
		}
		for name, value := range ws.ExternalSymbols.Locations {
			if !currentNames[name] {
				ws.GlobalSymbols.Locations[name] = value
			}
		}
		for name, value := range ws.ExternalSymbols.References {
			if !currentNames[name] {
				ws.GlobalSymbols.References[name] = append(ws.GlobalSymbols.References[name], value...)
			}
		}
	}

	for _, doc := range ws.Documents {
		for name := range doc.Symbols.Local {
			ws.GlobalSymbols.All[name] = true
			ws.GlobalSymbols.Locations[name] = doc.Symbols.LocalLocations[name]
		}

		for name, refs := range doc.Symbols.LocalRefs {
			ws.GlobalSymbols.References[name] =
				append(ws.GlobalSymbols.References[name], refs...)
		}
		for name, refs := range doc.Symbols.RoleRefs {
			ws.GlobalSymbols.References[name] = append(ws.GlobalSymbols.References[name], refs...)
		}
	}
}
