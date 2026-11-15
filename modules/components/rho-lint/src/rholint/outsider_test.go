package rholint

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/deps"
)

// anApplication is what `rho init` writes, with the files a test adds to it.
func anApplication(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "module.yml"), `module:
  name: csv_report
  type: rho
  version: 0.1.0
  description: x
main: src/main.rho
build:
  language: c
  targets: [c]
`)
	for path, text := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(path)), text)
	}
	return root
}

func read(t *testing.T, root, path string) Source {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	text, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	return Source{Path: full, Root: root, Text: string(text), Generated: map[string]bool{}}
}

// A test file is read as the compiler is given it, with the runner's `test`
// marker taken off, so it parses — and its positions stay where they are.
func TestATestFileParses(t *testing.T) {
	root := anApplication(t, map[string]string{
		"src/main.rho":       "module csv_report\n\nfn main() void {\n}\n",
		"tests/csv_test.rho": "module csv_test\n\ntest fn plain() void {\n    var x: u32 = 1\n    while true {\n        x = x + 1\n    }\n    print(int_to_str(x))\n}\n\nfn main() void {\n    plain()\n}\n",
	})
	source := read(t, root, "tests/csv_test.rho")
	assertIDs(t, rulesRun(t, RhoRules(), RhoParse, source))
	if found := rulesRun(t, RhoRules(), RhoCheck, source); len(found) != 0 {
		t.Fatalf("%+v", found)
	}
	// A rule over the tree still runs, at the line the file has.
	found := rulesRun(t, RhoRules(), RhoUnreachable, source)
	if len(found) != 1 || found[0].Line != 8 || found[0].Column != 5 {
		t.Fatalf("found %+v, want the unreachable print at 8:5", found)
	}
}

// main.rho declaring the module's name is what `rho init` writes and what the
// documentation says, and it is not reported.
func TestMainDeclaringTheModuleIsNotReported(t *testing.T) {
	root := anApplication(t, map[string]string{
		"src/main.rho":  "module csv_report\n\nfn main() void {\n}\n",
		"src/csv.rho":   "module csv_report.csv\n",
		"src/other.rho": "module somebody_else\n",
	})
	assertIDs(t, rulesRun(t, RhoRules(), RhoModuleName, read(t, root, "src/main.rho")))
	assertIDs(t, rulesRun(t, RhoRules(), RhoModuleName, read(t, root, "src/csv.rho")))
	assertIDs(t, rulesRun(t, RhoRules(), RhoModuleName, read(t, root, "src/other.rho")), RhoModuleName)
}

// A bare call of a function an import declares is a use of the import, and so
// is a generic call and one inside a `for`.
func TestABareCallIsAUseOfTheImport(t *testing.T) {
	root := anApplication(t, map[string]string{
		"src/main.rho": `module csv_report

import std.assert.assert
import std.collections.slices
import std.core.cmp

fn main() void {
    var xs: i32[3] = [3, 1, 2]
    let s: Slice[i32] = xs[..]
    for x in s {
        assert_true(x > 0)
    }
    sort_by[i32](s, fn(a: i32, b: i32) -> bool { return a < b })
}
`,
	})
	found := rulesRun(t, RhoRules(), RhoUnusedImport, read(t, root, "src/main.rho"))
	if len(found) != 1 || found[0].Message != "std.core.cmp is imported and nothing in this file names it" {
		t.Fatalf("found %+v, want only std.core.cmp reported", found)
	}
}

// A binding declared after a loop shadows nothing the loop bound.
func TestABindingAfterTheLoopIsNotAShadow(t *testing.T) {
	source := aRho("/project/src/main.rho", `module main

fn main() void {
    var i: u32 = 0
    while i < 3 {
        let line: u32 = i
        i = i + line
    }
    let line: u32 = 7
    if line > 0 {
        let line: u32 = 8
    }
}
`)
	found := rulesRun(t, RhoRules(), RhoShadowedBinding, source)
	if len(found) != 1 || found[0].Line != 11 {
		t.Fatalf("found %+v, want only the let inside the if, at line 11", found)
	}
}

// What a build wrote under the build directory is never read.
func TestGeneratedFilesAreLeftOut(t *testing.T) {
	root := anApplication(t, map[string]string{
		"src/main.rho":               "module csv_report\n\nfn main() void {\n}\n",
		"build/generated/c/app.c":    "void app_init(void) {}\n",
		"build/generated/x/copy.rho": "module copy\n",
		"native/hal.c":               "int hal(void) { return 0; }\n",
	})
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil {
		t.Fatal(err)
	}
	exclusions := Exclusions{Root: root, Generated: GeneratedDirectories(root, manifest)}
	if exclusions.Keeps(Unit{File: filepath.Join(root, "build", "generated", "c", "app.c")}) {
		t.Error("a generated C file is read")
	}
	if !exclusions.Keeps(Unit{File: filepath.Join(root, "native", "hal.c")}) {
		t.Error("the project's own C is left out")
	}
	result, err := Analyse(Options{Root: root, Manifest: manifest, Engines: []string{EngineRho, EngineC}})
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range result.Units {
		if filepath.Dir(unit.File) != root && filepath.HasPrefix(unit.File, filepath.Join(root, "build")) {
			t.Errorf("a run read %s", unit.File)
		}
	}
}
