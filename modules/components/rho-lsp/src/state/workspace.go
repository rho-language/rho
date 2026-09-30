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
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rho/rho-lsp/src/protocol"
)

/* -------------------------------------------------------------------------- */
/*  URI Normalization Helper                                                  */
/* -------------------------------------------------------------------------- */

// NormalizeURI converts file URIs, WSL remote URIs, or raw paths into a uniform canonical path key.
func NormalizeURI(uri string) string {
	// 1. Use PathUnescape (NOT QueryUnescape) to preserve '+' in wsl+ubuntu schemes
	if decoded, err := url.PathUnescape(uri); err == nil {
		uri = decoded
	}

	// 2. Handle schemes (vscode-remote://, file://, etc.)
	if idx := strings.Index(uri, "://"); idx != -1 {
		scheme := uri[:idx]
		if scheme != "file" {
			// Find where the absolute path starts after authority
			pathStart := strings.Index(uri[idx+3:], "/")
			if pathStart != -1 {
				uri = uri[idx+3+pathStart:]
			}
		} else {
			uri = strings.TrimPrefix(uri, "file://")
		}
	}

	// 3. Clean Windows drive letter formatting if applicable (/c:/path -> C:/path)
	uri = filepath.Clean(uri)
	return filepath.ToSlash(uri)
}

/* -------------------------------------------------------------------------- */
/*  Workspace                                                                 */
/* -------------------------------------------------------------------------- */

type Workspace struct {
	mu              sync.RWMutex
	Documents       map[string]*Document // Key: Normalized Canonical Path
	URIMap          map[string]string    // Key: Normalized Path -> Value: Original VS Code URI
	GlobalSymbols   *GlobalSymbolTable
	ExternalSymbols *GlobalSymbolTable
	// SemanticTokens is the last result for a document, kept so that a request
	// for the same text twice is not computed twice. It is read and written
	// through the workspace's own lock, like everything else here: a cache is
	// exactly the kind of thing two requests touch at once, and an
	// unsynchronised map on a locked struct is worse than no lock at all, because
	// it looks correct.
	SemanticTokens map[string][]uint32
	Roots          []string
}

// SemanticTokensFor is the last tokens computed for a document.
func (ws *Workspace) SemanticTokensFor(path string) ([]uint32, bool) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	tokens, found := ws.SemanticTokens[path]
	return tokens, found
}

// SetSemanticTokens records the tokens computed for a document.
func (ws *Workspace) SetSemanticTokens(path string, tokens []uint32) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.SemanticTokens[path] = tokens
}

// ForgetSemanticTokens drops what was computed for a document, which is what a
// close has to do: a cache that outlives its document is a document's tokens
// answered to a file that no longer has any.
func (ws *Workspace) ForgetSemanticTokens(path string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	delete(ws.SemanticTokens, path)
}

func NewWorkspace() *Workspace {
	return &Workspace{
		Documents:      map[string]*Document{},
		URIMap:         map[string]string{},
		GlobalSymbols:  NewGlobalSymbolTable(),
		SemanticTokens: map[string][]uint32{},
	}
}

/* -------------------------------------------------------------------------- */
/*  Document Management                                                       */
/* -------------------------------------------------------------------------- */

func (ws *Workspace) OpenDocument(uri, text string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	normURI := NormalizeURI(uri)
	ws.URIMap[normURI] = uri // Remember original URI for LSP responses

	// Whatever the global table already holds is kept aside, because the table is
	// rebuilt from the open documents below and a rebuild would otherwise drop
	// every name that did not come from one. A symbol the server was given some
	// other way — a caller that seeded the workspace, a scan that has not run
	// yet — is a symbol a person is still typing, and losing it because they
	// opened a file is the one failure completion cannot recover from.
	if len(ws.Documents) == 0 && ws.GlobalSymbols != nil {
		ws.ExternalSymbols = ws.GlobalSymbols
	}

	doc := NewDocument(normURI, text)
	doc.URI = uri // Preserve original URI in Document
	ws.Documents[normURI] = doc
	ws.updateGlobalSymbols()
}

func (ws *Workspace) UpdateDocument(uri, text string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	normURI := NormalizeURI(uri)
	ws.URIMap[normURI] = uri

	if doc, ok := ws.Documents[normURI]; ok {
		doc.Update(text)
		ws.updateGlobalSymbols()
	}
}

func (ws *Workspace) ApplyChanges(uri string, version int, changes []protocol.TextDocumentContentChangeEvent) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	normURI := NormalizeURI(uri)
	if doc, ok := ws.Documents[normURI]; ok {
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

	normURI := NormalizeURI(uri)
	delete(ws.Documents, normURI)
	delete(ws.URIMap, normURI)
	delete(ws.SemanticTokens, normURI)
	ws.updateGlobalSymbols()
}

func (ws *Workspace) GetDocument(uri string) *Document {
	normURI := NormalizeURI(uri)

	ws.mu.RLock()
	doc, exists := ws.Documents[normURI]
	ws.mu.RUnlock()

	if exists {
		return doc
	}

	// Disk Fallback: Attempt to load from disk if file is not currently in memory
	if loadedDoc, err := ws.LoadDocumentFromFile(uri); err == nil {
		return loadedDoc
	}

	return nil
}

func (ws *Workspace) LoadDocumentFromFile(rawURIOrPath string) (*Document, error) {
	normURI := NormalizeURI(rawURIOrPath)

	// Quick check if already loaded
	ws.mu.RLock()
	if doc, ok := ws.Documents[normURI]; ok {
		ws.mu.RUnlock()
		return doc, nil
	}
	ws.mu.RUnlock()

	// Perform disk read WITHOUT holding the mutex lock
	content, err := os.ReadFile(normURI)
	if err != nil {
		return nil, err
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Double-check after acquiring write lock
	if doc, ok := ws.Documents[normURI]; ok {
		return doc, nil
	}

	doc := NewDocument(normURI, string(content))
	doc.URI = rawURIOrPath
	ws.Documents[normURI] = doc
	ws.URIMap[normURI] = rawURIOrPath
	ws.updateGlobalSymbols()

	return doc, nil
}

// GetOriginalURI returns the VS Code URI associated with a normalized path
func (ws *Workspace) GetOriginalURI(normURI string) string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	if orig, ok := ws.URIMap[normURI]; ok {
		return orig
	}
	return "file://" + normURI
}

func (ws *Workspace) SetRoots(roots []string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.Roots = make([]string, 0, len(roots))
	for _, root := range roots {
		ws.Roots = append(ws.Roots, NormalizeURI(root))
	}
}

/* -------------------------------------------------------------------------- */
/*  Recompute                                                                 */
/* -------------------------------------------------------------------------- */

func (ws *Workspace) Recompute(uri string) []any {
	normURI := NormalizeURI(uri)

	doc := ws.GetDocument(normURI)
	if doc == nil {
		return []any{}
	}

	doc.Recompute()

	ws.mu.Lock()
	ws.updateGlobalSymbols()
	ws.mu.Unlock()

	// Convert diagnostics to LSP format
	out := make([]any, 0, len(doc.Diags))
	for _, d := range doc.Diags {
		out = append(out, d)
	}

	return out
}

/* -------------------------------------------------------------------------- */
/*  Global Symbol Indexing                                                    */
/* -------------------------------------------------------------------------- */

func (ws *Workspace) updateGlobalSymbols() {
	ws.GlobalSymbols = NewGlobalSymbolTable()

	// The names the open documents declare, so a document's own declaration wins
	// over one that was only ever in the table that was kept aside.
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

// MemberField is a struct field the workspace knows about, and the struct it
// came from.
type MemberField struct {
	Struct string
	Name   string
	Line   int
	Column int
}

// MemberFields is every struct field declared in the documents the server has
// been given.
//
// It walks the open documents rather than the workspace on disk, because this
// server reads what a client sends it and nothing else: a project it has not been
// shown is a project it has no opinion about.
func (ws *Workspace) MemberFields() []MemberField {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	out := []MemberField{}
	for _, doc := range ws.Documents {
		if doc == nil {
			continue
		}
		for _, field := range doc.Fields {
			out = append(out, MemberField{
				Struct: field.Struct,
				Name:   field.Name,
				Line:   field.Line,
				Column: field.Column,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Struct < out[j].Struct
	})
	return out
}
