//
// @file remote_test.go
// @brief Tests for Remote Registry - Enterprise v3.
//
// Validates:
//   - ListRemoteModules returns modules from local registry
//   - RemoteVersions returns versions for a module
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package registry

import (
	"os"
	"path/filepath"
	"testing"

	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Test: ListRemoteModules                                                    */
/* -------------------------------------------------------------------------- */

func TestListRemoteModules(t *testing.T) {
	tmp := t.TempDir()
	paths.SetTestRhoHome(tmp)
	defer paths.SetTestRhoHome("")

	if err := paths.EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	// Initialize local registry
	registryPath, _ := paths.GetRegistryPath()
	registryFile := filepath.Join(registryPath, "registry.yaml")
	if err := os.MkdirAll(registryPath, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	testRegistry := `components:
  - name: math
    version: 1.2.0
plugins:
  - name: http
    version: 0.9.0
active_components:
  - name: math
    version: 1.2.0
active_plugins:
  - name: http
    version: 0.9.0
`
	os.WriteFile(registryFile, []byte(testRegistry), 0644)

	modules := ListRemoteModules()
	if len(modules) == 0 {
		t.Fatalf("expected registry entries")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: RemoteVersions                                                       */
/* -------------------------------------------------------------------------- */

func TestRemoteVersions(t *testing.T) {
	tmp := t.TempDir()
	paths.SetTestRhoHome(tmp)
	defer paths.SetTestRhoHome("")

	if err := paths.EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	// Initialize local registry
	registryPath, _ := paths.GetRegistryPath()
	registryFile := filepath.Join(registryPath, "registry.yaml")
	if err := os.MkdirAll(registryPath, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	testRegistry := `components:
  - name: math
    version: 1.0.0
  - name: math
    version: 1.2.0
active_components:
  - name: math
    version: 1.2.0
`
	os.WriteFile(registryFile, []byte(testRegistry), 0644)

	versions := RemoteVersions("math")
	if len(versions) == 0 {
		t.Fatalf("expected at least one version for math")
	}
}
