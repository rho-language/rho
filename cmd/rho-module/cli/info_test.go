//
// @file info_test.go
// @brief Tests for `rho module info` - Enterprise v3.
//
// Validates:
//   - CLI routing
//   - error handling
//   - ReadModuleInfo invocation
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

func TestInfoNoArgs(t *testing.T) {
	err := Info([]string{})
	if err == nil {
		t.Fatalf("expected error for missing module.yml")
	}
	// Should fail because current dir doesn't have module.yml
	if !strings.Contains(err.Error(), "module.yml") {
		t.Fatalf("expected module.yml error, got: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid Command                                                      */
/* -------------------------------------------------------------------------- */

func TestInfoInvalidCommand(t *testing.T) {
	err := Info([]string{"invalid"})
	if err == nil {
		t.Fatalf("expected error for invalid args")
	}
}
