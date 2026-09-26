//
// @file main.go
// @brief Standalone Rho test runner.
//
// The implementation lives in src/rhotest, which `rho test` calls directly.
// This binary is the same runner for use on its own, or in a shell that
// should not go through the rho CLI.
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

	"rho/src/rhotest"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// A test is an ordinary program, so it runs on one backend. The default is
	// the native one; a Go project is tested with --target go.
	options := rhotest.Options{Target: rhotest.DefaultTarget}
	rest := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-target", "--target":
			if i+1 >= len(args) {
				fmt.Println("Usage: rho-test <path> [--target c|go]")
				return 2
			}
			options.Target = args[i+1]
			i++
		case "-rho", "--rho":
			if i+1 >= len(args) {
				fmt.Println("Usage: rho-test <path> [--rho <path-to-rho>]")
				return 2
			}
			options.RhoBinary = args[i+1]
			i++
		default:
			rest = append(rest, args[i])
		}
	}
	if len(rest) < 1 {
		fmt.Println("Usage: rho-test <path> [--target c|go] [--rho <path-to-rho>]")
		return 2
	}
	options.Path = rest[0]
	return rhotest.Run(options)
}
