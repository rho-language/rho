//
// @file loader.go
// @brief Loads test cases from Rho test files - Enterprise v1.
//
// Responsibilities:
//   - Read a .rho file
//   - Detect functions declared as:
//         test fn NAME(...)
//   - Produce a TestFile containing TestCase entries
//
// Notes:
//   - Minimal parser, line-based scanning
//   - No semantic validation here
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

import (
	"bufio"
	"os"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  LoadTests                                                                  */
/* -------------------------------------------------------------------------- */

func LoadTests(path string) (TestFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return TestFile{}, err
	}
	defer f.Close()

	var tests []TestCase
	scanner := bufio.NewScanner(f)
	line := 1

	for scanner.Scan() {
		txt := strings.TrimSpace(scanner.Text())

		// Detect: test fn NAME(
		if strings.HasPrefix(txt, "test fn ") {
			name := strings.TrimPrefix(txt, "test fn ")
			name = strings.Split(name, "(")[0]

			tests = append(tests, TestCase{
				Name: name,
				File: path,
				Line: line,
			})
		}

		line++
	}

	return TestFile{
		Path:  path,
		Tests: tests,
	}, nil
}
