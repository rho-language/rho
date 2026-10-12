//
// @file cli.go
// @brief What this toolchain can be asked to do, on this machine.
//
// There are two kinds of command, and the difference is not a matter of taste.
//
// A **core** command is the toolchain's own: it is inside the binary, it works
// on a machine that has never run `rho install`, and it is the same list
// everywhere. A **component** command is published by a module: it exists on a
// machine that has the module installed and has turned it on, and it is a
// different command on a different machine.
//
// A list that mixed them, or one written down and not read, is how `rho help`
// came to offer commands that did nothing — a command for a component nobody
// installed, offered by a toolchain that cannot run it. So the list is read, and
// both the toolchain and the dashboard that describes it read it from here, so
// that there is one list rather than two that agree today.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"path/filepath"
	"sort"

	"rho/src/deps"
)

// Core is a command the toolchain has for itself.
//
// It is a function of the binary rather than of the installation, which is what
// lets a machine with nothing installed still be told what it can do.
func Core() []Command {
	return []Command{
		{Invocation: "run <file.rho>", Description: "Run a Rho program"},
		{Invocation: "build <file.rho> [-o]", Description: "Compile Rho to C, and build it"},
		{Invocation: "init [<generator>] <directory>", Description: "Start a Rho application, or a project a module knows how to generate"},
		{Invocation: "install", Description: "Install Rho programming language"},
		{Invocation: "pkg <command>", Description: "Package management (init, pack, unpack, inspect)"},
		{Invocation: "module <command>", Description: "Modules in the local registry"},
		{Invocation: "plugin <command>", Description: "Plugins: list, enable, disable"},
		{Invocation: "config <command>", Description: "Read and set configuration"},
		{Invocation: "ide <editor> [dir]", Description: "Write what an editor reads: launch.json, tasks.json"},
		{Invocation: "version", Description: "Print the Rho version"},
		{Invocation: "help", Description: "Show this help"},
	}
}

// Command is one command, as a list of them shows it.
type Command struct {
	// Invocation is what a person types after `rho`.
	Invocation string
	// Description is what it is for, as the module states it.
	Description string
	// Source is "core" for a command in the toolchain, or the component's name
	// for one a component publishes.
	Source string
}

// Components are the component commands this machine can actually run.
//
// A component is here only if it is installed and only if it has been enabled,
// because those are two separate decisions: a module can be on the machine
// because a dependency wanted it, and using it is something a person has to
// have said yes to.
func Components() []Command {
	home := deps.RhoHome()
	if home == "" {
		// Nothing is installed, so nothing is available. A toolchain that has
		// never been installed still has its own commands, and saying so is what
		// this returns rather than a list of components that are not there.
		return nil
	}
	active, err := deps.LoadEnabled()
	if err != nil {
		// A record that cannot be read means nothing can be said about what is
		// enabled, so nothing is claimed. A list of commands that might not run
		// is the thing this whole file exists to prevent.
		return nil
	}
	found := []Command{}
	seen := map[string]bool{}
	for _, reserved := range Core() {
		seen[head(reserved.Invocation)] = true
	}
	for _, installed := range deps.ListInstalled(deps.KindComponent) {
		if !active.IsEnabled(deps.KindComponent, installed.Name, installed.Version) {
			continue
		}
		manifest, err := deps.LoadManifest(filepath.Join(installed.Root, "module.yml"))
		if err != nil || manifest == nil || manifest.Command == "" {
			continue
		}
		// A component that publishes a name the toolchain has is a collision, and
		// the toolchain's is the one that runs — so it is not offered again as
		// though there were a choice.
		if seen[manifest.Command] {
			continue
		}
		seen[manifest.Command] = true
		found = append(found, Command{
			Invocation:  manifest.Command,
			Description: manifest.Description,
			Source:      installed.Name,
		})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Invocation < found[j].Invocation })
	return found
}

// All is everything this machine can be asked to do: what the toolchain has, and
// what is installed and turned on.
func All() ([]Command, []Command) { return Core(), Components() }

// head is the first word of an invocation, which is the command's name.
func head(invocation string) string {
	for i, r := range invocation {
		if r == ' ' {
			return invocation[:i]
		}
	}
	return invocation
}

// IsInstalled reports whether there is a Rho installation to read commands from.
func IsInstalled() bool { return deps.RhoHome() != "" }
