//
// @file build.go
// @brief `rho module build` - Enterprise v3.
//
// Builds the Rho module binary from the current directory.
// Does NOT install - only builds the binary.
//
// Usage:
//   rho module build
//   rho module build <dir>
//   rho module build <dir> -o <output-dir>
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

	"rho/cmd/rho-module/core"
)

/* -------------------------------------------------------------------------- */
/*  Build Command                                                             */
/* -------------------------------------------------------------------------- */

func Build(args []string) error {
	// Manual parsing to allow -o flag anywhere
	outputDir := ""
	var positionalArgs []string
	
	i := 0
	for i < len(args) {
		if args[i] == "-o" {
			if i+1 < len(args) {
				outputDir = args[i+1]
				i += 2
			} else {
				return fmt.Errorf("-o requires an argument")
			}
		} else {
			positionalArgs = append(positionalArgs, args[i])
			i++
		}
	}

	dir := "."
	if len(positionalArgs) == 1 {
		dir = positionalArgs[0]
	} else if len(positionalArgs) > 1 {
		fmt.Fprintln(os.Stderr, "Usage: rho module build [dir] [-o output-dir]")
		return fmt.Errorf("invalid arguments")
	}

	binaryPath, err := core.BuildModule(dir, outputDir)
	if err != nil {
		return fmt.Errorf("rho module build: %w", err)
	}

	fmt.Printf("Built binary: %s\n", binaryPath)
	return nil
}