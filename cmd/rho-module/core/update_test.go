package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Update Dependencies                                                  */
/* -------------------------------------------------------------------------- */

func TestUpdateDependencies(t *testing.T) {
	tmp := t.TempDir()

	// module.yml - use range constraint to allow upgrade
	os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: app
  type: rho
  version: 0.1.0

main: src/main.rho

deps:
  math: ">=1.0.0 <2.0.0"
`), 0644)

	// src/main.rho
	os.Mkdir(filepath.Join(tmp, "src"), 0755)
	os.WriteFile(filepath.Join(tmp, "src", "main.rho"), []byte("module app"), 0644)

	// old lockfile
	os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte(`
lock:
  math: "1.0.0"
`), 0644)

	// Fake global cache module (new version) - the simulated registry has 1.2.0
	global := filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod", "math@1.2.0")
	os.MkdirAll(global, 0755)
	os.WriteFile(filepath.Join(global, "module.yml"), []byte("module: math"), 0644)

	if err := UpdateDependencies(tmp); err != nil {
		t.Fatalf("UpdateDependencies failed: %v", err)
	}

	// Check updated lockfile
	data, _ := os.ReadFile(filepath.Join(tmp, "rho.lock"))
	// lockfile may have leading newline
	expected := "lock:\n  math: \"1.2.0\"\n"
	if !strings.Contains(string(data), expected) {
		t.Fatalf("rho.lock not updated, got: %q", string(data))
	}

	// Check updated local cache
	if _, err := os.Stat(filepath.Join(tmp, ".rho-deps", "math@1.2.0")); err != nil {
		t.Fatalf("math@1.2.0 not installed locally")
	}
}
