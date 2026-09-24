//
// @file search.go
// @brief `rho module search` - Enterprise v3.
//
// Searches for modules in the remote registry by keyword.
// Displays:
//   - module name
//   - latest version
//   - type (rho/module/plugin)
//   - description
//
// Usage:
//   rho module search <query>
//
// Examples:
//   rho module search math
//   rho module search io
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
/*  Search Command                                                             */
/* -------------------------------------------------------------------------- */

func Search(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: rho module search <query>")
	}

	query := args[0]

	results, err := core.SearchRegistry(query)
	if err != nil {
		return fmt.Errorf("rho module search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No modules found for:", query)
		return nil
	}

	fmt.Println("Results for:", query)
	for _, r := range results {
		fmt.Printf("- %s (%s) %s\n", r.Name, r.Version, r.Type)
		if r.Description != "" {
			fmt.Println("    ", r.Description)
		}
	}

	return nil
}
