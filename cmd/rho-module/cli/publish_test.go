//
// @file publish_test.go
// @brief Tests for `rho module publish` - Enterprise v3.
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

func TestPublishNoArgs(t *testing.T) {
	err := Publish([]string{})
	if err == nil {
		t.Fatalf("expected error for missing module.yml")
	}
	// Should fail because current dir doesn't have module.yml
	if !strings.Contains(err.Error(), "module.yml") {
		t.Fatalf("expected module.yml error, got: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid Args                                                         */
/* -------------------------------------------------------------------------- */

func TestPublishInvalidArgs(t *testing.T) {
	err := Publish([]string{"one", "two"})
	if err == nil {
		t.Fatalf("expected error for too many args")
	}
}
