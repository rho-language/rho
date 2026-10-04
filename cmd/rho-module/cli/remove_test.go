//
// @file remove_test.go
// @brief Tests for `rho module remove` - Enterprise v3.
//
// Validates:
//   - CLI argument validation
//   - error handling
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: CLI Help                                                             */
/* -------------------------------------------------------------------------- */

func TestRemoveNoArgs(t *testing.T) {
	err := Remove([]string{})
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid Args                                                         */
/* -------------------------------------------------------------------------- */

func TestRemoveInvalidArgs(t *testing.T) {
	err := Remove([]string{"one", "two"})
	if err == nil {
		t.Fatalf("expected error for too many args")
	}
}
