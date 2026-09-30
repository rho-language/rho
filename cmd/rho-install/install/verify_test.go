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
