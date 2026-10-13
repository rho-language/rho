package core

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/deps"
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

/* -------------------------------------------------------------------------- */
/*  Test: Which version a name means                                          */
/* -------------------------------------------------------------------------- */

func TestCompareVersionsOrdersDottedNumbers(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.1.0", "0.1.1", -1},
		{"0.2.0", "0.1.9", 1},
		{"1.0.0", "0.9.9", 1},
		{"0.1.10", "0.1.9", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.left, c.right); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.left, c.right, got, c.want)
		}
	}
}

// Several versions can be installed at once and only one runs. A name means the
// enabled one when something is enabled, and the newest otherwise — never
// whichever directory a scan reached first.
func TestLoadInstalledModulePrefersTheEnabledVersion(t *testing.T) {
	tmp := t.TempDir()
	componentDir := filepath.Join(tmp, ".rho", "modules", "installed", "component")
	for _, version := range []string{"0.1.0", "0.1.1"} {
		dir := filepath.Join(componentDir, "rho-lsp@"+version)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := "module:\n  name: rho-lsp\n  type: component\n  version: " + version +
			"\n  command: lsp\n\nmain: rho-lsp\n"
		if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", tmp)
	t.Setenv("RHO_HOME", filepath.Join(tmp, ".rho"))

	mod, err := LoadInstalledModule("rho-lsp")
	if err != nil {
		t.Fatal(err)
	}
	if mod.Version != "0.1.1" {
		t.Fatalf("with nothing enabled the newest should be meant, got %s", mod.Version)
	}

	// The enabled version wins even when it is not the newest.
	if err := deps.SetVersionEnabled("component", "rho-lsp", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	mod, err = LoadInstalledModule("rho-lsp")
	if err != nil {
		t.Fatal(err)
	}
	if mod.Version != "0.1.0" {
		t.Fatalf("the enabled version should be meant, got %s", mod.Version)
	}
}
