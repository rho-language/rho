//
// @file types_test.go
// @brief Tests for core types in Rho Test Framework.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

import "testing"

/* -------------------------------------------------------------------------- */
/*  Basic Types Sanity                                                         */
/* -------------------------------------------------------------------------- */

func TestTypesBasic(t *testing.T) {
	tc := TestCase{
		Name: "test_add",
		File: "math_test.rho",
		Line: 10,
	}

	if tc.Name != "test_add" {
		t.Fatalf("expected test_add, got %s", tc.Name)
	}
	if tc.File != "math_test.rho" {
		t.Fatalf("unexpected file: %s", tc.File)
	}
	if tc.Line != 10 {
		t.Fatalf("unexpected line: %d", tc.Line)
	}

	tf := TestFile{
		Path:  "math_test.rho",
		Tests: []TestCase{tc},
	}
	if tf.Path != "math_test.rho" {
		t.Fatalf("unexpected path: %s", tf.Path)
	}
	if len(tf.Tests) != 1 {
		t.Fatalf("expected 1 test case, got %d", len(tf.Tests))
	}

	res := TestResult{
		Case:   tc,
		Passed: true,
		Output: "ok",
	}
	if !res.Passed {
		t.Fatalf("expected test to pass")
	}
	if res.Output != "ok" {
		t.Fatalf("unexpected output: %s", res.Output)
	}
}
