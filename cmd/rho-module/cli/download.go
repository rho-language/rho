//
// @file download.go
// @brief `rho module download` - Enterprise v3.
//
// Downloads a Rho module or plugin from a remote registry.
// Steps:
//   1. Resolve module name + version
//   2. Download archive from registry
//   3. Store archive in global cache
//   4. Validate archive
//
// Usage:
//   rho module download <name>[@version]
//
// Examples:
//   rho module download math
//   rho module download math@1.2.0
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cli

import (
	"fmt"

	"rho/cmd/rho-module/core"
)

/* -------------------------------------------------------------------------- */
/*  Download Command                                                           */
/* -------------------------------------------------------------------------- */

func Download(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: rho module download <name>[@version]")
	}

	spec := args[0]

	if err := core.DownloadModule(spec); err != nil {
		return fmt.Errorf("rho module download: %w", err)
	}

	fmt.Println("Module downloaded:", spec)
	return nil
}
