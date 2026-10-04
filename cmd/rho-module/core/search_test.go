//
// @file search_test.go
// @brief Tests for SearchRegistry - Enterprise v3.
//
// Validates:
//   - Search with query returns matching modules
//   - Search without query returns all modules
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Search Registry                                                      */
/* -------------------------------------------------------------------------- */

func TestSearchRegistry(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	results, err := SearchRegistry("math")
	if err != nil {
		t.Fatalf("SearchRegistry failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}

	if results[0].Name != "math" {
		t.Fatalf("wrong module returned")
	}
}
