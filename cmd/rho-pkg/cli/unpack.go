//
// @file unpack.go
// @brief `rho pkg unpack` - Enterprise v3.
//
// CLI wrapper for unpacking `.rho-pkg` archives.
//
// Usage:
//   rho pkg unpack <file.rho-pkg>
//   rho pkg unpack <file.rho-pkg> <dest>
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
/*  Unpack Command                                                             */
/* -------------------------------------------------------------------------- */

func Unpack(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rho pkg unpack <file.rho-pkg> [dest]")
	}

	src := args[0]
	dest := "."
	if len(args) == 2 {
		dest = args[1]
	}

	if err := core.UnpackArchive(src, dest); err != nil {
		return fmt.Errorf("rho pkg unpack: %w", err)
	}

	fmt.Println("Unpacked:", src, "to", dest)
	return nil
}
