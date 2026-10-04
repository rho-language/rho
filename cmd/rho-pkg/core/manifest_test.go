//
// @file manifest_test.go
// @brief Tests for Rho Package Manifest - Enterprise v3.
//
// Validates:
//   - loading module.yml
//   - saving module.yml
//   - validation rules
//   - features list
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Load Valid Manifest                                                  */
/* -------------------------------------------------------------------------- */

func TestLoadValidManifest(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "module.yml")

	content := `
module:
  name: hello-rho
  type: rho
  version: 0.1.0

main: src/main.rho

features:
  - math
  - io
`
	os.WriteFile(path, []byte(content), 0644)

	m, err := LoadModuleYAML(path)
	if err != nil {
		t.Fatalf("LoadModuleYAML failed: %v", err)
	}

	if m.Module.Name != "hello-rho" {
		t.Fatalf("wrong module name")
	}
	if m.Main != "src/main.rho" {
		t.Fatalf("wrong main")
	}
	if len(m.Features) != 2 {
		t.Fatalf("expected 2 features")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Save Manifest                                                        */
/* -------------------------------------------------------------------------- */

func TestSaveManifest(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "module.yml")

	m := ModuleYAML{
		Module: ModuleSection{
			Name:    "hello-rho",
			Type:    "rho",
			Version: "0.1.0",
		},
		Main:     "src/main.rho",
		Features: []string{"math"},
	}

	if err := SaveModuleYAML(path, m); err != nil {
		t.Fatalf("SaveModuleYAML failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("module.yml not written")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Validation Errors                                                    */
/* -------------------------------------------------------------------------- */

func TestValidateManifestErrors(t *testing.T) {
	m := ModuleYAML{
		Module: ModuleSection{
			Name:    "",
			Type:    "rho",
			Version: "0.1.0",
		},
		Main: "src/main.rho",
	}

	if err := ValidateModuleYAML(m); err == nil {
		t.Fatalf("expected validation error for missing name")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Schema Validation Gates Loading                                     */
/* -------------------------------------------------------------------------- */

// writeManifest writes a module.yml into a temp dir and returns its path.
func writeManifest(t *testing.T, content string) string {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "module.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRejectsUnknownType(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: bogus\n  version: 0.1.0\n\nmain: src/main.rho\n")

	_, err := LoadModuleYAML(path)
	if err == nil {
		t.Fatal("expected schema error for unknown module type")
	}
	if !strings.Contains(err.Error(), "module.type") {
		t.Fatalf("expected module.type in error, got: %v", err)
	}
}

func TestLoadRejectsNonRhoMain(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: build/app\n")

	if _, err := LoadModuleYAML(path); err == nil {
		t.Fatal("expected schema error for non-.rho main")
	}
}

func TestLoadRejectsOrphanDependency(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\ndepends:\n  - name: orphan\n    version: 1.0.0\n")

	if _, err := LoadModuleYAML(path); err == nil {
		t.Fatal("expected schema error for dependency without git or path")
	}
}

func TestLoadAcceptsComponentWithCommand(t *testing.T) {
	path := writeManifest(t, "module:\n  name: rho-lsp\n  type: component\n  version: 0.1.0\n  command: lsp\n\nmain: rho-lsp\n")

	m, err := LoadModuleYAML(path)
	if err != nil {
		t.Fatalf("expected component manifest to load, got: %v", err)
	}
	if m.Module.Command != "lsp" {
		t.Fatalf("expected command 'lsp', got %q", m.Module.Command)
	}
}

func TestLoadAcceptsBuildSection(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: c\n  include_dirs:\n    - include\n  sources:\n    - src/app.c\n")

	if _, err := LoadModuleYAML(path); err != nil {
		t.Fatalf("expected build section to validate, got: %v", err)
	}
}

func TestLoadRejectsUnknownBuildLanguage(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: rust\n")

	if _, err := LoadModuleYAML(path); err == nil {
		t.Fatal("expected schema error for unsupported build language")
	}
}
