//
// @file main.go
// @brief Rho CLI - Enterprise v3.
//
// Top-level CLI for the Rho compiler toolchain.
// Commands:
//   rho run <file.rho>
//   rho build <file.rho> [-o out.c]
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	src "rho/src" // orchestrator v3
)

/* -------------------------------------------------------------------------- */
/*  Entry point                                                               */
/* -------------------------------------------------------------------------- */

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: rho <command> [args]")
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {

	case "run":
		runCLI(os.Args[2:])

	case "build":
		buildCLI(os.Args[2:])

	case "help":
		printHelp()

	default:
		fmt.Println("Unknown command:", cmd)
		fmt.Println("Use: rho help")
		os.Exit(1)
	}
}

/* -------------------------------------------------------------------------- */
/*  rho run                                                                   */
/* -------------------------------------------------------------------------- */

func runCLI(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: rho run <file.rho>")
		os.Exit(2)
	}

	file := args[0]
	abs, _ := filepath.Abs(file)

	if err := RunFile(abs); err != nil {
		fmt.Fprintln(os.Stderr, "rho run:", err)
		os.Exit(1)
	}
}

/* -------------------------------------------------------------------------- */
/*  rho build                                                                 */
/* -------------------------------------------------------------------------- */

func buildCLI(args []string) {
	fs := flag.NewFlagSet("rho build", flag.ExitOnError)
	out := fs.String("o", "", "output C file (optional)")
	fs.Parse(args)

	if fs.NArg() != 1 {
		fmt.Println("Usage: rho build <file.rho> [-o out.c]")
		os.Exit(2)
	}

	in := fs.Arg(0)
	absIn, _ := filepath.Abs(in)

	if *out != "" {
		absOut, _ := filepath.Abs(*out)
		if err := src.CompileFileTo(absIn, absOut); err != nil {
			fmt.Fprintln(os.Stderr, "rho build:", err)
			os.Exit(1)
		}
		return
	}

	if err := src.CompileFileTo(absIn, src.ProjectOutputPath(absIn)); err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		os.Exit(1)
	}
}

/* -------------------------------------------------------------------------- */
/*  Help                                                                      */
/* -------------------------------------------------------------------------- */

func printHelp() {
	fmt.Println("Rho Programming Language CLI")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  rho run <file.rho>          Run a Rho program")
	fmt.Println("  rho build <file.rho> [-o]   Compile Rho → C")
	fmt.Println("  rho help                    Show this help")
}
