//
// @file uninstall.go
// @brief `rho module uninstall` - removing a module from the local registry.
//
// This is the local half of the module lifecycle: it removes what is installed
// in `~/.rho` and touches no registry. The remote half is `unpublish`, which
// withdraws a version from a registry and is a different act with a different
// guard — it is the one that changes what other people can install.
//
// The two are separate commands because they answer different questions and need
// different authority. Removing a copy from this machine is something a user may
// always do to their own installation; withdrawing a module from a registry is
// something only the identity that published it may do. A command that did both
// would be asking for a decision every time either one was wanted.
//
// The version the toolchain resolves to is the one refusal here. A selected
// version is the one a project is currently compiled and linked against, so
// removing it is refused and the user is told to deactivate first: a build that
// stopped resolving a component is worse than a version that is still on disk.
//
// Usage:
//   rho module uninstall <name>[@version]
//   rho module uninstall <name> --all
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
	"strings"

	"rho/cmd/rho-module/core"
)

/* -------------------------------------------------------------------------- */
/*  Uninstall Command                                                          */
/* -------------------------------------------------------------------------- */

func Uninstall(args []string) error {
	all := false
	specs := []string{}
	for _, arg := range args {
		if arg == "--all" || arg == "-a" {
			all = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("usage: rho module uninstall <name>[@version] [--all]")
		}
		specs = append(specs, arg)
	}
	if len(specs) == 0 {
		return fmt.Errorf("usage: rho module uninstall <name>[@version] [--all]")
	}

	// `--all` takes every version of a name rather than the one named, which is
	// what a user means by removing a module outright.
	if all && len(specs) != 1 {
		return fmt.Errorf("--all takes one module name")
	}

	removed := 0
	for _, spec := range specs {
		name, version := parseSpec(spec)
		if name == "" {
			return fmt.Errorf("usage: rho module uninstall <name>[@version] [--all]")
		}
		if version == "" {
			// No version named, and not `--all`: the newest installed, which
			// is the one a user uninstalling a module means.
			version = core.NewestInstalled(name)
			if version == "" {
				return fmt.Errorf("%s is not installed here", name)
			}
		}
		if err := core.UninstallLocal(name, version); err != nil {
			return fmt.Errorf("rho module uninstall: %w", err)
		}
		fmt.Printf("Uninstalled %s@%s from the local registry\n", name, version)
		removed++
	}
	if removed == 0 {
		return fmt.Errorf("nothing was removed")
	}
	return nil
}
