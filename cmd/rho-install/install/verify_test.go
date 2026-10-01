//
// @file verify_test.go
// @brief Tests for Rho Install - Installation verification subsystem.
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package install

import (
	"os"
	"path/filepath"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  IsInstalled                                                                */
/* -------------------------------------------------------------------------- */

func TestIsInstalled(t *testing.T) {
	// Use temp dir for test isolation
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	// Prepare full installation
	if err := EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}
	InitLocalRegistry()
	InitCache()
	InitConfig()

	// Fake binaries
	binPath, _ := GetBinPath()
	names := []string{
		"rho", "rho-deps", "rho-lsp",
	}

	for _, n := range names {
		os.WriteFile(filepath.Join(binPath, n), []byte("test"), 0o755)
	}

	if !IsInstalled() {
		t.Fatalf("IsInstalled returned false for a complete installation")
	}
}

// A purge must not make the toolchain report itself uninstalled.
//
// `rho deps purge` removes ~/.rho/pkg/mod by design, and it tells the reader to
// run `rho deps sync` to recover. If the module cache were part of what makes an
// installation count as installed, that purge would leave a machine where every
// command refuses to run — including the sync it just recommended. The cache is
// a cache: it is rebuilt on demand, so its absence says nothing about whether
// the toolchain is there.
func TestIsInstalledSurvivesAPurgedModuleCache(t *testing.T) {
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	if err := EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}
	InitLocalRegistry()
	InitCache()
	InitConfig()

	binPath, _ := GetBinPath()
	for _, n := range []string{"rho", "rho-deps", "rho-lsp"} {
		os.WriteFile(filepath.Join(binPath, n), []byte("test"), 0o755)
	}

	// The module cache is ~/.rho/pkg/mod, which is not what GetMetadataCachePath names:
	// in this package that is ~/.rho/cache, the registry and module metadata
	// cache. Two caches, two names, and the one a purge removes is the first.
	home, err := GetRhoHome()
	if err != nil {
		t.Fatalf("GetRhoHome failed: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(home, "pkg", "mod")); err != nil {
		t.Fatalf("could not purge the module cache: %v", err)
	}

	if !IsInstalled() {
		t.Fatalf("IsInstalled returned false after the module cache was purged")
	}
}

/* -------------------------------------------------------------------------- */
/*  VerifyInstallation                                                         */
/* -------------------------------------------------------------------------- */

func TestVerifyInstallation(t *testing.T) {
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	// Prepare full installation
	EnsurePaths()
	InitLocalRegistry()
	InitCache()
	InitConfig()

	// Fake binaries — only rho, missing rho-deps and rho-lsp
	binPath, _ := GetBinPath()
	os.WriteFile(filepath.Join(binPath, "rho"), []byte("test"), 0o755)

	// Missing other binaries → should fail
	if err := VerifyInstallation(); err == nil {
		t.Fatalf("VerifyInstallation should fail when binaries are missing")
	}
}
