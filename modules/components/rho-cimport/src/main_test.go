//
// @file main_test.go
// @brief Tests for the rho cimport command.
//
// The command is what a person and a build both run, so what is tested here is
// what it reads and what it refuses: a project that names no headers, a run that
// cannot find one, and the fact that a run which reports a problem is a run that
// failed.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// project writes a project that links a native library, and returns its root.
func project(t *testing.T, headers string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"module.yml": `module:
  name: app
  type: rho
  version: 0.1.0

main: src/main.rho

c_connection:
  library: robot
` + headers,
		"include/robot.h": "typedef uint32_t rh_u32;\n\nint32_t robot_init(rh_u32 speed);\n",
		"src/main.rho":    "module main\n\nfn main() void {\n    print(\"x\");\n}\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const declaredHeaders = `  headers:
    - robot.h
  include_dirs:
    - include
`

func TestNoArgumentsIsAUsage(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Fatalf("expected a usage answer, got %d", code)
	}
}

func TestAnUnknownCommandIsRefused(t *testing.T) {
	if code := run([]string{"transpile"}); code != 2 {
		t.Fatalf("expected an unknown command to be refused, got %d", code)
	}
}

func TestHeadersNamesWhatAProjectLinks(t *testing.T) {
	// A person asking what an interface would be made of gets the declarations,
	// without anything being written.
	root := project(t, declaredHeaders)
	if code := run([]string{"headers", root}); code != 0 {
		t.Fatalf("expected headers to answer, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "build", "generated", "modules", "ffi.rho")); err == nil {
		t.Fatal("expected headers to write nothing")
	}
}

func TestGenerateWritesTheInterfaceAndTheShim(t *testing.T) {
	root := project(t, declaredHeaders)
	if code := run([]string{"generate", root, "--force"}); code != 0 {
		t.Fatalf("expected the generation to answer, got %d", code)
	}
	// The Rho interface and the C shim are written to different places, because
	// they are different things: the interface is Rho the compiler reads, and the
	// shim is C the generated project compiles. Neither belongs in `src/`.
	for _, name := range []string{"ffi_shim.h", "ffi_shim.c"} {
		if _, err := os.Stat(filepath.Join(root, "build", "generated", "c", name)); err != nil {
			t.Errorf("expected the shim %s in the generated C project: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "build", "generated", "modules", "ffi.rho")); err != nil {
		t.Errorf("expected the interface under the build directory: %v", err)
	}
	for _, name := range []string{"ffi.rho", "ffi_shim.h", "ffi_shim.c"} {
		if _, err := os.Stat(filepath.Join(root, "src", name)); err == nil {
			t.Errorf("generated file %s was written into src/", name)
		}
	}
	// Flags come after the path, which is what a person types first.
	source, err := os.ReadFile(filepath.Join(root, "build", "generated", "modules", "ffi.rho"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "extern fn robot_init(speed: u32) i32") {
		t.Fatalf("expected the declaration, got:\n%s", source)
	}
}

func TestCheckWritesNothingAndSaysWhatItWouldWrite(t *testing.T) {
	root := project(t, declaredHeaders)
	if code := run([]string{"check", root}); code != 0 {
		t.Fatalf("expected check to answer, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "build", "generated", "modules", "ffi.rho")); err == nil {
		t.Fatal("expected check to write nothing")
	}
}

func TestAProjectThatNamesNoHeadersIsToldSo(t *testing.T) {
	// A project with a connection and no headers has nothing to generate from, and
	// saying so is better than a run that succeeds and writes nothing.
	root := project(t, "")
	if code := run([]string{"generate", root, "--force"}); code == 0 {
		t.Fatal("expected a project with no headers to fail the run")
	}
}

func TestAHeaderThatCannotBeReadFailsTheRun(t *testing.T) {
	// A binding with a gap in it is worse than one that was not generated, so a run
	// that could not read a header is a run that failed.
	root := project(t, `  headers:
    - absent.h
  include_dirs:
    - include
`)
	if code := run([]string{"generate", root, "--force"}); code == 0 {
		t.Fatal("expected a missing header to fail the run")
	}
}

func TestAProjectWithNoConnectionIsToldSo(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"),
		[]byte("module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"generate", root, "--force"}); code == 0 {
		t.Fatal("expected a project with no connection to fail the run")
	}
}
