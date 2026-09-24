//
// @file unpublish.go
// @brief `rho module unpublish` - Enterprise v3.
//
// Unpublishes a module version from the local registry.
// Fails if trying to unpublish the only active version.
//
// Usage:
//   rho module unpublish <name>[@version]
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
/*  Unpublish Command                                                          */
/* -------------------------------------------------------------------------- */

func Unpublish(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: rho module unpublish <name>[@version]")
	}

	spec := args[0]
	name, version := parseSpec(spec)

	if version == "" {
		return fmt.Errorf("version required for unpublish")
	}

	if err := core.UnpublishModule(name, version); err != nil {
		return fmt.Errorf("rho module unpublish: %w", err)
	}
	return nil
}