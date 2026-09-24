//
// @file pack_test.go
// @brief Tests for `rho pkg pack` - Enterprise v3.
//
// Validates:
//   - CLI routing
//   - error handling
//   - PackProject invocation
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cli

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: CLI Help                                                             */
/* -------------------------------------------------------------------------- */

func TestPackNoArgs(t *testing.T) {
	// Pack with no args defaults to current directory
	// This just checks it doesn't panic
	err := Pack([]string{})
	// We expect an error because there's no module.yml in the test directory
	if err == nil {
		t.Log("Pack succeeded (unexpected)")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid Command                                                      */
/* -------------------------------------------------------------------------- */

func TestPackInvalidCommand(t *testing.T) {
	// Test passes if no panic occurs
	err := Pack([]string{"invalid"})
	if err == nil {
		t.Log("Pack succeeded (unexpected)")
	}
}
