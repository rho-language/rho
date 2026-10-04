//
// @file paths_test.go
// @brief Tests for Rho Install - Paths subsystem.
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
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  GetRhoHome                                                                 */
/* -------------------------------------------------------------------------- */

func TestGetRhoHome(t *testing.T) {
	home, err := GetRhoHome()
	if err != nil {
		t.Fatalf("GetRhoHome failed: %v", err)
	}

	if home == "" {
		t.Fatalf("GetRhoHome returned empty path")
	}
}

/* -------------------------------------------------------------------------- */
/*  EnsurePaths                                                                */
/* -------------------------------------------------------------------------- */

func TestEnsurePaths(t *testing.T) {
	err := EnsurePaths()
	if err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	home, _ := GetRhoHome()

	expected := []string{
		home + "/bin",
		home + "/modules/installed",
		home + "/registry/local",
		home + "/cache",
		home + "/config",
		home + "/auth",
		home + "/pkg/mod",
	}

	for _, p := range expected {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Fatalf("expected directory missing: %s", p)
		}
	}
}
