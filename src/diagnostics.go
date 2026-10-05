//
// @file diagnostics.go
// @brief What is wrong with a file, and where.
//
// Everything that reads a Rho file — the compiler, and a language server, and
// whatever else comes — wants the same thing: a list of mistakes, each with the
// place in the file it is in. Before this, that list existed only as an `error`
// with a message, and the place was either scraped out of the message or not
// there at all.
//
// This is the shape that does not have that problem. A `Diagnostic` is a place
// and a sentence, and it is what a caller should be given rather than an error
// it has to interpret.
//
// A diagnostic is about one file. A load that brings in an import reports that
// import's problems against that import, because a mistake in a library is not
// the caller's to fix and pointing at the caller's file sends them looking in the
// wrong place.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rho

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
	"rho/src/types"
)

// Severity is how bad a diagnostic is, using the names a language server speaks.
type Severity string

const (
	// SeverityError is a mistake that stops the file compiling.
	SeverityError Severity = "error"
	// SeverityWarning is something worth saying that would still compile.
	SeverityWarning Severity = "warning"
)

// Diagnostic is one thing wrong with a file, and where it is.
type Diagnostic struct {
	// Severity is how bad it is.
	Severity Severity `json:"severity"`
	// Message is what is wrong, in a sentence.
	Message string `json:"message"`
	// Line and Column are where it is, counting from one. They are zero when the
	// place is not known, which is a fact the caller should be able to see rather
	// than have to guess at.
	Line   int `json:"line"`
	Column int `json:"column"`
	// Scope names the declaration the problem is in, for a mistake that is about
	// the whole of one rather than about a place within it.
	Scope string `json:"scope,omitempty"`
}

// Positioned reports whether a diagnostic knows where it is.
func (d Diagnostic) Positioned() bool { return d.Line > 0 }

// Range is the diagnostic as a range, which is what a language server wants to
// underline.
func (d Diagnostic) Range() Range {
	return Range{
		Start: Position{Line: d.Line, Column: d.Column},
		End:   Position{Line: d.Line, Column: d.Column + 1},
	}
}

// Position is a place in a file.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Range is a span in a file.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// DocumentDiagnostics is everything wrong with one file.
type DocumentDiagnostics struct {
	// Path is the file, as the caller named it.
	Path string `json:"path"`
	// Diagnostics is what is wrong with it, in the order it was found.
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Diagnose reads a file and reports what is wrong with it.
//
// It is the same reading the compiler does — the same lexer, the same parser,
// the same checker — so a language server and a build cannot disagree about
// whether a file is correct. A server that had a front end of its own would be a
// second opinion about the same file, and would go stale the first time the
// language moved.
//
// The file is read as it was given rather than as a project entry, so a person
// editing one file gets answers about that file without the project having to
// build. Imports and includes are still followed, because a mistake in an
// included file is a mistake in the file that includes it.
func Diagnose(path string) DocumentDiagnostics {
	out := DocumentDiagnostics{Path: path, Diagnostics: []Diagnostic{}}

	module, err := loadModuleGraphWithDependencies(path, nil)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, fromCompilerError(path, err)...)
		return out
	}

	tc := types.New()
	if err := tc.CheckModule(*module); err != nil {
		out.Diagnostics = append(out.Diagnostics, fromCompilerError(path, err)...)
	}
	return out
}

// fromCompilerError turns whatever the compiler reported into diagnostics.
//
// A parser reports a place inside a message rather than as an error value, so
// that one is still read out of the text — and it is the parser in this same
// repository, so the wording cannot drift away from the reader silently. Anything
// that arrives carrying a position already is used as it is.
func fromCompilerError(path string, err error) []Diagnostic {
	if parseErr := parser.AsError(err); parseErr != nil {
		return []Diagnostic{{
			Severity: SeverityError,
			Message:  parseErr.Message,
			Line:     parseErr.Line,
			Column:   parseErr.Column,
		}}
	}

	if positioned := types.AsError(err); positioned != nil {
		message := positioned.Message
		if positioned.Scope != "" && !positioned.Position.Valid() {
			message = positioned.Scope + ": " + positioned.Message
		}
		return []Diagnostic{{
			Severity: SeverityError,
			Message:  message,
			Scope:    positioned.Scope,
			Line:     positioned.Position.Line,
			Column:   positioned.Position.Column,
		}}
	}

	message := err.Error()
	// The message is prefixed with the phase and the file, which the caller
	// already knows and should not be told twice.
	if colon := strings.Index(message, ": "); colon >= 0 {
		if rest := message[colon+2:]; !strings.HasPrefix(rest, path) {
			message = rest
		}
	}
	line, column := placeOf(err, path)
	return []Diagnostic{{
		Severity: SeverityError,
		Message:  message,
		Line:     line,
		Column:   column,
	}}
}

// placeOf is the place a parser error names, when it names one.
func placeOf(err error, path string) (line, column int) {
	if parseErr := parser.AsError(err); parseErr != nil {
		if parseErr.Line > 0 && parseErr.Column > 0 {
			return parseErr.Line, parseErr.Column
		}
	}

	source, readErr := osReadFile(path)
	if readErr != nil {
		return 0, 0
	}
	offset := byteOffsetInMessage(err.Error())
	if offset < 0 || offset > len(source) {
		return 0, 0
	}
	prefix := source[:offset]
	line = 1 + strings.Count(prefix, "\n")
	column = offset + 1
	if last := strings.LastIndex(prefix, "\n"); last >= 0 {
		column = offset - last
	}
	return line, column
}

// ParseSource reads Rho source and reports what is wrong with it, without going
// near a project or an import.
//
// A caller that has the text rather than a path — an unsaved buffer, or a snippet
// someone pasted — gets the same reading the compiler would give that text, and
// the same places.
func ParseSource(path, source string) DocumentDiagnostics {
	out := DocumentDiagnostics{Path: path, Diagnostics: []Diagnostic{}}

	tokens, err := lexer.Lex(source)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, fromCompilerError(path, err)...)
		return out
	}
	module, err := parser.Parse(tokens)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, fromCompilerError(path, err)...)
		return out
	}
	module.Name = filepath.Base(path)

	tc := types.New()
	if err := tc.CheckModule(*module); err != nil {
		out.Diagnostics = append(out.Diagnostics, fromCompilerError(path, err)...)
	}
	return out
}

// byteOffsetInMessage is the place a parser error names, or -1.
//
// The parser reports a byte offset inside its message rather than as an error
// value, so this is the one place a place is read out of text. It is read from a
// message this repository writes, and a message that stops saying so yields -1
// rather than a wrong number.
func byteOffsetInMessage(message string) int {
	marker := strings.Index(message, "byte ")
	if marker < 0 {
		return -1
	}
	begin := marker + len("byte ")
	stop := begin
	for stop < len(message) && message[stop] >= '0' && message[stop] <= '9' {
		stop++
	}
	if stop == begin {
		return -1
	}
	offset, err := strconv.Atoi(message[begin:stop])
	if err != nil {
		return -1
	}
	return offset
}

// osReadFile is reading a file, which here means a diagnostic about it needs the
// text to count lines in.
func osReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	return string(data), err
}
