//
// @file types.go
// @brief Core types for Rho Test Framework - Enterprise v1.
//
// Defines:
//   - TestCase: a single test function
//   - TestFile: a file containing multiple test cases
//   - TestResult: PASS/FAIL result with optional output.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

/* -------------------------------------------------------------------------- */
/*  TestCase                                                                   */
/* -------------------------------------------------------------------------- */

type TestCase struct {
	Name string // test function name
	File string // source file path
	Line int    // line number in file
}

/* -------------------------------------------------------------------------- */
/*  TestFile                                                                   */
/* -------------------------------------------------------------------------- */

type TestFile struct {
	Path  string     // file path
	Tests []TestCase // all test cases in this file
}

/* -------------------------------------------------------------------------- */
/*  TestResult                                                                 */
/* -------------------------------------------------------------------------- */

type TestResult struct {
	Case   TestCase // test case metadata
	Passed bool     // true if test passed
	Output string   // optional textual output from execution
}
