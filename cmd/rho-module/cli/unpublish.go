//
// @file unpublish.go
// @brief `rho module unpublish` - withdrawing a version from a remote registry.
//
// This is the remote half of the module lifecycle, and the one act in it that
// changes what other people can install. A version that is withdrawn stops being
// resolvable, so every project depending on it stops building, and the guard is
// therefore a question of authority rather than of convenience: only the
// identity that published the module may withdraw it.
//
// That identity is the credential `rho auth login` stored. The registry keeps the
// owner of a module name from the credential that first published it, and a
// maintainer — the same identity for now — may add versions but not withdraw
// the module, because taking it away is the part a dependee cannot recover from.
//
// The local half is `uninstall`, which removes a copy from this machine and
// needs no authority over anyone. The two are separate commands because a user
// who wants to remove a component from their own installation should not be asked
// to prove they are the person who published it to a registry.
//
// Usage:
//   rho module unpublish <name>@<version>
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
/*  Unpublish Command                                                          */
/* -------------------------------------------------------------------------- */

func Unpublish(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: rho module unpublish <name>@<version>")
	}
	spec := args[0]
	name, version := parseSpec(spec)
	if name == "" {
		return fmt.Errorf("usage: rho module unpublish <name>@<version>")
	}
	if version == "" {
		return fmt.Errorf(
			"a version is required: unpublishing a name would withdraw every version of it, " +
				"which is almost never what is meant. Name the one to withdraw, as <name>@<version>")
	}

	cfg, err := remote.LoadConfig()
	if err != nil {
		return fmt.Errorf("rho module unpublish: %w", err)
	}
	client := remote.NewClient(cfg)
	if cfg.Remote.URL == "" {
		return fmt.Errorf("rho module unpublish: no remote registry is configured")
	}
	if !client.HasCredential() {
		// Without a credential the registry cannot tell who is asking, and a
		// registry that cannot tell cannot let anyone withdraw anything. Saying so
		// is more useful than a refusal from the far end.
		return fmt.Errorf(
			"rho module unpublish: %s has no credential; log in with 'rho auth login %s' first",
			cfg.Remote.URL, cfg.Remote.URL)
	}

	if err := remote.UnpublishModule(client, name, version); err != nil {
		return fmt.Errorf("rho module unpublish: %w", err)
	}
	fmt.Printf("Unpublished %s@%s from %s\n", name, version, cfg.Remote.URL)

	// A withdrawn version that is installed here is left installed: the user
	// asked the registry to stop offering it, not for this machine to stop using
	// it. Removing the local copy is `rho module uninstall`, and doing it here
	// would take away something the user did not ask to lose.
	if core.IsInstalledVersion(name, version) {
		fmt.Printf(
			"%s@%s is still installed here; 'rho module uninstall %s@%s' removes it\n",
			name, version, name, version)
	}
	return nil
}
