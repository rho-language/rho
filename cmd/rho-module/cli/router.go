//
// @file router.go
// @brief Module CLI router - Enterprise v3.
//
// Routes `rho module <command>` calls from the main rho CLI to individual
// command implementations. Returns int exit codes for integration.
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
	"os"
)

/* -------------------------------------------------------------------------- */
/*  Main Entry Point                                                           */
/* -------------------------------------------------------------------------- */

func Main(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: rho module <command> [args]")
		fmt.Fprintln(os.Stderr, Commands())
		return 2
	}

	// Help is answered before anything else, because a command that does not
	// understand `--help` treats it as an argument and fails with a message
	// about a directory that does not exist.
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(os.Stderr, Usage())
		return 0
	}

	cmd := args[0]

	var err error
	switch cmd {
	case "init":
		err = Init(args[1:])
	case "validate":
		err = Validate(args[1:])
	case "info":
		err = Info(args[1:])
	case "publish":
		err = Publish(args[1:])
	case "unpublish":
		err = Unpublish(args[1:])
	case "uninstall":
		err = Uninstall(args[1:])
	case "activate":
		err = Activate(args[1:])
	case "deactivate":
		err = Deactivate(args[1:])
	case "visibility":
		err = visibilityCommand(args[1:])
	case "mine":
		err = Mine(args[1:])
	case "download":
		err = Download(args[1:])
	case "install":
		err = Install(args[1:])
	case "build":
		err = Build(args[1:])
	case "remove":
		err = Remove(args[1:])
	case "update":
		err = Update(args[1:])
	case "search":
		err = Search(args[1:])
	case "list":
		err = List(args[1:])
	case "enable":
		err = Enable(args[1:])
	case "disable":
		err = Disable(args[1:])
	case "plugin":
		err = PluginCommand(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown module command:", cmd)
		fmt.Fprintln(os.Stderr, Commands())
		return 2
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// Usage is what `rho module` and `rho module --help` print.
//
// The two registries are named apart in it, because a user reading this has to
// be able to tell which command acts on which: `install` and `uninstall` are
// about the local registry, `publish` and `unpublish` about a remote one, and
// publishing also installs locally because a module that is published from a
// machine is on that machine.
func Usage() string {
	return `rho module <command> [args]

The local registry is this installation (~/.rho). The remote registry is the
one in ~/.rho/config/registry.yaml, which you point at a registry and log into
with 'rho auth login'.

In the local registry:
  install <name>[@version]   fetch a module from the remote registry and install it
  uninstall <name>[@version] remove an installed module; --all removes every version
  activate <name>@<version>  select the version a project resolves against
  deactivate <name>@<version> clear the selection
  list [query]               what is installed here
  search [query]             what is installed here, and what the remote has

In the remote registry:
  publish <dir>              pack a module, install it here, and upload it
  unpublish <name>@<version> withdraw a version; only the module's owner may
  visibility <name> [public|private]
                             show or change whether anybody may install it
  mine [query]               the modules you published, and their state

A project's own dependencies:
  install [dir]              resolve and install a project's dependencies
  update [dir]               update them
  remove <name>              drop one from a project

About a module on disk:
  init <name> <type>         create a module
  validate [dir]             check a manifest
  info [dir]                 read a manifest
  build [dir]                build a component's command
  download <name>[@version]  fetch a package into the cache without installing

Commands: init, validate, info, build, publish, unpublish, install, uninstall,
          activate, deactivate, visibility, mine, download, remove, update,
          search, list`
}

// Commands is the one-line list the usage ends with, for a message that has to
// be short.
func Commands() string {
	return "Commands: init, validate, info, build, publish, unpublish, install, " +
		"uninstall, activate, deactivate, visibility, mine, download, remove, " +
		"update, search, list"
}
