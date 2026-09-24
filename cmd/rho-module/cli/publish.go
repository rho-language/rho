//
// @file publish.go
// @brief `rho module publish` - Enterprise v3.
//
// Publishes a Rho module or plugin to a remote registry.
// Steps:
//   1. Validate module structure
//   2. Read module metadata
//   3. Pack module into `.rho-module` archive
//   4. Upload archive to remote registry
//
// Usage:
//   rho module publish
//   rho module publish <dir>
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
/*  Publish Command                                                            */
/* -------------------------------------------------------------------------- */

func Publish(args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	if err := core.PublishModule(dir); err != nil {
		return fmt.Errorf("rho module publish: %w", err)
	}

	fmt.Println("Module published:", dir)
	return nil
}
