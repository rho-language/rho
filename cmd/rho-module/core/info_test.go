//
// @file info_test.go
// @brief Tests for ReadModuleInfo - Enterprise v3.
//
// Validates:
//   - module.yml parsing
//   - plugin hooks reading
//   - error handling
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package core

import (
	"os"
	"path/filepath"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: ReadModuleInfo                                                       */
/* -------------------------------------------------------------------------- */

func TestReadModuleInfo(t *testing.T) {
	tmp := t.TempDir()

	// Create module.yml
	moduleYAML := `module:
  name: testmod
  type: module
  version: 1.2.3

main: src/lib.rho

deps:
  math: ">=1.0.0"

features:
  - async
  - crypto
`
	err := os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(moduleYAML), 0644)
	if err != nil {
		t.Fatalf("cannot write module.yml: %v", err)
	}

	// Create src/lib.rho
	err = os.MkdirAll(filepath.Join(tmp, "src"), 0755)
	if err != nil {
		t.Fatalf("cannot create src/: %v", err)
	}
	err = os.WriteFile(filepath.Join(tmp, "src", "lib.rho"), []byte("module testmod\n"), 0644)
	if err != nil {
		t.Fatalf("cannot write lib.rho: %v", err)
	}

	info, err := ReadModuleInfo(tmp)
	if err != nil {
		t.Fatalf("ReadModuleInfo failed: %v", err)
	}

	if info.Module.Name != "testmod" {
		t.Fatalf("expected name 'testmod', got %q", info.Module.Name)
	}
	if info.Module.Type != "module" {
		t.Fatalf("expected type 'module', got %q", info.Module.Type)
	}
	if info.Module.Version != "1.2.3" {
		t.Fatalf("expected version '1.2.3', got %q", info.Module.Version)
	}
	if info.Main != "src/lib.rho" {
		t.Fatalf("expected main 'src/lib.rho', got %q", info.Main)
	}
	if len(info.Deps) != 1 || info.Deps["math"] != ">=1.0.0" {
		t.Fatalf("expected deps map with math, got %v", info.Deps)
	}
	if len(info.Features) != 2 || info.Features[0] != "async" || info.Features[1] != "crypto" {
		t.Fatalf("expected features [async crypto], got %v", info.Features)
	}
	if len(info.Hooks) != 0 {
		t.Fatalf("expected no hooks for module type, got %v", info.Hooks)
	}
}

func TestReadModuleInfoPluginWithHooks(t *testing.T) {
	tmp := t.TempDir()

	// Create module.yml for plugin
	moduleYAML := `module:
  name: testplugin
  type: plugin
  version: 0.5.0

main: src/plugin.rho
`
	err := os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(moduleYAML), 0644)
	if err != nil {
		t.Fatalf("cannot write module.yml: %v", err)
	}

	// Create src/plugin.rho
	err = os.MkdirAll(filepath.Join(tmp, "src"), 0755)
	if err != nil {
		t.Fatalf("cannot create src/: %v", err)
	}
	err = os.WriteFile(filepath.Join(tmp, "src", "plugin.rho"), []byte("module testplugin\n"), 0644)
	if err != nil {
		t.Fatalf("cannot write plugin.rho: %v", err)
	}

	// Create plugin/hooks.yml
	err = os.MkdirAll(filepath.Join(tmp, "plugin"), 0755)
	if err != nil {
		t.Fatalf("cannot create plugin/: %v", err)
	}
	hooksYAML := `hooks:
  - init
  - configure
  - shutdown
`
	err = os.WriteFile(filepath.Join(tmp, "plugin", "hooks.yml"), []byte(hooksYAML), 0644)
	if err != nil {
		t.Fatalf("cannot write hooks.yml: %v", err)
	}

	info, err := ReadModuleInfo(tmp)
	if err != nil {
		t.Fatalf("ReadModuleInfo failed: %v", err)
	}

	if info.Module.Name != "testplugin" {
		t.Fatalf("expected name 'testplugin', got %q", info.Module.Name)
	}
	if info.Module.Type != "plugin" {
		t.Fatalf("expected type 'plugin', got %q", info.Module.Type)
	}
	if len(info.Hooks) != 3 {
		t.Fatalf("expected 3 hooks, got %d: %v", len(info.Hooks), info.Hooks)
	}
	if info.Hooks[0] != "init" || info.Hooks[1] != "configure" || info.Hooks[2] != "shutdown" {
		t.Fatalf("unexpected hooks: %v", info.Hooks)
	}
}

func TestReadModuleInfoMissingFile(t *testing.T) {
	tmp := t.TempDir()

	_, err := ReadModuleInfo(tmp)
	if err == nil {
		t.Fatalf("expected error for missing module.yml")
	}
}

func TestReadModuleInfoInvalidYAML(t *testing.T) {
	tmp := t.TempDir()

	err := os.WriteFile(filepath.Join(tmp, "module.yml"), []byte("invalid: yaml: ["), 0644)
	if err != nil {
		t.Fatalf("cannot write module.yml: %v", err)
	}

	_, err = ReadModuleInfo(tmp)
	if err == nil {
		t.Fatalf("expected error for invalid YAML")
	}
}

func TestReadModuleInfoMissingRequiredFields(t *testing.T) {
	tmp := t.TempDir()

	// Missing module.name
	moduleYAML := `module:
  type: module
  version: 1.0.0

main: src/main.rho
`
	err := os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(moduleYAML), 0644)
	if err != nil {
		t.Fatalf("cannot write module.yml: %v", err)
	}

	_, err = ReadModuleInfo(tmp)
	if err == nil {
		t.Fatalf("expected error for missing module.name")
	}
	if !contains(err.Error(), "module.name missing") {
		t.Fatalf("expected 'module.name missing' in error, got: %v", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}