//
// @file search.go
// @brief `rho module search` - Enterprise v3.
//
// Searches for modules in the local and remote registry by keyword.
// Displays:
//   - module name
//   - version
//   - type (component/plugin)
//   - active status (local modules)
//
// Usage:
//   rho module search <query>
//   rho module search            (lists all local modules)
//
// If a remote registry is configured (via ~/.rho/config/registry.yaml),
// remote results are also displayed with [REMOTE] marker.
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
	"rho/cmd/rho-module/remote"
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

	// Local search
	results, err := core.SearchRegistry(query)
	if err != nil {
		return fmt.Errorf("rho module search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No modules found for:", query)
	} else {
		if query == "" {
			fmt.Println("Local modules:")
		} else {
			fmt.Println("Local results for:", query)
		}
		for _, r := range results {
			activeMark := ""
			if r.Active {
				activeMark = " [ACTIVE]"
			}
			fmt.Printf("- %s@%s (%s)%s\n", r.Name, r.Version, r.Type, activeMark)
		}
	}

	// Remote search (if configured)
	cfg, err := remote.LoadConfig()
	if err == nil && cfg.Remote.Token != "" {
		client := remote.NewClient(cfg)
		remoteResults, err := remote.SearchRemote(client, query)
		if err == nil && len(remoteResults) > 0 {
			fmt.Println("\nRemote modules:")
			for _, r := range remoteResults {
				fmt.Printf("- %s@%s (%s) [REMOTE]\n", r.Name, r.Version, r.Type)
			}
		}
	}

	return nil
}
