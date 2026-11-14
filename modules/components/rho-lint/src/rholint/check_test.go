//
// @file check_test.go
// @brief rho lint reports what the build refuses, where the build says it is.
//
// The broken programs are the compiler's own test cases (src/testdata/diagnostics
// in the checkout), so lint's findings and the build's diagnostics are compared on
// the same files.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rholint

import (
	"os"
	"path/filepath"
	"testing"

	rho "rho/src"
)

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

func onDisk(t *testing.T, dir, file string) Source {
	t.Helper()
	path := filepath.Join(dir, file)
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return Source{Path: path, Root: dir, Text: string(text), Generated: map[string]bool{}}
}

// Every error the checker finds in a file is a finding at the same line and
// column, with the same message.
func TestRhoCheckReportsWhatTheBuildRefuses(t *testing.T) {
	dir := brokenCase(t, "several")
	source := onDisk(t, dir, "src/main.rho")
	want := rho.DiagnoseInProgram(source.Path, source.Text).Diagnostics
	found := rulesRun(t, RhoRules(), RhoCheck, source)
	if len(found) != len(want) || len(want) != 4 {
		t.Fatalf("found %d findings, the compiler reports %d (want 4)", len(found), len(want))
	}
	for i := range want {
		if found[i].Line != want[i].Line || found[i].Column != want[i].Column || found[i].Message != want[i].Message {
			t.Errorf("finding %d is %d:%d %q, the compiler says %d:%d %q", i+1,
				found[i].Line, found[i].Column, found[i].Message, want[i].Line, want[i].Column, want[i].Message)
		}
		if found[i].Severity != SeverityError || found[i].File != "src/main.rho" {
			t.Errorf("finding %d is a %s in %s", i+1, found[i].Severity, found[i].File)
		}
	}
}

// A file that does not parse is reported at each place the parser stops, not
// once at the top of the file.
func TestRhoParseReportsEachBrokenDeclarationWhereItIs(t *testing.T) {
	source := aRho("/project/src/broken.rho", "module broken\n\nfn a() void {\n    let x: u32 = = 1\n}\n\nfn b( void {\n}\n")
	found := rulesRun(t, RhoRules(), RhoParse, source)
	if len(found) != 2 {
		t.Fatalf("found %d, want 2: %+v", len(found), found)
	}
	if found[0].Line != 4 || found[0].Column != 18 || found[1].Line != 7 {
		t.Errorf("reported at %d:%d and %d:%d, want 4:18 and line 7", found[0].Line, found[0].Column, found[1].Line, found[1].Column)
	}
}
