//
// @file visibility.go
// @brief `rho module visibility` - whether a published module is anybody's.
//
// A module is public or private, and this changes which. It is a statement about
// the module's name rather than about a version, so it is made once and holds for
// every version the module gains afterwards — a module that had to be made
// private again on each release would be a module that leaked on the release
// somebody forgot.
//
// Only the owner may change it, and the registry is what decides that: this
// command presents the credential `rho auth login` stored and reports what the
// registry answers. A private module is not in a search for anybody but its
// owner, so making one public is also how its owner puts it back in front of
// people.
//
// Usage:
//   rho module visibility <name>            what it is now
//   rho module visibility <name> public     make it installable by anybody
//   rho module visibility <name> private    make it its owner's alone
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

	"rho/cmd/rho-module/remote"
)

/* -------------------------------------------------------------------------- */
/*  Visibility Command                                                         */
/* -------------------------------------------------------------------------- */

// visibilityCommand is spelled out rather than named `Visibility` so it cannot
// be confused with the field of the same name on a module's metadata, which this
// command is about.
func visibilityCommand(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("usage: rho module visibility <name> [public|private]")
	}
	name := args[0]
	if name == "" {
		return fmt.Errorf("usage: rho module visibility <name> [public|private]")
	}

	cfg, err := remote.LoadConfig()
	if err != nil {
		return fmt.Errorf("rho module visibility: %w", err)
	}
	if cfg.Remote.URL == "" {
		return fmt.Errorf("rho module visibility: no remote registry is configured")
	}
	client := remote.NewClient(cfg)
	if !client.HasCredential() {
		return fmt.Errorf(
			"rho module visibility: %s has no credential; log in with 'rho auth login %s' first",
			cfg.Remote.URL, cfg.Remote.URL)
	}

	// With no visibility named this reports rather than changes, which is the
	// question a user asks before deciding: what is it now.
	if len(args) == 1 {
		return reportVisibility(client, name)
	}

	visibility := args[1]
	if visibility != "public" && visibility != "private" {
		return fmt.Errorf("visibility must be public or private, not %q", visibility)
	}
	if err := remote.SetVisibility(client, name, visibility); err != nil {
		return fmt.Errorf("rho module visibility: %w", err)
	}
	if visibility == "private" {
		fmt.Printf("%s is now private: only you can install it, and it is not in a search for anybody else\n", name)
		return nil
	}
	fmt.Printf("%s is now public: anybody can install it\n", name)
	return nil
}

// reportVisibility prints what a module's visibility is, asked of the registry
// rather than of anything cached, because visibility is the registry's to decide
// and a local copy of the answer is a copy that can be stale.
func reportVisibility(client *remote.Client, name string) error {
	// Asking for your own modules and looking for the name is how the answer is
	// obtained: a private module does not appear in a search for anybody else,
	// but it does appear in your own.
	modules, err := remote.SearchMine(client, name)
	if err != nil {
		return fmt.Errorf("rho module visibility: %w", err)
	}
	for _, module := range modules {
		if module.Name != name {
			continue
		}
		visibility := module.Visibility
		if visibility == "" {
			visibility = "public"
		}
		fmt.Printf("%s is %s\n", name, visibility)
		return nil
	}
	return fmt.Errorf("%s is not published by you on %s", name, client.BaseURL)
}
