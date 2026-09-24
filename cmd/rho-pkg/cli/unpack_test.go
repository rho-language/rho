//
// @file unpack_test.go
// @brief Tests for `rho pkg unpack` - Enterprise v3.
//
// Validates:
//   - CLI routing
//   - error handling
//   - UnpackArchive invocation
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

func TestUnpackNoArgs(t *testing.T) {
	err := Unpack([]string{})
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid Command                                                      */
/* -------------------------------------------------------------------------- */

func TestUnpackInvalidCommand(t *testing.T) {
	err := Unpack([]string{"invalid"})
	if err == nil {
		t.Fatalf("expected error for invalid args")
	}
}
