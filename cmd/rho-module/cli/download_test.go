//
// @file download_test.go
// @brief Tests for `rho module download` - Enterprise v3.
//
// Validates:
//   - CLI argument validation
//   - error handling
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

func TestDownloadNoArgs(t *testing.T) {
	err := Download([]string{})
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("expected usage error, got: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid Args                                                         */
/* -------------------------------------------------------------------------- */

func TestDownloadInvalidArgs(t *testing.T) {
	err := Download([]string{"one", "two"})
	if err == nil {
		t.Fatalf("expected error for too many args")
	}
}
