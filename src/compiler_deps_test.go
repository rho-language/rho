//
// @file compiler_deps_test.go
// @brief Tests for the include-level dependency check.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rho

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDependencyManifest(t *testing.T, root, deps string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: demo\n" + deps
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "src", "main.rho")
}

func TestCheckModuleDependencyUnderEcosystemGroup(t *testing.T) {
	root := t.TempDir()
	source := writeDependencyManifest(t, root, "deps:\n  acme:\n    notifications: \">=0.1.0\"\n")
	if err := checkModuleDependency(source, "notifications"); err != nil {
		t.Fatalf("declared dependency was refused: %v", err)
	}
	err := checkModuleDependency(source, "forms")
	if err == nil || !strings.Contains(err.Error(), "does not declare") {
		t.Fatalf("missing dependency was accepted: %v", err)
	}
}

func TestCheckModuleDependencyAcceptsADirectDependency(t *testing.T) {
	root := t.TempDir()
	source := writeDependencyManifest(t, root, "deps:\n  notifications: \">=0.1.0\"\n")
	if err := checkModuleDependency(source, "notifications"); err != nil {
		t.Fatalf("a dependency declared directly was refused: %v", err)
	}
	if err := checkModuleDependency(source, "forms"); err == nil || !strings.Contains(err.Error(), "under deps") {
		t.Fatalf("an undeclared dependency was accepted: %v", err)
	}
}

func TestIncludedModuleNameFindsPlugins(t *testing.T) {
	if got := includedModuleName("/repo/plugins/alert-log/src/main.rho"); got != "alert-log" {
		t.Fatalf("plugin include = %q", got)
	}
	if got := includedModuleName("/repo/modules/forms/src/main.rho"); got != "forms" {
		t.Fatalf("module include = %q", got)
	}
	if got := includedModuleName("/repo/modules/forms/src/text.rho"); got != "" {
		t.Fatalf("sibling file = %q", got)
	}
}

func TestResolveBareEntityIncludes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "applications", "demo", "src", "main.rho")
	for _, path := range []string{
		filepath.Join(root, "modules", "forms", "src", "main.rho"),
		filepath.Join(root, "plugins", "alert-log", "src", "main.rho"),
		filepath.Join(root, "providers", "mesh", "src", "main.rho"),
		filepath.Join(root, "contracts", "alert", "manager.rho"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("module test\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{
		"forms":     filepath.Join(root, "modules", "forms", "src", "main.rho"),
		"alert-log": filepath.Join(root, "plugins", "alert-log", "src", "main.rho"),
		"mesh":      filepath.Join(root, "providers", "mesh", "src", "main.rho"),
		"alert":     filepath.Join(root, "contracts", "alert", "manager.rho"),
	} {
		got, ok := resolveInclude(source, name)
		if !ok || got != want {
			t.Errorf("resolveInclude(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
}

func TestResolveBareIncludeFromDependencyRoot(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "app", "src", "main.rho")
	dependency := filepath.Join(t.TempDir(), "forms")
	if err := os.MkdirAll(filepath.Join(dependency, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dependency, "src", "main.rho")
	if err := os.WriteFile(main, []byte("module forms\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got, ok := resolveInclude(source, "forms", dependency)
	if !ok || got != main {
		t.Fatalf("dependency include = %q, %v; want %q", got, ok, main)
	}
}

func TestResolveBareIncludeReadsTheManifestMain(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "contracts", "notify", "manager.rho")
	provider := filepath.Join(root, "providers", "mail")
	entry := filepath.Join(provider, "src", "provider.rho")
	for path, text := range map[string]string{
		source:                                "module notify\n",
		entry:                                 "module mail\n",
		filepath.Join(provider, "module.yml"): "module:\n  name: acme-provider-mail\n  type: provider\n  version: 0.1.0\nmain: src/provider.rho\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"mail", "acme-provider-mail"} {
		got, ok := resolveInclude(source, name)
		if !ok || got != entry {
			t.Errorf("resolveInclude(%q) = %q, %v; want %q", name, got, ok, entry)
		}
	}
}
