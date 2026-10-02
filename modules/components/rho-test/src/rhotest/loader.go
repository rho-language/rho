//
// @file loader.go
// @brief Reading the tests out of a Rho source file.
//
// A Rho test file is an ordinary source file with `test fn` where a `fn` would
// be. The runner strips the marker, compiles what is left and runs it, so what
// this file produces is not a program but a list of the cases in one — which is
// what a report needs to name a failure and what a dry run needs to show.
//
// Named rather than discovered by parsing a directory: a project's tests are what
// its manifest says, and this is how the kind of test this runner runs itself
// says which cases are in a file.
//
// @project Rho
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

// TestMarker is how a Rho function is declared to be a test.
//
// A marker on the declaration rather than a separate file, so a test lives beside
// the code it tests and a case and the thing it covers are edited together.
const TestMarker = "test fn "

// TestFileSuffix is what a file of tests is named, which is also how a manifest
// names one: a suite named `math` is `math_test.rho`.
const TestFileSuffix = "_test.rho"

// LoadTests reads the cases a Rho source file declares.
func LoadTests(path string) ([]Case, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cases []Case
	scanner := bufio.NewScanner(file)
	line := 1
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(text, TestMarker) {
			name := strings.TrimPrefix(text, TestMarker)
			// A declaration carries its parameters, and the report wants the name
			// rather than the whole signature: the file and the line are already
			// there for anything the parameters would have said.
			if index := strings.IndexAny(name, "( "); index >= 0 {
				name = name[:index]
			}
			cases = append(cases, Case{Name: strings.TrimSpace(name), File: path, Line: line})
		}
		line++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return cases, nil
}

// StripTestMarker turns a test file into the ordinary program the runner builds.
//
// The marker is removed rather than taught to the compiler, because it says
// something about a file's role and not about the language in it: the program
// underneath is a `fn` like any other, and compiling it unchanged would be
// compiling a program whose declarations were renamed to fit a tool.
func StripTestMarker(source string) string {
	return strings.ReplaceAll(source, TestMarker, "fn ")
}

// IsRhoTestFile reports whether a path is a file this runner reads itself.
func IsRhoTestFile(path string) bool {
	return strings.HasSuffix(path, TestFileSuffix)
}
