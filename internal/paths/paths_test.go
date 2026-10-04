//
// @file paths_test.go
// @brief Tests for Rho Shared Paths - Enterprise v3.
//
// Validates:
//   - GetRhoHome returns correct path
//   - SetTestRhoHome overrides path for testing
//   - GetRegistryPath, GetModulesInstalledPath, GetCachePath
//   - EnsurePaths creates directory structure
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package paths

import (
	"os"
	"path/filepath"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: GetRhoHome                                                           */
/* -------------------------------------------------------------------------- */

func TestGetRhoHome(t *testing.T) {
	// Test with test home set
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	home, err := GetRhoHome()
	if err != nil {
		t.Fatalf("GetRhoHome failed: %v", err)
	}
	if home != tmp {
		t.Fatalf("expected %s, got %s", tmp, home)
	}
}

func TestGetRhoHomeFollowsTheEnvironment(t *testing.T) {
	// A sandbox or a CI runner points the installation somewhere else, and the
	// tool, the module commands and the compiler all have to agree on where.
	tmp := t.TempDir()
	t.Setenv(RhoHomeEnvVar, tmp)

	home, err := GetRhoHome()
	if err != nil {
		t.Fatalf("GetRhoHome failed: %v", err)
	}
	if home != tmp {
		t.Fatalf("expected %s, got %s", tmp, home)
	}
}

func TestTestRhoHomeOverridesTheEnvironment(t *testing.T) {
	// The test override is the stronger of the two, so a test that installs a
	// Rho home for itself is not redirected by the ambient environment.
	t.Setenv(RhoHomeEnvVar, t.TempDir())
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	home, err := GetRhoHome()
	if err != nil {
		t.Fatalf("GetRhoHome failed: %v", err)
	}
	if home != tmp {
		t.Fatalf("expected %s, got %s", tmp, home)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: GetRegistryPath                                                      */
/* -------------------------------------------------------------------------- */

func TestGetRegistryPath(t *testing.T) {
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	path, err := GetRegistryPath()
	if err != nil {
		t.Fatalf("GetRegistryPath failed: %v", err)
	}
	expected := filepath.Join(tmp, "registry", "local")
	if path != expected {
		t.Fatalf("expected %s, got %s", expected, path)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: GetModulesInstalledPath                                              */
/* -------------------------------------------------------------------------- */

func TestGetModulesInstalledPath(t *testing.T) {
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	path, err := GetModulesInstalledPath()
	if err != nil {
		t.Fatalf("GetModulesInstalledPath failed: %v", err)
	}
	expected := filepath.Join(tmp, "modules", "installed")
	if path != expected {
		t.Fatalf("expected %s, got %s", expected, path)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: GetCachePath                                                         */
/* -------------------------------------------------------------------------- */

func TestGetCachePath(t *testing.T) {
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	path, err := GetCachePath()
	if err != nil {
		t.Fatalf("GetCachePath failed: %v", err)
	}
	expected := filepath.Join(tmp, "pkg", "mod")
	if path != expected {
		t.Fatalf("expected %s, got %s", expected, path)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: EnsurePaths                                                          */
/* -------------------------------------------------------------------------- */

func TestEnsurePaths(t *testing.T) {
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	if err := EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	// Check all directories were created
	dirs := []string{
		filepath.Join(tmp, "bin"),
		filepath.Join(tmp, "modules", "installed"),
		filepath.Join(tmp, "registry", "local"),
		filepath.Join(tmp, "cache"),
		filepath.Join(tmp, "config"),
		filepath.Join(tmp, "pkg", "mod"),
	}

	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Fatalf("directory %s not created: %v", d, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", d)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: SetTestRhoHome Reset                                                 */
/* -------------------------------------------------------------------------- */

func TestSetTestRhoHomeReset(t *testing.T) {
	tmp := t.TempDir()
	SetTestRhoHome(tmp)

	// Reset
	SetTestRhoHome("")

	// Should fall back to real home dir (no error, but won't be tmp)
	home, err := GetRhoHome()
	if err != nil {
		t.Fatalf("GetRhoHome failed after reset: %v", err)
	}
	if home == tmp {
		t.Fatalf("expected different path after reset")
	}
}