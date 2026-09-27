//
// @file main_test.go
// @brief Tests for the rho kconfig command.
//
// The command is what a build and a person both call, so what is tested here is
// the shape of the request rather than the merge: which flags a caller may pass,
// what an unflagged run reads, and that a run which cannot do what it was asked
// says so and stops.
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

// project is a project that declares a configuration bridge.
func project(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"module.yml": `module:
  name: sensorhub
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: c

config:
  dialect: freertos
  kconfig_fragments:
    - kconfig/board.Kconfig
  values:
    CONFIG_UART_BAUD: "57600"
`,
		"kconfig/board.Kconfig": "config CONFIG_UART_BAUD\n\tint\n\tdefault 115200\n",
		"src/main.rho":          "module main\n\nfn main() void {\n    print(\"x\");\n}\n",
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

func TestNoArgumentsIsAUsageAndNotARun(t *testing.T) {
	// A command called with nothing prints how to call it rather than guessing.
	if code := run(nil); code != 2 {
		t.Fatalf("expected a usage answer, got %d", code)
	}
}

func TestAnUnknownCommandIsRefused(t *testing.T) {
	if code := run([]string{"frobnicate"}); code != 2 {
		t.Fatalf("expected an unknown command to be refused, got %d", code)
	}
}

func TestHelpAnswers(t *testing.T) {
	if code := run([]string{"--help"}); code != 0 {
		t.Fatalf("expected help to answer, got %d", code)
	}
}

func TestValuesReportsWhatAProjectDeclared(t *testing.T) {
	// A person asking what a project configured starts by asking what it
	// declared, and the answer is its own manifest read back.
	root := project(t)
	if code := run([]string{"values", "--root", root}); code != 0 {
		t.Fatalf("expected values to answer, got %d", code)
	}
}

func TestValuesSaysSoWhenAProjectDeclaresNothing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"),
		[]byte("module:\n  name: plain\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"values", "--root", root}); code != 0 {
		t.Fatalf("expected a project with no bridge to answer, got %d", code)
	}
}

func TestMergeWithAProjectsOwnDeclarationWritesItsDialect(t *testing.T) {
	// The unflagged run a person does: the project says what it declared, and
	// the merge writes the dialect it named.
	root := project(t)
	if code := run([]string{"merge", "--root", root, "--force", "--quiet"}); code != 0 {
		t.Fatalf("expected the merge to answer, got %d", code)
	}
	header, err := os.ReadFile(filepath.Join(root, "FreeRTOSConfig.h"))
	if err != nil {
		t.Fatalf("expected the FreeRTOS header, got %v", err)
	}
	if !strings.Contains(string(header), "#define UART_BAUD 57600") {
		t.Fatalf("expected the project's value in the header, got:\n%s", header)
	}
}

func TestMergeWithAFragmentItCannotReadStops(t *testing.T) {
	// A build whose configuration cannot be read is a build that should stop
	// rather than compile against whatever was there.
	root := t.TempDir()
	if code := run([]string{"merge", "--root", root, "--dialect", "linux", "absent.Kconfig"}); code == 0 {
		t.Fatal("expected a missing fragment to fail the run")
	}
}

func TestMergeWithAnUnknownDialectStops(t *testing.T) {
	root := t.TempDir()
	fragment := filepath.Join(root, "board.Kconfig")
	if err := os.WriteFile(fragment, []byte("config CONFIG_ONE\n\tbool\n\tdefault y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"merge", "--root", root, "--dialect", "zephyr", "board.Kconfig"}); code == 0 {
		t.Fatal("expected an unknown dialect to fail the run")
	}
}

func TestSetNeedsTheFragmentThatDeclaresTheSymbol(t *testing.T) {
	// Setting a value is checked against the fragments, so the fragment is
	// needed: without it there is nothing to check the symbol against.
	root := project(t)
	// Asking for the fragment is the right answer: a symbol is only a symbol
	// because some fragment declares it, and a value nothing reads is a setting
	// that silently did nothing.
	if code := run([]string{"set", "--root", root, "CONFIG_UART_BAUD", "9600"}); code != 2 {
		t.Fatalf("expected set to ask for the fragment, got %d", code)
	}
}

func TestSetWritesTheValueItWasGiven(t *testing.T) {
	root := project(t)
	fragment := filepath.Join(root, "kconfig", "board.Kconfig")
	if code := run([]string{"set", "--root", root, "--fragment", fragment, "CONFIG_UART_BAUD", "9600"}); code != 0 {
		t.Fatalf("expected set to answer, got %d", code)
	}
	written, err := os.ReadFile(filepath.Join(root, "kconfig", "rho-defconfig"))
	if err != nil {
		t.Fatalf("expected the defconfig to be written, got %v", err)
	}
	if !strings.Contains(string(written), "CONFIG_UART_BAUD=9600") {
		t.Fatalf("expected the value, got:\n%s", written)
	}
}
