//
// @file install.go
// @brief `rho module install` - installing from the remote registry, or a project's dependencies.
//
// It has two forms because there are two things a user means by "install":
//
//	rho module install <name>[@version]   fetch a module from the remote registry
//	                                       and install it into this local registry
//	rho module install [dir]              resolve and install the dependencies
//	                                       a project declares
//
// The first is the common one and is what pairs with `uninstall`: the local
// registry gains and loses a module. The second is what a project does before it
// builds, and it is kept because a project with dependencies has no other way to
// get them.
//
// The form is decided by what the argument looks like, not by a flag: a name with
// an @ or a leading dot-free word is a module, and a path is a project. A
// directory that exists is taken to be a project, so a module name that happens
// to match a directory in the working directory has to be named with `./` to
// mean itself — the same rule the rest of the toolchain uses for a path.
//
// Usage:
//   rho module install <name>[@version]
//   rho module install [dir]
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
	"os"
	"strings"

	"rho/cmd/rho-module/core"
	"rho/cmd/rho-module/remote"
)

/* -------------------------------------------------------------------------- */
/*  Install Command                                                            */
/* -------------------------------------------------------------------------- */

func Install(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf(
			"usage: rho module install <name>[@version]   install a module from the remote registry\n" +
				"       rho module install [dir]              install a project's dependencies")
	}

	// A name is a module; anything that names a place on disk is a project.
	if len(args) == 1 && isModuleName(args[0]) {
		return installModule(args[0])
	}
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}
	if err := core.InstallDependencies(dir); err != nil {
		return fmt.Errorf("rho module install: %w", err)
	}
	fmt.Println("Dependencies installed:", dir)
	return nil
}

// isModuleName reports whether an argument names a module rather than a project
// directory.
//
// A path is a path, and so is anything that exists on disk: a module name that
// collides with a directory in the working directory has to be spelled `./name` to
// mean the module, which is the rule the rest of the toolchain already uses.
func isModuleName(arg string) bool {
	if strings.HasPrefix(arg, ".") || strings.ContainsAny(arg, "/\\") {
		return false
	}
	if _, err := os.Stat(arg); err == nil {
		return false
	}
	return true
}

// installModule fetches a module from the remote registry and installs it.
func installModule(spec string) error {
	name, version := parseSpec(spec)
	if name == "" {
		return fmt.Errorf("usage: rho module install <name>[@version]")
	}

	cfg, err := remote.LoadConfig()
	if err != nil {
		return fmt.Errorf("rho module install: %w", err)
	}
	if cfg.Remote.URL == "" {
		return fmt.Errorf(
			"rho module install: no remote registry is configured; " +
				"set one in ~/.rho/config/registry.yaml")
	}
	client := remote.NewClient(cfg)

	resolved, err := remote.ResolveRemoteVersion(client, name, version)
	if err != nil {
		return fmt.Errorf("rho module install: %w", err)
	}
	installed, err := core.InstallFromRemote(client, name, resolved)
	if err != nil {
		return fmt.Errorf("rho module install: %w", err)
	}
	fmt.Printf("Installed %s@%s from %s (%s)\n", name, resolved, cfg.Remote.URL, installed.Kind)
	return nil
}
