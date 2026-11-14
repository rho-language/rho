//
// @file pack.go
// @brief `rho pkg pack` - Enterprise v3.
//
// CLI wrapper for the Rho packager. Creates `.rho-pkg` archives
// from a Rho project directory.
//
// Usage:
//   rho pkg pack
//   rho pkg pack <dir>
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"fmt"
	"os"

	"rho/cmd/rho-pkg/core"
)

/* -------------------------------------------------------------------------- */
/*  Pack Command                                                               */
/* -------------------------------------------------------------------------- */

func Pack(args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	pkgPath, err := core.PackProject(dir)
	if err != nil {
		return fmt.Errorf("rho pkg pack: %w", err)
	}

	fmt.Println("Created package:", pkgPath)
	return nil
}

func Main(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: rho pkg <command> [args]")
		fmt.Fprintln(os.Stderr, "Commands: init, pack, unpack, inspect")
		return 2
	}

	cmd := args[0]
	// A flag is never a name: `rho pkg init --help` asks for help, and must not
	// make a directory called --help.
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			fmt.Println("usage: rho pkg <command> [args]")
			fmt.Println("  init <name>          start a package directory")
			fmt.Println("  pack [dir]           pack a module into a .rho-pkg")
			fmt.Println("  unpack <file> [dir]  unpack a package")
			fmt.Println("  inspect <file>       show what a package holds")
			return 0
		}
	}

	var err error
	switch cmd {
	case "init":
		err = Init(args[1:])
	case "pack":
		err = Pack(args[1:])
	case "unpack":
		err = Unpack(args[1:])
	case "inspect":
		err = Inspect(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown pkg command:", cmd)
		fmt.Fprintln(os.Stderr, "Commands: init, pack, unpack, inspect")
		return 2
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
