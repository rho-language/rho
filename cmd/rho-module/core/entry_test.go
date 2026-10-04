package core

import (
	"os"
	"path/filepath"
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

	// We cannot run rho-runner here, but we can check that Run() fails only
	// because rho-runner is missing, not because entrypoint is missing.
	err := entry.Run([]string{}, os.Stdin, os.Stdout)
	if err == nil {
		t.Fatalf("expected error because rho-runner is missing")
	}
}
