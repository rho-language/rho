//
// @file activate_test.go
// @brief Tests for `rho module activate` - Enterprise v3.
//
// Validates:
//   - CLI argument validation
//   - Activate functionality
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: CLI Argument Validation                                              */
/* -------------------------------------------------------------------------- */

func TestActivateNoArgs(t *testing.T) {
	err := Activate([]string{})
	if err == nil {
		t.Fatalf("expected error for no args")
	}
}

func TestActivateTooManyArgs(t *testing.T) {
	err := Activate([]string{"one", "two"})
	if err == nil {
		t.Fatalf("expected error for too many args")
	}
}

func TestActivateMissingVersion(t *testing.T) {
	err := Activate([]string{"math"})
	if err == nil {
		t.Fatalf("expected error for missing version")
	}
}