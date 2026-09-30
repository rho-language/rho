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
	"github.com/rho/rho-lsp/src/protocol"
	"rho/src"
	"rho/src/frontend/lexer"
	"unicode/utf8"
)

/* -------------------------------------------------------------------------- */
/*  Document                                                                   */
/* -------------------------------------------------------------------------- */

type Document struct {
	URI     string
	Text    string
	Version int
	// Tokens are the file's tokens, which is what the handler that highlights
	// with them reads. They come from the compiler's lexer, so a keyword this
	// server highlights is a keyword the compiler has.
	Tokens []lexer.Token
	// Index is what the file declares and where, read from the compiler's own
	// syntax tree.
	Index *Index
	// Symbols is that, in the shape the navigation handlers ask their questions
	// of.
	Symbols *SymbolTable
	Diags   []protocol.Diagnostic
	// Fields are the struct fields the file declares. A completion after a dot
	// offers these, which is the whole of what it can offer: this server does
	// not resolve a receiver's type, so it offers every field it knows and says
	// which struct each came from.
	Fields []Field
}

/* -------------------------------------------------------------------------- */
/*  Constructors                                                               */
/* -------------------------------------------------------------------------- */

func NewDocument(uri, text string) *Document {
	doc := &Document{
		URI:     uri,
		Text:    text,
		Tokens:  []lexer.Token{},
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
	d.Tokens = Tokens(d.Text)

	// The document is read the way the compiler reads it, so a squiggle here and
	// a build failure there are the same finding said twice by the same reading.
	//
	// A server with its own parser would be a second opinion about the language,
	// and would go stale the first time the language moved: a new keyword, a new
	// declaration, a new rule. Reading the compiler's front end is what makes the
	// two unable to disagree.
	d.Diags = compilerDiagnostics(d.URI, d.Text)

	// And what the file declares is read from the same tree, so a name a
	// completion offers is a name the compiler knows and a place it reports is a
	// place in the same coordinates everything else is in.
	d.Index = BuildIndex(d.URI, d.Text)
	if d.Index != nil {
		d.Symbols = d.Index.Symbols
		d.Fields = d.Index.Fields
	} else {
		d.Symbols = NewSymbolTable()
		d.Fields = nil
	}
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

// compilerDiagnostics is what the compiler makes of this text, in the shape a
// language server publishes.
func compilerDiagnostics(uri, text string) []protocol.Diagnostic {
	found := rho.ParseSource(uri, text)
	out := make([]protocol.Diagnostic, 0, len(found.Diagnostics))
	for _, diagnostic := range found.Diagnostics {
		severity := 1
		if diagnostic.Severity == "warning" {
			severity = 2
		}
		line, column := diagnostic.Line, diagnostic.Column
		if !diagnostic.Positioned() {
			// A finding with no place still has to be published; the first line is
			// where an editor will show it, and a blank entry would be dropped.
			line, column = 0, 0
		}
		out = append(out, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: line, Character: column},
				End:   protocol.Position{Line: line, Character: column + 1},
			},
			Severity: severity,
			Message:  diagnostic.Message,
		})
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  NodeAt                                                                      */
/* -------------------------------------------------------------------------- */

// NodeAt is what is under the cursor, if anything is.
func (d *Document) NodeAt(line, col int) (NamedPosition, bool) {
	if d.Index == nil {
		return NamedPosition{}, false
	}
	return d.Index.At(line, col)
}
