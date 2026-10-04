//
// @file remove.go
// @brief `rho module remove` - Enterprise v3.
//
// Removes a dependency from the local project cache (.rho-deps/) and
// updates the rho.lock file accordingly.
//
// Usage:
//   rho module remove <name>
//   rho module remove <name>@<version>
//
// Examples:
//   rho module remove math
//   rho module remove math@1.2.0
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
/*  Remove Command                                                             */
/* -------------------------------------------------------------------------- */

func Remove(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: rho module remove <name>[@version]")
	}

	spec := args[0]

	if err := core.RemoveDependency(".", spec); err != nil {
		return fmt.Errorf("rho module remove: %w", err)
	}

	fmt.Println("Dependency removed:", spec)
	return nil
}
