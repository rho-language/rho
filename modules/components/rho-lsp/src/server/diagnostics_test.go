//
// @file diagnostics_test.go
// @brief The editor underlines what the build refuses, at the same places.
//
// The broken programs are the compiler's own test cases (src/testdata/diagnostics
// in the checkout), so the build's expectations and the editor's are about the
// same files. Each case is opened over the protocol, and what the server
// publishes is compared with what the compiler reports: the same messages, the
// same places, counted from zero as the protocol counts.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rho "rho/src"
)

// brokenCase is a case directory of the compiler's diagnostics tests.
func brokenCase(t *testing.T, name string) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "src", "testdata", "diagnostics", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("the compiler's test cases are not in this checkout: %v", err)
	}
	return dir
}

type published struct {
	URI         string `json:"uri"`
	Diagnostics []struct {
		Range struct {
			Start struct{ Line, Character int } `json:"start"`
			End   struct{ Line, Character int } `json:"end"`
		} `json:"range"`
		Severity           int    `json:"severity"`
		Message            string `json:"message"`
		Source             string `json:"source"`
		RelatedInformation []struct {
			Message string `json:"message"`
		} `json:"relatedInformation"`
	} `json:"diagnostics"`
}

// openOverProtocol sends a didOpen for a file and returns what the server
// published for it.
func openOverProtocol(t *testing.T, path string) published {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(string(text))
	payload := `{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file://` + path + `","languageId":"rho","version":1,"text":` + string(quoted) + `}}}`
	out, err := send(frame(payload))
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range strings.Split(out, "Content-Length: ") {
		body := message
		if at := strings.Index(body, "\r\n\r\n"); at >= 0 {
			body = body[at+4:]
		}
		var notification struct {
			Method string    `json:"method"`
			Params published `json:"params"`
		}
		if json.Unmarshal([]byte(body), &notification) == nil && notification.Method == "textDocument/publishDiagnostics" {
			return notification.Params
		}
	}
	t.Fatalf("nothing was published for %s:\n%s", path, out)
	return published{}
}

// The editor publishes, for each file, the compiler's diagnostics about that
// file: same message, same line and column (from zero), and an underline that
// starts there.
func TestTheEditorPublishesWhatTheBuildReports(t *testing.T) {
	for _, name := range []string{"several", "cascade", "parse", "generic", "stdmisuse", "lex"} {
		t.Run(name, func(t *testing.T) {
			entry := filepath.Join(brokenCase(t, name), "src", "main.rho")
			text, _ := os.ReadFile(entry)
			want := rho.DiagnoseInProgram(entry, string(text)).Diagnostics
			got := openOverProtocol(t, entry).Diagnostics
			if len(got) != len(want) || len(want) == 0 {
				t.Fatalf("published %d diagnostics, the compiler reports %d:\n%s", len(got), len(want), rho.RenderDiagnostics(want, ""))
			}
			for i := range want {
				g, w := got[i], want[i]
				if g.Message != w.Message || g.Range.Start.Line != w.Line-1 || g.Range.Start.Character != w.Column-1 {
					t.Errorf("published %d:%d %q, the compiler says %d:%d %q",
						g.Range.Start.Line+1, g.Range.Start.Character+1, g.Message, w.Line, w.Column, w.Message)
				}
				if g.Severity != 1 || g.Source != "rho" {
					t.Errorf("diagnostic %d is severity %d from %q, want an error from rho", i+1, g.Severity, g.Source)
				}
				if len(g.RelatedInformation) != len(w.Notes) {
					t.Errorf("diagnostic %d carries %d notes, the compiler has %d", i+1, len(g.RelatedInformation), len(w.Notes))
				}
			}
		})
	}
}

// A type error in an included file is published for that file, under the
// expression that is wrong — the whole of it — and not for the file including it.
func TestAnIncludedFilesErrorIsPublishedForThatFile(t *testing.T) {
	dir := brokenCase(t, "included")
	if got := openOverProtocol(t, filepath.Join(dir, "src", "main.rho")).Diagnostics; len(got) != 0 {
		t.Errorf("the including file was given its include's error: %+v", got)
	}
	got := openOverProtocol(t, filepath.Join(dir, "src", "helper.rho")).Diagnostics
	if len(got) != 1 {
		t.Fatalf("published %d diagnostics for helper.rho, want 1: %+v", len(got), got)
	}
	r := got[0].Range
	// `let y: u32 = "no"` on the fourth line: the string literal, columns 17 to 21.
	if r.Start.Line != 3 || r.Start.Character != 17 || r.End.Line != 3 || r.End.Character != 21 {
		t.Errorf("underlined %d:%d-%d:%d, want 3:17-3:21 (the string literal)", r.Start.Line, r.Start.Character, r.End.Line, r.End.Character)
	}
	if !strings.Contains(got[0].Message, "cannot use a string as u32 in `let y`") {
		t.Errorf("published %q", got[0].Message)
	}
}

// A file that does not parse stops the program loading, and the file including
// it is told at its include line, naming where the mistake is.
func TestABrokenIncludeIsShownAtTheIncludeLine(t *testing.T) {
	dir := brokenCase(t, "included_parse")
	got := openOverProtocol(t, filepath.Join(dir, "src", "main.rho")).Diagnostics
	if len(got) != 1 {
		t.Fatalf("published %d diagnostics, want 1: %+v", len(got), got)
	}
	if got[0].Range.Start.Line != 1 || got[0].Range.Start.Character != 0 {
		t.Errorf("shown at %d:%d, want the include on line 2", got[0].Range.Start.Line+1, got[0].Range.Start.Character+1)
	}
	if !strings.HasPrefix(got[0].Message, "helper.rho:5:1: ") {
		t.Errorf("the message does not say where the mistake is: %q", got[0].Message)
	}
	helper := openOverProtocol(t, filepath.Join(dir, "src", "helper.rho")).Diagnostics
	if len(helper) != 1 || helper[0].Range.Start.Line != 4 {
		t.Errorf("helper.rho was not told at line 5: %+v", helper)
	}
}
