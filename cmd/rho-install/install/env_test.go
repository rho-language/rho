//
// @file env_test.go
// @brief Tests for Rho Install - Environment setup subsystem.
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  DetectShell                                                                */
/* -------------------------------------------------------------------------- */

func TestDetectShell(t *testing.T) {
	os.Setenv("SHELL", "/bin/bash")
	if DetectShell() != "bash" {
		t.Fatalf("DetectShell failed for bash")
	}

	os.Setenv("SHELL", "/usr/bin/zsh")
	if DetectShell() != "zsh" {
		t.Fatalf("DetectShell failed for zsh")
	}

	os.Setenv("SHELL", "/usr/bin/fish")
	if DetectShell() != "fish" {
		t.Fatalf("DetectShell failed for fish")
	}
}

/* -------------------------------------------------------------------------- */
/*  AddRhoToPath                                                               */
/* -------------------------------------------------------------------------- */

func TestAddRhoToPath(t *testing.T) {
	tmp := t.TempDir()
	SetTestRhoHome(tmp)
	defer SetTestRhoHome("")

	// Override HOME for test
	os.Setenv("HOME", tmp)

	// Prepare ~/.rho/bin
	if err := EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	// Fake shell rc file - should be at parent of rhoHome
	rhoHome, _ := GetRhoHome()
	home := filepath.Dir(rhoHome)
	rc := filepath.Join(home, ".bashrc")
	os.WriteFile(rc, []byte{}, 0o644)

	os.Setenv("SHELL", "/bin/bash")

	if err := AddRhoToPath(); err != nil {
		t.Fatalf("AddRhoToPath failed: %v", err)
	}

	content, _ := os.ReadFile(rc)
	binPath, _ := GetBinPath()
	expectedLine := fmt.Sprintf(`export PATH="%s:$PATH"`, binPath)
	if !strings.Contains(string(content), expectedLine) {
		t.Fatalf("PATH entry missing in rc file: expected %q", expectedLine)
	}
}
