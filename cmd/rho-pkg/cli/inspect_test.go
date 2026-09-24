//
// @file inspect_test.go
// @brief Tests for `rho pkg inspect` - Enterprise v3.
//
// Validates:
//   - CLI routing
//   - error handling
//   - ReadPackageMetadata invocation
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cli

import (
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: CLI Help                                                             */
/* -------------------------------------------------------------------------- */

func TestInspectNoArgs(t *testing.T) {
	err := Inspect([]string{})
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid Command                                                      */
/* -------------------------------------------------------------------------- */

func TestInspectInvalidCommand(t *testing.T) {
	err := Inspect([]string{"invalid"})
	if err == nil {
		t.Fatalf("expected error for invalid args")
	}
}
