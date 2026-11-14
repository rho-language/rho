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
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/rho/rho-lsp/src/protocol"
	"rho/src"
	"rho/src/frontend/lexer"
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
//
// A file on disk is checked as `rho build` checks it: as part of the program
// its project's module.yml names, with that project's dependencies, the buffer
// standing in for what was last saved. So a name another file declares is known,
// a type error is reported, and the place underlined is the place the build
// prints. A buffer with no file behind it is read on its own.
func compilerDiagnostics(uri, text string) []protocol.Diagnostic {
	path := NormalizeURI(uri)
	var found rho.DocumentDiagnostics
	onDisk := false
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		onDisk = true
		found = rho.DiagnoseInProgram(path, text)
	} else {
		found = rho.ParseSource(uri, text)
	}
	return publishable(found.Diagnostics, path, onDisk, text)
}

// publishable turns the compiler's diagnostics into the protocol's, keeping
// the ones about this file.
//
// A mistake in another file belongs to that file and is published when it is
// open; it is shown here only when one of its notes points into this file — the
// include or import that brought the broken file in — and then at that line,
// naming the file it is in.
func publishable(diagnostics []rho.Diagnostic, path string, onDisk bool, text string) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		message := diagnostic.Message
		line, column := diagnostic.Line, diagnostic.Column
		endLine, endColumn := diagnostic.EndLine, diagnostic.EndColumn
		if onDisk && diagnostic.File != "" && filepath.ToSlash(diagnostic.File) != path {
			placed := false
			for _, note := range diagnostic.Notes {
				if filepath.ToSlash(note.File) == path {
					message = fmt.Sprintf("%s:%d:%d: %s", filepath.Base(diagnostic.File), diagnostic.Line, diagnostic.Column, diagnostic.Message)
					line, column, endLine, endColumn = note.Line, note.Column, 0, 0
					placed = true
					break
				}
			}
			if !placed {
				continue
			}
		}
		severity := 1
		if diagnostic.Severity == rho.SeverityWarning {
			severity = 2
		}
		// The compiler counts from one and the protocol from zero. A finding with
		// no place still has to be published; the first line is where an editor
		// will show it, and a blank entry would be dropped.
		start := protocol.Position{Line: max(line-1, 0), Character: max(column-1, 0)}
		end := protocol.Position{Line: start.Line, Character: start.Character + tokenLength(text, start)}
		if endLine > 0 {
			end = protocol.Position{Line: endLine - 1, Character: max(endColumn-1, 0)}
		}
		published := protocol.Diagnostic{
			Range:    protocol.Range{Start: start, End: end},
			Severity: severity,
			Code:     diagnostic.Code,
			Source:   "rho",
			Message:  message,
		}
		for _, note := range diagnostic.Notes {
			if note.File == "" || note.Line == 0 {
				continue
			}
			at := protocol.Position{Line: note.Line - 1, Character: max(note.Column-1, 0)}
			published.RelatedInformation = append(published.RelatedInformation, protocol.DiagnosticRelatedInformation{
				Location: protocol.Location{URI: "file://" + filepath.ToSlash(note.File), Range: protocol.Range{Start: at, End: at}},
				Message:  note.Message,
			})
		}
		out = append(out, published)
	}
	return out
}

// tokenLength is how many characters the token at a position spans, so the
// underline covers the name or literal the compiler pointed at rather than its
// first character. It counts as the protocol does, in UTF-16 units.
func tokenLength(text string, at protocol.Position) int {
	lines := strings.Split(text, "\n")
	if at.Line >= len(lines) {
		return 1
	}
	runes := []rune(lines[at.Line])
	if at.Character >= len(runes) {
		return 1
	}
	isWord := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
	i := at.Character
	switch {
	case runes[i] == '"':
		i++
		for i < len(runes) && runes[i] != '"' {
			if runes[i] == '\\' {
				i++
			}
			i++
		}
		i++
	case isWord(runes[i]):
		for i < len(runes) && isWord(runes[i]) {
			i++
		}
	default:
		i++
	}
	if i > len(runes) {
		i = len(runes)
	}
	return len(utf16.Encode(runes[at.Character:i]))
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
