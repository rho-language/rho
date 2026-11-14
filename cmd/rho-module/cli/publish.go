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
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
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
	localOnly := false
	for _, arg := range args {
		switch arg {
		case "--local", "-local":
			// Installed here and nowhere else: a release tried on this machine
			// before it goes to the remote registry.
			localOnly = true
		default:
			dir = arg
		}
	}

	if err := core.PublishModuleTo(dir, localOnly); err != nil {
		return fmt.Errorf("rho module publish: %w", err)
	}

	if localOnly {
		fmt.Println("Module installed in the local registry only:", dir)
		return nil
	}
	fmt.Println("Module published:", dir)
	return nil
}
