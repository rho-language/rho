//
// @file reporter.go
// @brief Prints test results - Rho Test Framework (Enterprise v1)
//
// Responsibilities:
//   - Print PASS/FAIL for each test case
//   - Print final summary
//   - Minimal formatting, no colors yet
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import "fmt"

/* -------------------------------------------------------------------------- */
/*  Report                                                                     */
/* -------------------------------------------------------------------------- */

func Report(results []TestResult) {
	pass := 0
	fail := 0

	for _, r := range results {
		if r.Passed {
			fmt.Printf("[PASS] %s\n", r.Case.Name)
			pass++
		} else {
			fmt.Printf("[FAIL] %s\n", r.Case.Name)
			if r.Output != "" {
				fmt.Printf("       %s\n", r.Output)
			}
			fail++
		}
	}

	fmt.Printf("\n%d tests, %d passed, %d failed\n", len(results), pass, fail)
}
