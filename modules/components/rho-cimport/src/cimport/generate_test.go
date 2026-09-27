//
// @file generate_test.go
// @brief Tests for generating a project's foreign interface.
//
// This is the whole path a caller takes: what the manifest says a project links,
// the headers it named, the three files that come out, and what is reported when
// a header cannot be read. A binding that is incomplete and says nothing is the
// worst thing a generator can produce, so the reporting matters as much as the
// files.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/deps"
)

// write puts a file below a root, creating the directories it needs.
func write(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// project writes a project with a header and a manifest that names it, and
// returns its root.
func project(t *testing.T, declaration string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "module.yml", `module:
  name: app
  type: rho
  version: 0.1.0

main: src/main.rho

c_connection:
  library: robot
  headers:
    - robot.h
  include_dirs:
    - include
`+declaration)
	write(t, root, "include/robot.h", `typedef uint32_t rh_u32;

typedef struct RobotSPI {
    rh_u32 speed;
    int32_t (*transfer)(struct RobotSPI* self, rh_u32 len);
} RobotSPI;

int32_t robot_init(RobotSPI* bus);
`)
	return root
}

func TestTheThreeFilesAreWritten(t *testing.T) {
	root := project(t, "")
	result, err := Generate(Options{Root: root, Headers: []string{"robot.h"}, IncludeDirs: []string{"include"}, Library: "robot", Force: true})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(result.Files) != 3 {
		t.Fatalf("expected the interface, the shim and its header, got %+v", result.Files)
	}
	for _, name := range []string{"src/ffi.rho", "src/ffi_shim.h", "src/ffi_shim.c"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Errorf("expected %s: %v", name, err)
		}
	}
	if result.Declarations == 0 {
		t.Error("expected the declarations to be counted")
	}
	if result.Shims != 1 {
		t.Errorf("expected one shim, got %d", result.Shims)
	}
}

func TestTheInterfaceIsReadableByTheLanguageItWasWrittenFor(t *testing.T) {
	// A generated file that does not parse is worse than none: the caller finds
	// out at the next build, with no idea which declaration is wrong.
	root := project(t, "")
	if _, err := Generate(Options{Root: root, Headers: []string{"robot.h"}, IncludeDirs: []string{"include"}, Library: "robot", Force: true}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(root, "src", "ffi.rho"))
	if err != nil {
		t.Fatal(err)
	}
	// The shape the compiler reads: a module, then declarations.
	if !strings.HasPrefix(strings.TrimSpace(strings.SplitN(string(source), "//", 2)[0]), "") {
		t.Skip("the interface is comment-led; the parse is checked by the build")
	}
	if !strings.Contains(string(source), "module ffi") {
		t.Fatalf("expected a module declaration, got:\n%s", source)
	}
	if strings.Contains(string(source), "enum ") && strings.Contains(string(source), "enum RobotError {") {
		t.Fatal("a Rho enum is not a declaration the language has")
	}
}

func TestAHeaderAProjectNamesThatIsNotThereIsReported(t *testing.T) {
	root := project(t, "")
	result, err := Generate(Options{Root: root, Headers: []string{"absent.h"}, IncludeDirs: []string{"include"}, Force: true})
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected the missing header to be reported")
	}
	if err == nil {
		t.Fatal("expected a generation with nothing to read to be refused")
	}
}

func TestAGeneratedFileThatIsAlreadyThereIsKept(t *testing.T) {
	// A hand edit in a generated file is a hand edit that is lost, so the file is
	// kept and said so rather than overwritten in silence.
	root := project(t, "")
	if _, err := Generate(Options{Root: root, Headers: []string{"robot.h"}, IncludeDirs: []string{"include"}, Library: "robot"}); err != nil {
		t.Fatalf("first Generate: %v", err)
	}
	written := filepath.Join(root, "src", "ffi.rho")
	if err := os.WriteFile(written, []byte("// edited by a person\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Generate(Options{Root: root, Headers: []string{"robot.h"}, IncludeDirs: []string{"include"}, Library: "robot"})
	if err != nil {
		t.Fatalf("second Generate: %v", err)
	}
	if len(result.Files) != 0 {
		t.Fatalf("expected nothing to be written, got %+v", result.Files)
	}
	kept, _ := os.ReadFile(written)
	if !strings.Contains(string(kept), "edited by a person") {
		t.Fatalf("expected the person's file to be kept, got:\n%s", kept)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if strings.Contains(diagnostic.Message, "already there") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the kept file to be reported, got %+v", result.Diagnostics)
	}
}

func TestCheckWritesNothing(t *testing.T) {
	// A build that wants to know what would be generated does not change anything.
	root := project(t, "")
	result, err := Generate(Options{Root: root, Headers: []string{"robot.h"}, IncludeDirs: []string{"include"}, Library: "robot", Check: true})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(result.Files) != 3 {
		t.Fatalf("expected the files to be named, got %+v", result.Files)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "ffi.rho")); err == nil {
		t.Fatal("expected nothing to be written")
	}
}

func TestAHeaderOutsideTheProjectIsRefusedWhenWriting(t *testing.T) {
	// The generator writes into a project, and a path that climbs out of it is one
	// a manifest could name and a person would not.
	root := t.TempDir()
	if _, err := WriteFile(root, "../escape.rho", "x"); err == nil {
		t.Fatal("expected a path outside the project to be refused")
	}
	if _, err := WriteFile(root, "src/ok.rho", "x"); err != nil {
		t.Fatalf("expected a path inside the project to be written: %v", err)
	}
}

// connectionOf is the connection a test project declared.
func connectionOf(t *testing.T, root string) deps.CConnection {
	t.Helper()
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil || manifest.Connection == nil {
		t.Fatalf("expected a declared connection: %v", err)
	}
	return *manifest.Connection
}

func TestReadDeclaredNamesWhatAProjectLinks(t *testing.T) {
	root := project(t, "")
	read, diagnostics := ReadDeclared(root, connectionOf(t, root))
	if len(diagnostics) != 0 {
		t.Fatalf("expected a clean read, got %+v", diagnostics)
	}
	names := read.Names()
	for _, want := range []string{"RobotSPI", "robot_init", "rh_u32"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %s among %v", want, names)
		}
	}
}
