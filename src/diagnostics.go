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
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"rho/src/deps"
	"rho/src/frontend/ast"
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

// MaxRenderedErrors is how many errors a terminal is shown before the rest are
// counted: a screen of twenty mistakes is already more than one sitting fixes,
// and the twenty-first is usually the first one again.
const MaxRenderedErrors = 20

// Diagnostic is one thing wrong with a program, and where it is.
//
// It is the one shape every reader of a Rho file reports in: `rho build` renders
// it, `rho lint` writes it as SARIF, and `rho lsp` publishes it, so the editor
// underlines exactly the place the build refuses.
type Diagnostic struct {
	// File is the source the mistake is in: the included or imported file it
	// was written in, not the file that brought it into the program. Empty when
	// the mistake belongs to no file.
	File string `json:"file,omitempty"`
	// Line and Column are where it is, counting from one. They are zero when the
	// place is not known, which is a fact the caller should be able to see rather
	// than have to guess at.
	Line   int `json:"line"`
	Column int `json:"column"`
	// EndLine and EndColumn are where it ends, exclusive, when that is known.
	EndLine   int `json:"end_line,omitempty"`
	EndColumn int `json:"end_column,omitempty"`
	// Severity is how bad it is.
	Severity Severity `json:"severity"`
	// Code names the kind of mistake for a tool that filters on it: the
	// phase that found it (`lex`, `parse`, `type`, `backend`, `arena`,
	// `exports`).
	Code string `json:"code,omitempty"`
	// Message is what is wrong, and what to do about it.
	Message string `json:"message"`
	// Notes are other places worth seeing with it: where a function called
	// wrongly is declared, which file included the one the mistake is in.
	Notes []Note `json:"notes,omitempty"`
	// Scope names the declaration the problem is in, for a mistake that is about
	// the whole of one rather than about a place within it.
	Scope string `json:"scope,omitempty"`
}

// Note is a further place a diagnostic points at.
type Note struct {
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
}

// Positioned reports whether a diagnostic knows where it is.
func (d Diagnostic) Positioned() bool { return d.Line > 0 }

// Range is the diagnostic as a range, which is what a language server wants to
// underline. Without a known end it is the one character at the start.
func (d Diagnostic) Range() Range {
	end := Position{Line: d.Line, Column: d.Column + 1}
	if d.EndLine > 0 {
		end = Position{Line: d.EndLine, Column: d.EndColumn}
	}
	return Range{Start: Position{Line: d.Line, Column: d.Column}, End: end}
}

// String is the diagnostic as the CLI prints it.
func (d Diagnostic) String() string { return d.Render("") }

// Render is the diagnostic as a terminal shows it, in the form compilers share
// so an editor's problem matcher reads it:
//
//	src/helper.rho:3:14: error: cannot use a string as u32 in `let y`; ...
//	src/main.rho:2:9: note: helper.rho is included here
//
// A file under base is written relative to it.
func (d Diagnostic) Render(base string) string {
	severity := d.Severity
	if severity == "" {
		severity = SeverityError
	}
	message := d.Message
	if d.Scope != "" && !d.Positioned() {
		message = d.Scope + ": " + message
	}
	var out strings.Builder
	out.WriteString(place(d.File, d.Line, d.Column, base))
	out.WriteString(string(severity) + ": " + message)
	for _, note := range d.Notes {
		out.WriteString("\n" + place(note.File, note.Line, note.Column, base) + "note: " + note.Message)
	}
	return out.String()
}

// place is `file:line:col: `, as much of it as is known.
func place(file string, line, column int, base string) string {
	if file == "" && line == 0 {
		return ""
	}
	if base != "" && file != "" {
		if relative, err := filepath.Rel(base, file); err == nil && !strings.HasPrefix(relative, "..") {
			file = relative
		}
	}
	switch {
	case line > 0 && column > 0:
		return fmt.Sprintf("%s:%d:%d: ", file, line, column)
	case line > 0:
		return fmt.Sprintf("%s:%d: ", file, line)
	}
	return file + ": "
}

// Diagnostics is what a build reports when the program is wrong. It is an
// error, so it travels the way every failure does, and a caller that wants the
// places asks for them with DiagnosticsOf rather than reading them out of text.
type Diagnostics []Diagnostic

// Error renders every diagnostic, the first MaxRenderedErrors of them.
func (d Diagnostics) Error() string { return RenderDiagnostics(d, "") }

// RenderDiagnostics renders diagnostics for a terminal, a file under base
// written relative to it, stopping after MaxRenderedErrors errors and saying how
// many more there were.
func RenderDiagnostics(diagnostics []Diagnostic, base string) string {
	lines := []string{}
	errorsShown, errorsHidden := 0, 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity != SeverityWarning {
			if errorsShown == MaxRenderedErrors {
				errorsHidden++
				continue
			}
			errorsShown++
		}
		lines = append(lines, diagnostic.Render(base))
	}
	if errorsHidden > 0 {
		lines = append(lines, fmt.Sprintf("and %d more errors", errorsHidden))
	}
	return strings.Join(lines, "\n")
}

// DiagnosticsOf is the diagnostics an error carries, or nil when it is a
// failure with no place in a program (a missing file, a manifest that does not
// read).
func DiagnosticsOf(err error) []Diagnostic {
	var diagnostics Diagnostics
	if errors.As(err, &diagnostics) {
		return diagnostics
	}
	return nil
}

// sourceDiagnostics turns whatever a phase of the compiler returned into
// diagnostics. Each carries the file its own error names — the included or
// imported file a node came from — and path, the file being compiled, only
// when the error does not name one.
func sourceDiagnostics(path, source, code string, err error) Diagnostics {
	if existing := DiagnosticsOf(err); existing != nil {
		return existing
	}
	if lexErr := lexer.AsError(err); lexErr != nil {
		return Diagnostics{{File: path, Line: lexErr.Line, Column: lexErr.Column, Severity: SeverityError, Code: "lex", Message: lexErr.Message}}
	}
	if parseErrs := parser.Errors(err); len(parseErrs) > 0 {
		out := Diagnostics{}
		for _, parseErr := range parseErrs {
			file := parseErr.File
			if file == "" {
				file = path
			}
			line, column := parseErr.Line, parseErr.Column
			if line == 0 && parseErr.Offset > 0 {
				line, column = compilerLineColumnAt(source, parseErr.Offset)
			}
			out = append(out, Diagnostic{File: file, Line: line, Column: column, Severity: SeverityError, Code: "parse", Message: parseErr.Message})
		}
		return out
	}
	if typeErrs := types.Errors(err); len(typeErrs) > 0 {
		out := Diagnostics{}
		for _, typeErr := range typeErrs {
			out = append(out, fromTypeError(path, typeErr))
		}
		return out
	}
	diagnostic := Diagnostic{File: path, Severity: SeverityError, Code: code, Message: err.Error()}
	if byteOffset := compilerByteOffsetInMessage(err.Error()); byteOffset >= 0 && source != "" {
		diagnostic.Line, diagnostic.Column = compilerLineColumnAt(source, byteOffset)
	}
	return Diagnostics{diagnostic}
}

// fromTypeError is one type error as a diagnostic.
func fromTypeError(path string, err *types.Error) Diagnostic {
	file := err.Position.File
	if file == "" {
		file = path
	}
	diagnostic := Diagnostic{
		File:     file,
		Line:     err.Position.Line,
		Column:   err.Position.Column,
		Severity: SeverityError,
		Code:     "type",
		Message:  err.Error(),
		Scope:    err.Scope,
	}
	if err.Code != "" {
		diagnostic.Code = err.Code
	}
	if !err.Position.Valid() {
		diagnostic.Line, diagnostic.Column = 0, 0
	}
	for _, note := range err.Notes {
		noteFile := note.Position.File
		if noteFile == "" && note.Position.Valid() {
			noteFile = path
		}
		diagnostic.Notes = append(diagnostic.Notes, Note{File: noteFile, Line: note.Position.Line, Column: note.Position.Column, Message: note.Message})
	}
	return diagnostic
}

// typeWarnings are what the checker found worth saying about a program that
// does not stop it compiling — an arm of a match that never runs — as
// warnings, each in the file it is about.
func typeWarnings(path string, tc *types.TypeChecker) Diagnostics {
	out := Diagnostics{}
	for _, warning := range tc.Warnings() {
		diagnostic := fromTypeError(path, warning)
		diagnostic.Severity = SeverityWarning
		out = append(out, diagnostic)
	}
	return out
}

// withIncludeNote says which file brought a file with mistakes into the
// program, so a reader who only knows the file they built can follow the chain.
func withIncludeNote(err error, note Note) error {
	diagnostics := DiagnosticsOf(err)
	if diagnostics == nil {
		return nil
	}
	out := make(Diagnostics, len(diagnostics))
	for i, diagnostic := range diagnostics {
		diagnostic.Notes = append(append([]Note{}, diagnostic.Notes...), note)
		out[i] = diagnostic
	}
	return out
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
	// Diagnostics is what is wrong with it, in the order it was found. A
	// diagnostic's File says which file it is in: Diagnose follows includes and
	// imports, and a mistake found through them names its own file.
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Diagnose reads a file and reports what is wrong with it.
//
// It is the same reading the compiler does — the same loader, lexer, parser and
// checker — so a language server and a build cannot disagree about whether a
// file is correct. Imports and includes are followed, because a mistake in an
// included file is a mistake in the program, and each diagnostic names the file
// it is in.
func Diagnose(path string) DocumentDiagnostics {
	return DiagnoseSource(path, "", false)
}

// DiagnoseSource is Diagnose for a file whose text the caller holds: an editor
// buffer not yet saved. With overlay set, text stands in for the file on disk
// wherever the program reads it, and the rest of the program is read as the
// build reads it.
func DiagnoseSource(path, text string, overlay bool) DocumentDiagnostics {
	out := DocumentDiagnostics{Path: path, Diagnostics: []Diagnostic{}}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if overlay {
		setSourceOverlay(path, text)
		defer clearSourceOverlay(path)
	}
	module, err := loadModuleGraphWithDependencies(path, nil)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, sourceDiagnostics(path, readSource(path), "load", err)...)
		return out
	}
	tc := types.New()
	if err := tc.CheckModule(*module); err != nil {
		out.Diagnostics = append(out.Diagnostics, sourceDiagnostics(path, readSource(path), "type", err)...)
	}
	out.Diagnostics = append(out.Diagnostics, typeWarnings(path, tc)...)
	return out
}

// DiagnoseInProgram is what a build of the program a file belongs to reports
// about that file: the file is checked as part of its project — from the entry
// its module.yml names, with the project's dependencies — so a name another file
// declares is known and a mistake is reported where the build would report it.
// text stands in for the file on disk. A file the program does not load is
// diagnosed as an entry of its own.
//
// Every diagnostic comes back with its own File; a caller showing one file
// keeps the ones in it.
func DiagnoseInProgram(path, text string) DocumentDiagnostics {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	out := DocumentDiagnostics{Path: abs, Diagnostics: []Diagnostic{}}
	projectRoot := ProjectRoot(abs)
	roots := projectDependencyRoots(projectRoot)

	setSourceOverlay(abs, text)
	defer clearSourceOverlay(abs)

	entry := abs
	if manifestEntry, _, err := ResolveEntryPoint(projectRoot); err == nil && manifestEntry != "" {
		entry = manifestEntry
	}
	module, err := loadModuleGraphWithDependencies(entry, roots)
	if entry != abs && (err != nil && !mentionsFile(err, abs) || err == nil && !loads(module, abs)) {
		entry = abs
		module, err = loadModuleGraphWithDependencies(entry, roots)
	}
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, sourceDiagnostics(entry, readSource(entry), "load", err)...)
		return out
	}
	tc := types.New()
	if err := tc.CheckModule(*module); err != nil {
		out.Diagnostics = append(out.Diagnostics, sourceDiagnostics(entry, readSource(entry), "type", err)...)
	}
	out.Diagnostics = append(out.Diagnostics, typeWarnings(entry, tc)...)
	return out
}

// loads reports whether a loaded program was read from a file.
func loads(module *ast.Module, file string) bool {
	for _, loaded := range module.LoadedFiles {
		if loaded == file {
			return true
		}
	}
	return false
}

// mentionsFile reports whether a load failure is about a file.
func mentionsFile(err error, file string) bool {
	for _, diagnostic := range DiagnosticsOf(err) {
		if diagnostic.File == file {
			return true
		}
	}
	return false
}

// projectDependencyRoots are the directories a build of the project reads its
// Rho dependencies from, as `rho build` resolves them: each resolved Rho
// dependency and the generated modules. A dependency that cannot be resolved
// is left out, and a name it would declare is reported as the build reports it.
//
// A language server asks on every change, and the answer changes only with the
// manifest, so it is kept per project until the manifest is written again.
func projectDependencyRoots(projectRoot string) []string {
	key := projectRoot
	if info, err := os.Stat(deps.ManifestPath(projectRoot)); err == nil {
		key += "\x00" + info.ModTime().String()
	}
	if cached, ok := dependencyRootsCache.Load(key); ok {
		return cached.([]string)
	}
	roots := resolveDependencyRoots(projectRoot)
	dependencyRootsCache.Store(key, roots)
	return roots
}

var dependencyRootsCache sync.Map

func resolveDependencyRoots(projectRoot string) []string {
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil || manifest == nil {
		return nil
	}
	roots := []string{}
	locator := &deps.DefaultLocator{
		ProjectRoot: projectRoot,
		GlobalCache: filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod"),
		LocalCache:  filepath.Join(projectRoot, ".rho-deps"),
	}
	resolver := &deps.Resolver{Locator: locator}
	if graph, _, err := resolver.Resolve(manifest, projectRoot); err == nil && graph != nil {
		for _, node := range graph.Nodes {
			if node.Type == deps.DepTypeRho {
				roots = append(roots, node.Path)
			}
		}
	}
	generated := filepath.Join(manifest.BuildDirectory(), "generated", "modules")
	if !filepath.IsAbs(generated) {
		generated = filepath.Join(projectRoot, generated)
	}
	return append(roots, generated)
}

// ParseSource reads Rho source and reports what is wrong with it, without going
// near a project or an import.
//
// A caller that has the text rather than a path — an unsaved buffer, or a snippet
// someone pasted — gets the same reading the compiler would give that text, and
// the same places. A name another module declares cannot be known without the
// import, so a file that imports anything is reported for its syntax only.
func ParseSource(path, source string) DocumentDiagnostics {
	out := DocumentDiagnostics{Path: path, Diagnostics: []Diagnostic{}}

	tokens, err := lexer.Lex(source)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, sourceDiagnostics(path, source, "lex", err)...)
		return out
	}
	module, err := parser.ParseFile(tokens, path)
	if err != nil {
		out.Diagnostics = append(out.Diagnostics, sourceDiagnostics(path, source, "parse", err)...)
		return out
	}
	parser.AttachDocTags(module, source)
	module.Name = filepath.Base(path)
	if len(module.Imports) > 0 || len(module.Includes) > 0 {
		return out
	}

	tc := types.New()
	if err := tc.CheckModule(*module); err != nil {
		out.Diagnostics = append(out.Diagnostics, sourceDiagnostics(path, source, "type", err)...)
	}
	out.Diagnostics = append(out.Diagnostics, typeWarnings(path, tc)...)
	return out
}

// sourceOverlay is text that stands in for a file on disk while a buffer is
// diagnosed: a language server checks what the editor holds, not what was last
// saved. Diagnosing one buffer at a time is what keeps it a single entry.
var (
	sourceOverlayMu sync.Mutex
	sourceOverlay   atomic.Pointer[overlayEntry]
)

type overlayEntry struct{ path, text string }

func setSourceOverlay(path, text string) {
	sourceOverlayMu.Lock()
	sourceOverlay.Store(&overlayEntry{path: path, text: text})
}

func clearSourceOverlay(string) {
	sourceOverlay.Store(nil)
	sourceOverlayMu.Unlock()
}

// readSourceFile reads a source the way the loader does: the overlay when the
// file is the buffer being diagnosed, the disk otherwise.
func readSourceFile(abs string) ([]byte, error) {
	if entry := sourceOverlay.Load(); entry != nil && entry.path == abs {
		return []byte(entry.text), nil
	}
	return os.ReadFile(abs)
}
