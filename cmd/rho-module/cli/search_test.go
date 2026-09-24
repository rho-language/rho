//
// @file search_test.go
// @brief Tests for `rho module search` - Enterprise v3.
//
// Validates:
//   - CLI argument validation
//   - search with query
//   - search without query (lists all)
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

func TestSearchNoArgs(t *testing.T) {
	// Now returns all modules without error
	err := Search([]string{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid Args                                                         */
/* -------------------------------------------------------------------------- */

func TestSearchInvalidArgs(t *testing.T) {
	err := Search([]string{"one", "two"})
	if err == nil {
		t.Fatalf("expected error for too many args")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Search With Query                                                    */
/* -------------------------------------------------------------------------- */

func TestSearchWithQuery(t *testing.T) {
	err := Search([]string{"math"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
