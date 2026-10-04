//
// @file update.go
// @brief `rho module update` - Enterprise v3.
//
// Updates all dependencies declared in module.yml by:
//   1. Reading module.yml
//   2. Resolving latest compatible versions
//   3. Comparing with rho.lock
//   4. Downloading updated modules
//   5. Installing updated modules into .rho-deps/
//   6. Regenerating rho.lock
//
// Usage:
//   rho module update
//   rho module update <dir>
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

	"rho/cmd/rho-module/core"
)

/* -------------------------------------------------------------------------- */
/*  Update Command                                                             */
/* -------------------------------------------------------------------------- */

func Update(args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	if err := core.UpdateDependencies(dir); err != nil {
		return fmt.Errorf("rho module update: %w", err)
	}

	fmt.Println("Dependencies updated:", dir)
	return nil
}
