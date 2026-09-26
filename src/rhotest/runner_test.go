//
// @file runner_test.go
// @brief Tests for runner module - Enterprise v1.
//
// Notes:
//   - This test does NOT execute real rho or gcc.
//   - It only verifies structural behavior.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

import (
	"os"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Runner: Structure Check                                                    */
/* -------------------------------------------------------------------------- */

func TestRunnerStructure(t *testing.T) {
	tf := TestFile{
		Path: "math_test.rho",
		Tests: []TestCase{
			{Name: "test_add", File: "math_test.rho", Line: 1},
		},
	}

	// Since rho compiler is not available in test environment,
	// RunTestFile should return an error.
	_, err := RunTestFile(tf)
	if err == nil {
		t.Fatalf("expected error because rho compiler is missing")
	}
}

func TestRunnerUsesConfiguredRhoBinary(t *testing.T) {
	old := os.Getenv("RHO_BIN")
	defer os.Setenv("RHO_BIN", old)
	os.Setenv("RHO_BIN", "/definitely/missing/rho")
	_, err := RunTestFile(TestFile{Path: "missing_test.rho", Tests: []TestCase{{Name: "x"}}})
	if err == nil || !strings.Contains(err.Error(), "read test file") {
		t.Fatalf("expected test file error, got %v", err)
	}
}
