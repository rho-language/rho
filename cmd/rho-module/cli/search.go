//
// @file search.go
// @brief `rho module search` - Enterprise v3.
//
// Searches for modules in the local registry by keyword.
// Displays:
//   - module name
//   - version
//   - type (component/plugin)
//   - active status
//
// Usage:
//   rho module search <query>
//   rho module search            (lists all)
//
// Examples:
//   rho module search math
//   rho module search
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
	if len(args) > 1 {
		return fmt.Errorf("usage: rho module search [query]")
	}

	query := ""
	if len(args) == 1 {
		query = args[0]
	}

	results, err := core.SearchRegistry(query)
	if err != nil {
		return fmt.Errorf("rho module search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No modules found for:", query)
		return nil
	}

	if query == "" {
		fmt.Println("Installed modules:")
	} else {
		fmt.Println("Results for:", query)
	}
	for _, r := range results {
		activeMark := ""
		if r.Active {
			activeMark = " [ACTIVE]"
		}
		fmt.Printf("- %s@%s (%s)%s\n", r.Name, r.Version, r.Type, activeMark)
	}

	return nil
}
