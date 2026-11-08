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

func TestCheckModuleDependencyUnderRobohub(t *testing.T) {
	root := t.TempDir()
	source := writeDependencyManifest(t, root, "deps:\n  robohub:\n    notifications: \">=0.1.0\"\n")
	if err := checkModuleDependency(source, "notifications"); err != nil {
		t.Fatalf("declared dependency was refused: %v", err)
	}
	err := checkModuleDependency(source, "forms")
	if err == nil || !strings.Contains(err.Error(), "does not declare") {
		t.Fatalf("missing dependency was accepted: %v", err)
	}
}

func TestCheckModuleDependencyRejectsFlatRobohub(t *testing.T) {
	root := t.TempDir()
	source := writeDependencyManifest(t, root, "deps:\n  notifications: \">=0.1.0\"\n")
	err := checkModuleDependency(source, "notifications")
	if err == nil || !strings.Contains(err.Error(), "deps.robohub") {
		t.Fatalf("flat RoboHub dependency was accepted: %v", err)
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
