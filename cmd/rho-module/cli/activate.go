//
// @file activate.go
// @brief `rho module activate` - Enterprise v3.
//
// Sets a specific module version as the active one.
// This determines which version is used when running `rho <command>`.
//
// Usage:
//   rho module activate <name>[@version]
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
/*  Activate Command                                                           */
/* -------------------------------------------------------------------------- */

func Activate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: rho module activate <name>[@version]")
	}

	spec := args[0]
	name, version := parseSpec(spec)

	if version == "" {
		return fmt.Errorf("version required for activate")
	}

	if err := core.ActivateModule(name, version); err != nil {
		return fmt.Errorf("rho module activate: %w", err)
	}
	return nil
}
