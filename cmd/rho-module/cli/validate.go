//
// @file validate.go
// @brief `rho module validate` - Enterprise v3.
//
// Validates a Rho module or plugin by checking:
//   - module.yml structure
//   - required fields
//   - main file existence
//   - plugin hooks (if type=plugin)
//   - semantic version correctness
//   - directory structure
//
// Usage:
//   rho module validate
//   rho module validate <dir>
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
/*  Validate Command                                                           */
/* -------------------------------------------------------------------------- */

func Validate(args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	if err := core.ValidateModule(dir); err != nil {
		return fmt.Errorf("rho module validate: %w", err)
	}

	fmt.Println("Module is valid:", dir)
	return nil
}
