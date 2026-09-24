//
// @file install.go
// @brief `rho module install` - Enterprise v3.
//
// Installs all dependencies declared in module.yml by:
//   1. Reading module.yml
//   2. Resolving dependency versions
//   3. Downloading missing modules
//   4. Installing modules into local project cache (.rho-deps/)
//   5. Generating rho.lock
//
// Usage:
//   rho module install
//   rho module install <dir>
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
/*  Install Command                                                            */
/* -------------------------------------------------------------------------- */

func Install(args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	if err := core.InstallDependencies(dir); err != nil {
		return fmt.Errorf("rho module install: %w", err)
	}

	fmt.Println("Dependencies installed:", dir)
	return nil
}
