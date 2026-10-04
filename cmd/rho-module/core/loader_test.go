package core

import (
	"os"
	"path/filepath"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: LoadInstalledModule                                                  */
/* -------------------------------------------------------------------------- */

func TestLoadInstalledModule(t *testing.T) {
	tmp := t.TempDir()
	modulesDir := filepath.Join(tmp, ".rho", "modules", "installed", "component")
	os.MkdirAll(modulesDir, 0755)

	// Fake module folder
	modDir := filepath.Join(modulesDir, "rho-lsp@0.1.0")
	os.MkdirAll(modDir, 0755)

	// Fake manifest
	manifest := `
module:
  name: rho-lsp
  type: component
  version: 0.1.0
  command: lsp

main: rho-lsp
`
	os.WriteFile(filepath.Join(modDir, "module.yml"), []byte(manifest), 0644)

	// Override HOME
	os.Setenv("HOME", tmp)

	mod, err := LoadInstalledModule("rho-lsp")
	if err != nil {
		t.Fatalf("LoadInstalledModule failed: %v", err)
	}

	if mod.Name != "rho-lsp" {
		t.Fatalf("wrong module name")
	}
	if mod.Version != "0.1.0" {
		t.Fatalf("wrong module version")
	}
}
