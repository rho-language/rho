//
// @file inspect.go
// @brief `rho pkg inspect` - Enterprise v3.
//
// CLI wrapper for inspecting `.rho-pkg` archives.
//
// Usage:
//   rho pkg inspect <file.rho-pkg>
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

	"rho/cmd/rho-pkg/core"
)

/* -------------------------------------------------------------------------- */
/*  Inspect Command                                                            */
/* -------------------------------------------------------------------------- */

func Inspect(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: rho pkg inspect <file.rho-pkg>")
	}

	src := args[0]

	meta, err := core.ReadPackageMetadata(src)
	if err != nil {
		return fmt.Errorf("rho pkg inspect: %w", err)
	}

	fmt.Println("Package:", meta.Module.Name)
	fmt.Println("Type:   ", meta.Module.Type)
	fmt.Println("Version:", meta.Module.Version)
	fmt.Println("Main:   ", meta.Main)

	if len(meta.Features) > 0 {
		fmt.Println("Features:")
		for _, f := range meta.Features {
			fmt.Println("  -", f)
		}
	}

	return nil
}
