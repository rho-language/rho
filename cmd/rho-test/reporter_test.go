//
// @file reporter_test.go
// @brief Tests for reporter module - Enterprise v1.
//
// Notes:
//   - Only checks that Report() runs without crashing.
//   - No stdout capture yet.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import "testing"

/* -------------------------------------------------------------------------- */
/*  Reporter: Basic Execution                                                  */
/* -------------------------------------------------------------------------- */

func TestReporterBasic(t *testing.T) {
	results := []TestResult{
		{
			Case:   TestCase{Name: "test_a", File: "a_test.rho", Line: 1},
			Passed: true,
			Output: "ok",
		},
		{
			Case:   TestCase{Name: "test_b", File: "b_test.rho", Line: 2},
			Passed: false,
			Output: "fail",
		},
	}

	// Just ensure it doesn't crash.
	Report(results)
}
