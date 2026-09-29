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

// List reports what is installed in the local registry, and nothing else.
//
// It is separated from `search` because the two answer different questions and
// need different conditions: this one is a statement about this machine and
// always works, and `search` reaches a registry and so may not.
func List(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: rho module list [query]")
	}
	query := ""
	if len(args) == 1 {
		query = args[0]
	}
	return reportLocal(query)
}

func Search(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: rho module search [query]")
	}

	query := ""
	if len(args) == 1 {
		query = args[0]
	}

	if err := reportLocal(query); err != nil {
		return fmt.Errorf("rho module search: %w", err)
	}

	// The remote is asked only when a credential is available, because a search
	// that cannot authenticate is a search a registry may refuse, and a refusal
	// here would be indistinguishable from an empty registry.
	cfg, err := remote.LoadConfig()
	if err != nil {
		return nil
	}
	client := remote.NewClient(cfg)
	if cfg.Remote.URL == "" || !client.HasCredential() {
		return nil
	}
	remoteResults, err := remote.SearchRemote(client, query)
	if err != nil || len(remoteResults) == 0 {
		return nil
	}
	// The remote is reported separately and never merged with what is
	// installed: a module listed here is one this machine does not have, and a
	// reader who cannot tell the two cannot trust either.
	fmt.Println("\nAvailable in the remote registry (not installed here):")
	for _, r := range remoteResults {
		fmt.Printf("- %s@%s  %s\n", r.Name, r.Version, r.Type)
	}
	return nil
}

// reportLocal prints what is installed here.
func reportLocal(query string) error {
	results, err := core.SearchRegistry(query)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		if query == "" {
			fmt.Println("Nothing is installed in this local registry.")
			fmt.Println("Publish a module, or install one from the remote registry.")
		} else {
			fmt.Printf("Nothing installed here matches %q.\n", query)
		}
		return nil
	}
	if query == "" {
		fmt.Println("Installed in this local registry:")
	} else {
		fmt.Printf("Installed here, matching %q:\n", query)
	}
	// Grouped by kind, because a component, a plugin and a module are three
	// different things a user is thinking about, and a list that interleaves them
	// reads as one.
	for _, kind := range core.IndexKinds {
		shown := false
		for _, r := range results {
			if r.Type != kind {
				continue
			}
			if !shown {
				fmt.Printf("\n  %s:\n", kind)
				shown = true
			}
			// Two different things are worth knowing and they are not the same
			// one: which version the registry index resolves a name to, and
			// whether the module is enabled and used at all. A module can be
			// selected by the index and still not be the one a run uses.
			mark := ""
			if r.Active {
				mark = "  (selected)"
			}
			if kind == "component" || kind == "plugin" {
				state, err := enabledStateFor(kind, r.Name, r.Version)
				if err == nil && state != "" {
					mark += "  (" + state + ")"
				}
			}
			origin := ""
			if r.Origin == "remote" {
				origin = "  (from the remote registry)"
			}
			fmt.Printf("    %s@%s%s%s\n", r.Name, r.Version, mark, origin)
		}
	}
	return nil
}
