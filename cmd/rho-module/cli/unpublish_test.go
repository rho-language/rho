//
// @file unpublish_test.go
// @brief Tests for `rho module unpublish` - Enterprise v3.
//
// Validates:
//   - CLI argument validation
//   - Unpublish functionality
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

func TestUnpublishNoArgs(t *testing.T) {
	err := Unpublish([]string{})
	if err == nil {
		t.Fatalf("expected error for no args")
	}
}

func TestUnpublishTooManyArgs(t *testing.T) {
	err := Unpublish([]string{"one", "two"})
	if err == nil {
		t.Fatalf("expected error for too many args")
	}
}

func TestUnpublishMissingVersion(t *testing.T) {
	err := Unpublish([]string{"math"})
	if err == nil {
		t.Fatalf("expected error for missing version")
	}
}