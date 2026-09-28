//
// @file testutil.go
// @brief Test utilities for rho-module - Enterprise v3.
//
// Provides common test setup functions.
// Uses SetTestRhoHome for test isolation.
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

	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Test Setup                                                                 */
/* -------------------------------------------------------------------------- */

// SetupTestRegistry creates a temporary Rho home and initializes
// the local registry with test data.
// Returns the temp dir and a cleanup function.
func SetupTestRegistry(t *testing.T) (string, func()) {
	tmp := t.TempDir()
	paths.SetTestRhoHome(tmp)
	deferFunc := func() {
		paths.SetTestRhoHome("")
	}

	// Ensure paths
	if err := paths.EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	// Initialize local registry
	if err := InitLocalRegistry(); err != nil {
		t.Fatalf("InitLocalRegistry failed: %v", err)
	}

	// Populate with test data
	registryPath, _ := paths.GetRegistryPath()
	registryFile := filepath.Join(registryPath, "registry.yaml")

	testRegistry := `components:
  - name: math
    version: 1.0.0
  - name: math
    version: 1.2.0
  - name: io
    version: 1.0.0
  - name: io
    version: 1.1.0
plugins:
  - name: http
    version: 0.9.0
active_components:
  - name: math
    version: 1.2.0
  - name: io
    version: 1.1.0
active_plugins:
  - name: http
    version: 0.9.0
`
	os.WriteFile(registryFile, []byte(testRegistry), 0644)

	// The index says what is known about the installed modules; the tree is
	// what is installed. A fixture that wrote only the index described a
	// registry holding nothing, which is a state the toolchain cannot be in
	// without someone having deleted the directories by hand — so the fixture
	// installs them, the way a publish or a fetch does.
	installed := [][2]string{
		{"component", "math@1.0.0"},
		{"component", "math@1.2.0"},
		{"component", "io@1.0.0"},
		{"component", "io@1.1.0"},
		{"plugin", "http@0.9.0"},
	}
	modulesRoot, err := paths.GetModulesInstalledPath()
	if err != nil {
		t.Fatalf("GetModulesInstalledPath failed: %v", err)
	}
	for _, entry := range installed {
		dir := filepath.Join(modulesRoot, entry[0], entry[1])
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("cannot create %s: %v", dir, err)
		}
	}

	// Also create local cache for getLocalVersions tests
	cachePath, _ := paths.GetCachePath()
	os.MkdirAll(filepath.Join(cachePath, "math@1.0.0"), 0755)
	os.MkdirAll(filepath.Join(cachePath, "io@1.0.0"), 0755)
	os.MkdirAll(filepath.Join(cachePath, "io@1.1.0"), 0755)

	return tmp, deferFunc
}

/* -------------------------------------------------------------------------- */
/*  Test Setup for Publisher (requires Go module)                              */
/* -------------------------------------------------------------------------- */

// SetupTestPublisher creates a temporary Rho home and a minimal
// Go module for publishing tests.
func SetupTestPublisher(t *testing.T) (string, func()) {
	tmp := t.TempDir()
	paths.SetTestRhoHome(tmp)
	deferFunc := func() {
		paths.SetTestRhoHome("")
	}

	// Ensure paths
	if err := paths.EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	// Initialize local registry
	if err := InitLocalRegistry(); err != nil {
		t.Fatalf("InitLocalRegistry failed: %v", err)
	}

	return tmp, deferFunc
}

/* -------------------------------------------------------------------------- */
/*  InitLocalRegistry (for test setup)                                         */
/* -------------------------------------------------------------------------- */

func InitLocalRegistry() error {
	registryPath, err := paths.GetRegistryPath()
	if err != nil {
		return err
	}
	registryFile := filepath.Join(registryPath, "registry.yaml")

	// Create empty registry if not exists
	if _, err := os.Stat(registryFile); err != nil {
		emptyRegistry := `components:
plugins:
active_components:
active_plugins:
`
		return os.WriteFile(registryFile, []byte(emptyRegistry), 0644)
	}
	return nil
}
