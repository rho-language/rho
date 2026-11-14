//
// @file new.go
// @brief `rho new` — start a Rho source file with the header its module knows.
//
// @project Rho Programming Language
// @author Aurel Miron
// @date 2026-11-12
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"flag"
	"fmt"
	"os"

	"rho/src/scaffold"
)

// newCLI writes a new .rho file: its header (file, description, module name,
// project, author, date, licence) from the module's manifest, the module line,
// and for a *_test.rho file the shape of a test suite.
func newCLI(args []string) int {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	description := fs.String("description", "", "one line about what the file is for")
	author := fs.String("author", "", "the author, instead of the module's maintainer")
	date := fs.String("date", "", "the date, as YYYY-MM-DD, instead of today")
	force := fs.Bool("force", false, "replace a file that is already there")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: rho new [--description TEXT] [--author NAME] [--date YYYY-MM-DD] [--force] <file.rho>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "The module name, the author and the licence come from the nearest module.yml;")
		fmt.Fprintln(os.Stderr, "a file named *_test.rho starts as a test suite and is listed in tests/tests.yml.")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	result, err := scaffold.NewSource(scaffold.SourceRequest{
		Path:        fs.Arg(0),
		Description: *description,
		Author:      *author,
		Date:        *date,
		Force:       *force,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho new:", err)
		return 1
	}
	fmt.Printf("wrote %s (module %s)\n", result.Path, result.Module)
	if result.Suite != "" {
		fmt.Printf("listed %s in tests/tests.yml\n", result.Suite)
	}
	return 0
}
