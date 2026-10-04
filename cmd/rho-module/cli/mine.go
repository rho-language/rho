//
// @file mine.go
// @brief `rho module mine` - the modules you published, and their state.
//
// A private module is deliberately absent from what everybody else can see, so
// this is the only way its owner finds it. It is also how an owner checks what
// state a module is in without changing it: a module that was published private
// and is meant to be public is a mistake that shows up here and nowhere else.
//
// The question is answered by the registry rather than by anything kept locally,
// because visibility is the registry's to decide. A cached copy of the answer
// would be a copy that can be stale, and the one thing a user is asking this
// command is what the registry currently thinks.
//
// Usage:
//   rho module mine [query]
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

	"rho/cmd/rho-module/remote"
)

/* -------------------------------------------------------------------------- */
/*  Mine Command                                                               */
/* -------------------------------------------------------------------------- */

func Mine(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: rho module mine [query]")
	}
	query := ""
	if len(args) == 1 {
		query = args[0]
	}

	cfg, err := remote.LoadConfig()
	if err != nil {
		return fmt.Errorf("rho module mine: %w", err)
	}
	if cfg.Remote.URL == "" {
		return fmt.Errorf("rho module mine: no remote registry is configured")
	}
	client := remote.NewClient(cfg)
	if !client.HasCredential() {
		return fmt.Errorf(
			"rho module mine: %s has no credential; log in with 'rho auth login %s' first",
			cfg.Remote.URL, cfg.Remote.URL)
	}

	modules, err := remote.SearchMine(client, query)
	if err != nil {
		return fmt.Errorf("rho module mine: %w", err)
	}
	if len(modules) == 0 {
		if query == "" {
			fmt.Printf("You have published nothing to %s\n", cfg.Remote.URL)
		} else {
			fmt.Printf("You have published nothing matching %q to %s\n", query, cfg.Remote.URL)
		}
		return nil
	}

	fmt.Printf("Published by you on %s:\n\n", cfg.Remote.URL)
	for _, module := range modules {
		visibility := module.Visibility
		if visibility == "" {
			visibility = "public"
		}
		fmt.Printf("  %s@%s  %-10s %s\n", module.Name, module.Version, module.Type, visibility)
	}
	// A private module is worth a word, because a user who cannot find their own
	// module in a search is a user who needs to know why.
	for _, module := range modules {
		if module.Visibility == "private" {
			fmt.Printf("\n%s is private: only you can install it, and it is not in a search for anybody else.\n",
				module.Name)
			fmt.Printf("Make it public with: rho module visibility %s public\n", module.Name)
			break
		}
	}
	return nil
}
