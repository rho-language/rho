package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Entry                                                                */
/* -------------------------------------------------------------------------- */

func TestModuleEntry(t *testing.T) {
	tmp := t.TempDir()

	mod := &Module{
		Name:     "rho-lsp",
		Version:  "0.1.0",
		RootPath: tmp,
		Manifest: ModuleYAML{
			Main: "src/server.rho",
		},
	}

	os.MkdirAll(filepath.Join(tmp, "src"), 0755)
	os.WriteFile(filepath.Join(tmp, "src/server.rho"), []byte("print(\"ok\")"), 0644)

	entry := mod.Entry()
	if entry == nil {
		t.Fatalf("expected entry")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Run (only checks existence)                                          */
/* -------------------------------------------------------------------------- */

func TestModuleEntryRun(t *testing.T) {
	tmp := t.TempDir()

	mod := &Module{
		Name:     "rho-lsp",
		Version:  "0.1.0",
		RootPath: tmp,
		Manifest: ModuleYAML{
			Main: "src/server.rho",
		},
	}

	os.MkdirAll(filepath.Join(tmp, "src"), 0755)
	os.WriteFile(filepath.Join(tmp, "src/server.rho"), []byte("print(\"ok\")"), 0644)

	entry := mod.Entry()
	if entry == nil {
		t.Fatalf("expected entry")
	}

	// A source file is not a program, and the error says that rather than blaming
	// a runner that was never there.
	err := entry.Run([]string{}, os.Stdin, os.Stdout)
	if err == nil || !strings.Contains(err.Error(), "is a source file") {
		t.Fatalf("expected a source file to be refused as one, got %v", err)
	}
}

// A main that is an executable is run, with the module's root in its environment.
func TestModuleEntryRunsAnExecutable(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "tool")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$RHO_MODULE_ROOT\" > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	mod := &Module{Name: "tool", Version: "0.1.0", RootPath: tmp, Manifest: ModuleYAML{Main: "tool"}}

	out := filepath.Join(tmp, "out.txt")
	if err := mod.Entry().Run([]string{out}, os.Stdin, os.Stdout); err != nil {
		t.Fatalf("an executable main was not run: %v", err)
	}
	got, _ := os.ReadFile(out)
	if strings.TrimSpace(string(got)) != tmp {
		t.Errorf("the module root was %q, want %q", got, tmp)
	}
}
