//
// @file main.go
// @brief Rho Test CLI entrypoint - Enterprise v1.
//
// Usage:
//   rho-test <path>
//
// Responsibilities:
//   - Parse CLI arguments
//   - Discover test files
//   - Load test cases
//   - Run test files
//   - Report results
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"fmt"
	"os"
)

/* -------------------------------------------------------------------------- */
/*  main                                                                       */
/* -------------------------------------------------------------------------- */

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) < 1 {
		fmt.Println("Usage: rho-test <path>")
		return 2
	}

	path := args[0]

	/* --------------------------- Discover test files ------------------------ */
	files, err := Discover(path)
	if err != nil {
		fmt.Println("Error:", err)
		return 1
	}

	if len(files) == 0 {
		fmt.Println("No test files found.")
		return 0
	}

	var allResults []TestResult

	/* --------------------------- Load + Run each file ----------------------- */
	for _, f := range files {
		tf, err := LoadTests(f)
		if err != nil {
			fmt.Printf("Error loading %s: %v\n", f, err)
			return 1
		}

		results, err := RunTestFile(tf)
		if err != nil {
			fmt.Printf("Error running %s: %v\n", f, err)
			return 1
		}

		allResults = append(allResults, results...)
	}

	/* --------------------------- Report final results ----------------------- */
	Report(allResults)
	for _, result := range allResults {
		if !result.Passed {
			return 1
		}
	}
	return 0
}
