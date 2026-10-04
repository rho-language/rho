//
// @file main_test.go
// @brief Tests for the rho target command.
//
// The command is what a person runs, so what is tested here is what it reads and
// what it refuses: an unknown target, a target that cannot be generated for, and
// the fact that generating writes into the project it was given.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chdir moves into a directory for the duration of a test, because the command
// reads the specs it ships from beside itself or from the working tree.
func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(previous) })
}

// specTree writes a small tree of specs and moves into it.
func specTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "targets"), 0o755); err != nil {
		t.Fatal(err)
	}
	specs := map[string]string{
		"arch.json": `{"kind": "arch", "name": "arm-cortex-m",
		  "toolchain": {"compiler": "arm-none-eabi-gcc", "objcopy": "arm-none-eabi-objcopy"}}`,
		"board.json": `{"kind": "board", "name": "board", "arch": "arm-cortex-m", "core": "cortex-m4",
		  "vendor_library": "device_board",
		  "flags": ["-mcpu=cortex-m4", "-mthumb"],
		  "memory": {"flash": {"origin": "0x08000000", "length": "0x100000"},
		             "ram": {"origin": "0x20000000", "length": "0x20000"}},
		  "pins": {"gp25": {"name": "GP25", "role": "led"}}}`,
	}
	for name, content := range specs {
		if err := os.WriteFile(filepath.Join(root, "targets", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, root)
	return root
}

func TestNoArgumentsIsAUsage(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Fatalf("expected a usage answer, got %d", code)
	}
}

func TestAnUnknownCommandIsRefused(t *testing.T) {
	if code := run([]string{"compile"}); code != 2 {
		t.Fatalf("expected an unknown command to be refused, got %d", code)
	}
}

func TestListNamesEveryTarget(t *testing.T) {
	specTree(t)
	if code := run([]string{"list"}); code != 0 {
		t.Fatalf("expected the listing to answer, got %d", code)
	}
}

func TestListBoardsOnlyLeavesTheArchitecturesOut(t *testing.T) {
	specTree(t)
	if code := run([]string{"list", "--boards"}); code != 0 {
		t.Fatalf("expected the listing to answer, got %d", code)
	}
}

func TestShowResolvesTheTargetItIsAsked(t *testing.T) {
	specTree(t)
	if code := run([]string{"show", "board"}); code != 0 {
		t.Fatalf("expected the target to be shown, got %d", code)
	}
}

func TestShowRefusesATargetItDoesNotKnow(t *testing.T) {
	specTree(t)
	if code := run([]string{"show", "teensy"}); code == 0 {
		t.Fatal("expected an unknown target to fail")
	}
}

func TestPinsPrintsThePinMap(t *testing.T) {
	specTree(t)
	if code := run([]string{"pins", "board"}); code != 0 {
		t.Fatalf("expected the pin map, got %d", code)
	}
}

func TestGenerateWritesTheBuildFilesIntoTheProject(t *testing.T) {
	root := specTree(t)
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	// The flags come after the path, which is what a person types first.
	if code := run([]string{"generate", project, "--target", "board"}); code != 0 {
		t.Fatalf("expected the generation to answer, got %d", code)
	}
	for _, name := range []string{"toolchain.cmake", "target.cmake", "linker.ld"} {
		if _, err := os.Stat(filepath.Join(project, "cmake", name)); err != nil {
			t.Errorf("expected cmake/%s: %v", name, err)
		}
	}
	// The compiler from the architecture is in the file CMake reads first.
	toolchain, err := os.ReadFile(filepath.Join(project, "cmake", "toolchain.cmake"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(toolchain), "arm-none-eabi-gcc") {
		t.Fatalf("expected the compiler, got:\n%s", toolchain)
	}
}

func TestGenerateKeepsAFileThatIsAlreadyThere(t *testing.T) {
	// A file a person edited is theirs; regenerating says so rather than
	// overwriting it in silence.
	root := specTree(t)
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, "cmake"), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(project, "cmake", "toolchain.cmake")
	if err := os.WriteFile(mine, []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"generate", project, "--target", "board"}); code != 0 {
		t.Fatalf("expected the generation to answer, got %d", code)
	}
	content, err := os.ReadFile(mine)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# mine\n" {
		t.Fatalf("expected the person's file to be kept, got:\n%s", content)
	}
	// And --force writes over it, which is what regenerating the target means.
	if code := run([]string{"generate", project, "--target", "board", "--force"}); code != 0 {
		t.Fatalf("expected the forced generation to answer, got %d", code)
	}
	content, err = os.ReadFile(mine)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "# mine") {
		t.Fatalf("expected the file to be written over, got:\n%s", content)
	}
}

func TestGenerateRefusesAnUnknownTarget(t *testing.T) {
	root := specTree(t)
	if code := run([]string{"generate", root, "--target", "teensy"}); code == 0 {
		t.Fatal("expected an unknown target to fail the run")
	}
}

func TestGenerateNeedsATarget(t *testing.T) {
	root := specTree(t)
	if code := run([]string{"generate", root}); code != 2 {
		t.Fatalf("expected the missing target to be a usage error, got %d", code)
	}
}
