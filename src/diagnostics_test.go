//
// @file diagnostics_test.go
// @brief Deliberately broken programs, and exactly what the build says about each.
//
// Every directory under testdata/diagnostics is a project with a mistake in it —
// in an included file, an imported module, a standard library call, a generic,
// several at once — and a `want` file saying what the build reports: the file,
// the line, the column, the severity and a piece of the message, and the notes
// beside it. A report that names the wrong file, or the right file at the
// statement rather than the expression, fails here.
//
// The `want` format is one diagnostic per line, a note on the line after it:
//
//	src/helper.rho:4:18: error: cannot use a string as u32
//	note: src/main.rho:2:1: helper.rho is included here
//
// A path is relative to the case, or to the repository for a `std/` file. A
// case with a `board` file is built for that board, which is what makes a loop
// that keeps its strings an error rather than a warning.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// wantedDiagnostic is one line of a case's `want` file.
type wantedDiagnostic struct {
	file     string
	line     int
	column   int
	severity string
	message  string
	notes    []wantedDiagnostic
}

// parseWanted reads `path:line:col: severity: message`, or a note's
// `note: path:line:col: message`.
func parseWanted(t *testing.T, text string, note bool) wantedDiagnostic {
	t.Helper()
	if note {
		text = strings.TrimPrefix(text, "note: ")
	}
	parts := strings.SplitN(text, ":", 4)
	if len(parts) < 4 {
		t.Fatalf("a want line is not path:line:col: ...: %q", text)
	}
	line, err1 := strconv.Atoi(parts[1])
	column, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil {
		t.Fatalf("a want line has no line and column: %q", text)
	}
	rest := strings.TrimSpace(parts[3])
	wanted := wantedDiagnostic{file: parts[0], line: line, column: column, severity: "note", message: rest}
	if !note {
		severity, message, ok := strings.Cut(rest, ": ")
		if !ok {
			t.Fatalf("a want line has no severity: %q", text)
		}
		wanted.severity, wanted.message = severity, message
	}
	return wanted
}

func readWanted(t *testing.T, path string) []wantedDiagnostic {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := []wantedDiagnostic{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "note: ") {
			if len(out) == 0 {
				t.Fatalf("%s: a note before any diagnostic", path)
			}
			out[len(out)-1].notes = append(out[len(out)-1].notes, parseWanted(t, line, true))
			continue
		}
		out = append(out, parseWanted(t, line, false))
	}
	return out
}

// relativeTo writes a reported file the way a `want` file does.
func relativeTo(caseDir, repo, file string) string {
	for _, base := range []string{caseDir, repo} {
		if relative, err := filepath.Rel(base, file); err == nil && !strings.HasPrefix(relative, "..") {
			return filepath.ToSlash(relative)
		}
	}
	return file
}

// buildDiagnostics is what `rho build` reports for a case: the load, the check,
// and the refusals a C build and a board build make after lowering.
func buildDiagnostics(t *testing.T, caseDir string) []Diagnostic {
	t.Helper()
	entry := filepath.Join(caseDir, "src", "main.rho")
	module, escapes, err := compileToIR(nil, entry, nil)
	if err == nil {
		err = checkCRefusals(module)
	}
	if err == nil {
		board := ""
		if data, readErr := os.ReadFile(filepath.Join(caseDir, "board")); readErr == nil {
			board = strings.TrimSpace(string(data))
		}
		if board != "" {
			err = checkArenaEscapes(escapes, board)
		}
	}
	if err == nil {
		t.Fatal("the program built, and every case here is broken")
	}
	diagnostics := DiagnosticsOf(err)
	if diagnostics == nil {
		t.Fatalf("the failure carries no diagnostics: %v", err)
	}
	return diagnostics
}

func matchDiagnostics(t *testing.T, caseDir, repo string, got []Diagnostic, want []wantedDiagnostic) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d diagnostics, want %d:\n%s", len(got), len(want), RenderDiagnostics(got, caseDir))
	}
	for i, w := range want {
		g := got[i]
		file := relativeTo(caseDir, repo, g.File)
		if file != w.file || g.Line != w.line || g.Column != w.column || string(g.Severity) != w.severity || !strings.Contains(g.Message, w.message) {
			t.Errorf("diagnostic %d is\n  %s\nwant\n  %s:%d:%d: %s: ...%s...", i+1, g.Render(caseDir), w.file, w.line, w.column, w.severity, w.message)
		}
		if len(g.Notes) != len(w.notes) {
			t.Errorf("diagnostic %d has %d notes, want %d: %s", i+1, len(g.Notes), len(w.notes), g.Render(caseDir))
			continue
		}
		for j, wn := range w.notes {
			gn := g.Notes[j]
			if relativeTo(caseDir, repo, gn.File) != wn.file || gn.Line != wn.line || gn.Column != wn.column || !strings.Contains(gn.Message, wn.message) {
				t.Errorf("note %d of diagnostic %d is %s:%d:%d: %s, want %s:%d:%d: ...%s...",
					j+1, i+1, relativeTo(caseDir, repo, gn.File), gn.Line, gn.Column, gn.Message, wn.file, wn.line, wn.column, wn.message)
			}
		}
	}
}

func TestBrokenProgramsAreReportedWhereTheMistakeIs(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("testdata", "diagnostics"))
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := filepath.Abs("..")
	cases, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range cases {
		if !entry.IsDir() {
			continue
		}
		caseDir := filepath.Join(root, entry.Name())
		t.Run(entry.Name(), func(t *testing.T) {
			want := readWanted(t, filepath.Join(caseDir, "want"))
			matchDiagnostics(t, caseDir, repo, buildDiagnostics(t, caseDir), want)
		})
	}
}

// The editor's reading is the build's: DiagnoseInProgram, which `rho lsp` and
// `rho lint` call, reports the same check errors at the same places for every
// case the checker or loader refuses. (The refusals made after lowering — a C
// build, a board — belong to a build for a target and are not an editor's.)
func TestTheEditorSeesWhatTheBuildRefuses(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("testdata", "diagnostics"))
	repo, _ := filepath.Abs("..")
	for _, name := range []string{"included", "nested", "imported", "stdmisuse", "generic", "several", "cascade", "parse", "lex", "included_parse", "enums"} {
		caseDir := filepath.Join(root, name)
		t.Run(name, func(t *testing.T) {
			entry := filepath.Join(caseDir, "src", "main.rho")
			text, err := os.ReadFile(entry)
			if err != nil {
				t.Fatal(err)
			}
			got := DiagnoseInProgram(entry, string(text)).Diagnostics
			matchDiagnostics(t, caseDir, repo, got, readWanted(t, filepath.Join(caseDir, "want")))
		})
	}
}

// An unsaved buffer is what is checked, not the file on disk: the editor
// underlines what the person is looking at.
func TestDiagnoseInProgramReadsTheBufferNotTheDisk(t *testing.T) {
	caseDir, _ := filepath.Abs(filepath.Join("testdata", "diagnostics", "included"))
	helper := filepath.Join(caseDir, "src", "helper.rho")
	fixed := "module helper\n\nfn helper() void {\n    let y: u32 = 7\n}\n"
	if got := DiagnoseInProgram(helper, fixed).Diagnostics; len(got) != 0 {
		t.Fatalf("the fixed buffer was reported:\n%s", RenderDiagnostics(got, caseDir))
	}
	// And the included file, opened on its own, is checked as part of the
	// program that includes it, so a name main.rho declares is known.
	broken, _ := os.ReadFile(helper)
	got := DiagnoseInProgram(helper, string(broken)).Diagnostics
	if len(got) != 1 || got[0].File != helper || got[0].Line != 4 || got[0].Column != 18 {
		t.Fatalf("the included file's error is not where it is:\n%s", RenderDiagnostics(got, caseDir))
	}
}

// More errors than a screen holds are counted, not listed.
func TestRenderingStopsAtTheCapAndCountsTheRest(t *testing.T) {
	many := Diagnostics{}
	for i := 1; i <= MaxRenderedErrors+5; i++ {
		many = append(many, Diagnostic{File: "/p/main.rho", Line: i, Column: 1, Severity: SeverityError, Message: "wrong"})
	}
	many = append(many, Diagnostic{File: "/p/main.rho", Line: 99, Column: 2, Severity: SeverityWarning, Message: "careful"})
	rendered := RenderDiagnostics(many, "/p")
	if strings.Count(rendered, ": error: ") != MaxRenderedErrors {
		t.Errorf("rendered %d errors, want %d", strings.Count(rendered, ": error: "), MaxRenderedErrors)
	}
	if !strings.Contains(rendered, "and 5 more errors") || !strings.Contains(rendered, "main.rho:99:2: warning: careful") {
		t.Errorf("the rest are not counted, or the warning was dropped:\n%s", rendered)
	}
}

// A diagnostic is printed the way compilers print one, so an editor's problem
// matcher reads it.
func TestADiagnosticRendersAsFileLineColumnSeverityMessage(t *testing.T) {
	d := Diagnostic{File: "/p/src/helper.rho", Line: 4, Column: 18, Severity: SeverityError, Message: "wrong",
		Notes: []Note{{File: "/p/src/main.rho", Line: 2, Column: 1, Message: "helper.rho is included here"}}}
	want := "src/helper.rho:4:18: error: wrong\nsrc/main.rho:2:1: note: helper.rho is included here"
	if got := d.Render("/p"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
