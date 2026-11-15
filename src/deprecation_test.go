//
// @file deprecation_test.go
// @brief A use of something marked `@deprecated` is a warning, and only that.
//
// testdata/deprecation is a program whose module `lib` keeps a deprecated
// function, struct, enum variant and constant working, and whose module
// `legacy` is deprecated as a whole. main.rho uses each once (OldSize twice: as
// a type and as a literal); lib.rho uses them too, and is not told, because a
// module keeping an old name working is not a module that has to move off it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/types"
)

// deprecationWarnings are the warnings main.rho gets, in the order they are
// found, each with the declaration it notes.
var deprecationWarnings = []struct {
	line, column int
	message      string
	noteFile     string
	noteLine     int
}{
	{3, 1, "module legacy is deprecated: use lib instead", "legacy.rho", 7},
	{6, 18, "old_area is deprecated: use area instead", "lib.rho", 15},
	{7, 5, "OldSize is deprecated: use Size instead", "lib.rho", 24},
	{7, 22, "OldSize is deprecated: use Size instead", "lib.rho", 24},
	{8, 19, "Mode.Lit is deprecated: use Mode.On instead", "lib.rho", 37},
	{9, 18, "OLD_SCALE is deprecated: use SCALE instead", "lib.rho", 9},
}

// copyDeprecationCase copies the case into a directory of its own, so a build
// writes nothing into the tree.
func copyDeprecationCase(t *testing.T) string {
	t.Helper()
	from, _ := filepath.Abs(filepath.Join("testdata", "deprecation"))
	root := t.TempDir()
	err := filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(from, path)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(root, relative), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, relative), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func checkDeprecationWarnings(t *testing.T, root string, got []Diagnostic) {
	t.Helper()
	main := filepath.Join(root, "src", "main.rho")
	if len(got) != len(deprecationWarnings) {
		t.Fatalf("got %d diagnostics, want %d:\n%s", len(got), len(deprecationWarnings), RenderDiagnostics(got, root))
	}
	for i, want := range deprecationWarnings {
		g := got[i]
		if g.File != main || g.Line != want.line || g.Column != want.column || g.Severity != SeverityWarning || g.Message != want.message || g.Code != types.DeprecatedCode {
			t.Errorf("warning %d is %s [%s], want src/main.rho:%d:%d: warning: %s", i+1, g.Render(root), g.Code, want.line, want.column, want.message)
			continue
		}
		if len(g.Notes) != 1 || filepath.Base(g.Notes[0].File) != want.noteFile || g.Notes[0].Line != want.noteLine || !strings.Contains(g.Notes[0].Message, "is declared here") {
			t.Errorf("warning %d does not note its declaration at %s:%d: %s", i+1, want.noteFile, want.noteLine, g.Render(root))
		}
	}
}

// The editor's reading — DiagnoseInProgram, which `rho lsp` and `rho lint`
// publish — says each use once, at the use, and nothing about lib.rho's own.
func TestADeprecatedNameIsAWarningAtEveryUseOutsideItsModule(t *testing.T) {
	root := copyDeprecationCase(t)
	for _, file := range []string{"main.rho", "lib.rho"} {
		path := filepath.Join(root, "src", file)
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got := DiagnoseInProgram(path, string(text)).Diagnostics
		checkDeprecationWarnings(t, root, got)
	}
}

// The build reports the same warnings and still builds, and the program runs
// the same on C and on Go: deprecated is a word about the future, not a change
// to what the code does today.
func TestADeprecatedNameStillBuildsOnBothBackends(t *testing.T) {
	root := copyDeprecationCase(t)
	entry := filepath.Join(root, "src", "main.rho")
	module, err := loadModuleGraphWithDependencies(entry, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc := types.New()
	if err := tc.CheckModule(*module); err != nil {
		t.Fatalf("a deprecation stopped the check: %v", err)
	}
	checkDeprecationWarnings(t, root, typeWarnings(entry, tc))

	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("a deprecation stopped the build: %v", err)
	}
	cBinary := filepath.Join(t.TempDir(), "deprecation-c")
	build := exec.Command("gcc", "-std=c11", "-o", cBinary,
		filepath.Join(dirs[0], "main.c"), filepath.Join(dirs[0], "app.c"), filepath.Join(dirs[0], "runtime.c"), "-I", dirs[0], "-lm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("C build: %v\n%s", err, out)
	}
	goBinary := filepath.Join(t.TempDir(), "deprecation-go")
	goBuild := exec.Command("go", "build", "-o", goBinary, ".")
	goBuild.Dir = dirs[1]
	if out, err := goBuild.CombinedOutput(); err != nil {
		t.Fatalf("Go build: %v\n%s", err, out)
	}
	for _, binary := range []string{cBinary, goBinary} {
		out, err := exec.Command(binary).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "32" {
			t.Errorf("%s printed %q (%v), want 32", filepath.Base(binary), out, err)
		}
	}
}

// A local of a deprecated name is the local, and a file with no imports has no
// other module to be told about.
func TestALocalHidesADeprecatedNameAndAModuleIsNotToldAboutItself(t *testing.T) {
	source := `module app

// @deprecated use fresh instead
fn stale() u32 {
    return 1
}

fn fresh() u32 {
    return stale()
}

fn main() void {
    let stale: u32 = 2
    print(int_to_str(stale + fresh()))
}
`
	got := ParseSource("app.rho", source).Diagnostics
	if len(got) != 0 {
		t.Fatalf("a use inside the declaring module was reported:\n%s", RenderDiagnostics(got, ""))
	}
}
