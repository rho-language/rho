//
// @file bin_test.go
// @brief Tests for Rho Install - Binary deployment subsystem.
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package install

import (
	"os"
	"path/filepath"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  InstallBinary                                                              */
/* -------------------------------------------------------------------------- */

func TestInstallBinary(t *testing.T) {
	// Use temp dir for test isolation
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	// Prepare fake binary
	src := filepath.Join(tmp, "rho-test-bin")
	os.WriteFile(src, []byte("#!/bin/sh\necho test"), 0o755)

	// Ensure ~/.rho exists
	if err := EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	// Install
	if err := InstallBinary(src); err != nil {
		t.Fatalf("InstallBinary failed: %v", err)
	}

	// Verify
	binPath, _ := GetBinPath()
	dst := filepath.Join(binPath, "rho-test-bin")

	if _, err := os.Stat(dst); os.IsNotExist(err) {
		t.Fatalf("binary not installed: %s", dst)
	}
}

/* -------------------------------------------------------------------------- */
/*  InstallAllBinaries                                                         */
/* -------------------------------------------------------------------------- */

func TestInstallAllBinaries(t *testing.T) {
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	// Create fake binaries
	names := []string{
		"rho", "rho-deps", "rho-lsp",
	}

	for _, n := range names {
		os.WriteFile(filepath.Join(tmp, n), []byte("test"), 0o755)
	}

	if err := EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	if err := InstallAllBinaries(tmp); err != nil {
		t.Fatalf("InstallAllBinaries failed: %v", err)
	}

	binPath, _ := GetBinPath()

	for _, n := range names {
		if _, err := os.Stat(filepath.Join(binPath, n)); os.IsNotExist(err) {
			t.Fatalf("binary missing after InstallAllBinaries: %s", n)
		}
	}
}
