//
// @file documents.go
// @brief Rho LSP Document Manager - Enterprise v1.
//
// Manages:
//   - document text
//   - tokenization
//   - parsing (AST)
//   - diagnostics
//   - symbol extraction
//   - node lookup for hover/definition/references
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package state

import (
	"unicode/utf8"

	"github.com/rho/rho-lsp/src/analysis"
	"github.com/rho/rho-lsp/src/protocol"
)

/* -------------------------------------------------------------------------- */
/*  Document                                                                   */
/* -------------------------------------------------------------------------- */

type Document struct {
	URI     string
	Text    string
	Version int
	Tokens  []analysis.Token
	AST     *analysis.Module
	Indexer *analysis.Indexer
	Symbols *SymbolTable
	Diags   []protocol.Diagnostic
}

/* -------------------------------------------------------------------------- */
/*  Constructors                                                               */
/* -------------------------------------------------------------------------- */

func NewDocument(uri, text string) *Document {
	doc := &Document{
		URI:     uri,
		Text:    text,
		Tokens:  []analysis.Token{},
		AST:     nil,
		Symbols: NewSymbolTable(),
		Diags:   []protocol.Diagnostic{},
	}

	doc.Recompute()
	return doc
}

/* -------------------------------------------------------------------------- */
/*  Update                                                                     */
/* -------------------------------------------------------------------------- */

func (d *Document) Update(text string) {
	d.Text = text
	d.Recompute()
}

func (d *Document) ApplyChange(change protocol.TextDocumentContentChangeEvent) {
	if change.Range == nil {
		d.Update(change.Text)
		return
	}
	start, end := offsetAt(d.Text, change.Range.Start), offsetAt(d.Text, change.Range.End)
	if start > end {
		start, end = end, start
	}
	d.Update(d.Text[:start] + change.Text + d.Text[end:])
}

/* -------------------------------------------------------------------------- */
/*  Recompute                                                                   */
/* -------------------------------------------------------------------------- */

func (d *Document) Recompute() {
	// Lex
	d.Tokens = analysis.Lex(d.Text)

	// Parse
	var parseDiags []analysis.Diagnostic
	d.AST, parseDiags = analysis.Parse(d.Tokens)
	d.AST.URI = d.URI
	d.Diags = make([]protocol.Diagnostic, 0, len(parseDiags))
	d.Diags = append(d.Diags, analysisDiagnostics(parseDiags)...)
	// analysis.Analyze is scope-unaware (no params/let/imports/fields), so it
	// is not wired into live diagnostics to avoid false "undefined symbol" noise.

	// Extract symbols
	d.Symbols = ExtractSymbols(d.AST)
	d.Indexer = analysis.BuildIndexer(d.AST)
}

func offsetAt(text string, position protocol.Position) int {
	line, column := 0, 0
	for index := 0; index < len(text); {
		if line == position.Line && column >= position.Character {
			return index
		}
		if text[index] == '\n' {
			line++
			column = 0
		} else {
			r, size := utf8.DecodeRuneInString(text[index:])
			if r > 0xffff {
				column += 2
			} else {
				column++
			}
			index += size
			continue
		}
		index++
	}
	return len(text)
}

func analysisDiagnostics(diags []analysis.Diagnostic) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(diags))
	for _, diagnostic := range diags {
		out = append(out, protocol.Diagnostic{Range: protocol.Range{Start: protocol.Position{Line: diagnostic.Line, Character: diagnostic.Column}, End: protocol.Position{Line: diagnostic.Line, Character: diagnostic.Column + 1}}, Severity: 1, Message: diagnostic.Message})
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  NodeAt                                                                      */
/* -------------------------------------------------------------------------- */

func (d *Document) NodeAt(line, col int) analysis.Node {
	if d.Indexer == nil {
		return nil
	}
	return d.Indexer.NodeAt(line, col)
}
