//
// @file enable.go
// @brief Which of the installed modules is the one that runs.
//
// Installing a component or a plugin makes it available; using it is a separate
// decision. That is what these commands are for, and it is why they exist rather
// than a flag on install: a program that runs a component nobody chose is a
// program running code its author did not ask for, and the choice belongs to
// the person who installed it rather than to whoever installed it.
//
// Several versions of a module may be installed at once — a dependency brought in
// at one version, an upgrade leaving the old one behind — and exactly one of
// them is the one that runs. Choosing it is what `enable` does; choosing a
// particular version of it is what `enable name@version` does, which is how a
// machine that has to keep running against the version it was verified with
// pins itself to it.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"rho/src/deps"
	"rho/src/plugins"
)

/* -------------------------------------------------------------------------- */
/*  list                                                                       */
/* -------------------------------------------------------------------------- */

/* -------------------------------------------------------------------------- */
/*  enable / disable                                                           */
/* -------------------------------------------------------------------------- */

// Enable records that a module is the one that runs.
//
// A name with no version takes the highest version installed, which is what
// "turn it on" means. A name with a version takes that one, which is what
// pinning to a known-good release is for.
func Enable(args []string) error {
	if len(args) == 0 {
		return usageFor("enable", "rho module enable <name>[@<version>]")
	}
	kind, name, version := parseModuleRef(args[0])
	if version == "" {
		// Naming a module with no version means the newest one installed, which
		// is what turning something on means. Resolving it here rather than
		// passing an empty version is what keeps an empty version from being
		// recorded as a deliberate choice of nothing.
		installed := deps.InstalledVersions(kind, name)
		if len(installed) == 0 {
			return fmt.Errorf("%s %s is not installed", kind, name)
		}
		version = installed[0]
	}
	if err := deps.SetVersionEnabled(kind, name, version); err != nil {
		return err
	}
	fmt.Printf("enabled %s %s at %s\n", kind, name, version)
	return nil
}

// Disable records that a module is installed and not to be used.
//
// The version stays on disk. A disabled module is disabled, not removed, so
// turning it back on does not mean installing it again.
func Disable(args []string) error {
	if len(args) == 0 {
		return usageFor("disable", "rho module disable <name>")
	}
	kind, name, _ := parseModuleRef(args[0])
	if err := deps.Disable(kind, name); err != nil {
		return err
	}
	fmt.Printf("disabled %s %s; it stays installed and is not used until it is enabled again\n", kind, name)
	return nil
}

/* -------------------------------------------------------------------------- */
/*  plugins                                                                    */
/* -------------------------------------------------------------------------- */

// PluginList shows the plugins installed, what each one is, and whether it runs.
//
// The PROVIDES column is the answer to "what is this for". Hooks say how a plugin
// is called; what it provides says what it is, and two plugins may answer the
// same hook and be entirely different things.
func PluginList(args []string) error {
	home := deps.RhoHome()
	if home == "" {
		return fmt.Errorf("no Rho installation was found; 'rho install' creates one")
	}
	source := plugins.Source{RhoHome: home}
	found := deps.ListInstalled(deps.KindPlugin)
	if len(found) == 0 {
		fmt.Printf("no plugins installed under %s\n", home)
		return nil
	}
	rows := [][]string{{"NAME", "VERSION", "STATE", "EXTENDS", "PROVIDES"}}
	for _, installed := range found {
		// The root of the *enabled* version is what the plugin that runs is, so
		// a listing describes what is in use rather than what was installed
		// last. A plugin with no enabled version is described from the tree it
		// was installed into, because the reader still wants to know what they
		// have.
		root := deps.InstalledPluginRoot(installed.Name)
		if root == "" {
			root = installed.Root
		}
		manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
		if err != nil || manifest == nil || manifest.Extends == nil {
			continue
		}
		provides := manifest.Extends.Implements
		if len(provides) == 0 {
			// A plugin that says what it implements says nothing here, which is
			// worth seeing: it is offered to every run of its component.
			provides = []string{"(unspecified)"}
		}
		rows = append(rows, []string{
			installed.Name, installed.Version, stateOf(found, installed), manifest.Extends.Component,
			strings.Join(provides, ", "),
		})
	}
	if len(rows) == 1 {
		fmt.Printf("no plugins under %s extend a component\n", home)
		return nil
	}
	_ = source
	writeRows(rows)
	return nil
}

func stateOf(found []deps.InstalledModule, one deps.InstalledModule) string {
	if one.Enabled {
		return "active"
	}
	for _, other := range found {
		if other.Name == one.Name {
			return "installed"
		}
	}
	return ""
}

// PluginCommand is `rho plugin ...`: listing plugins, and turning them on and
// off. It is a command of its own rather than three, so that the plugins a
// machine has are one thing a person asks about.
func PluginCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf(
			"usage: rho plugin <list|enable|disable> [name]\n\n" +
				"  list      what is installed, what each plugin provides, and what runs\n" +
				"  enable    a plugin runs; a version may be named, as name@version\n" +
				"  disable   a plugin stays installed and is not used")
	}
	switch args[0] {
	case "list":
		return PluginList(args[1:])
	case "enable":
		return PluginEnable(args[1:])
	case "disable":
		return PluginDisable(args[1:])
	default:
		return fmt.Errorf("unknown plugin command %q; expected list, enable or disable", args[0])
	}
}

// PluginEnable turns a plugin on, and says what it will now be asked for.
func PluginEnable(args []string) error {
	if len(args) == 0 {
		return usageFor("plugin enable", "rho plugin enable <name>[@<version>]")
	}
	if err := Enable([]string{args[0]}); err != nil {
		return err
	}
	reportWhatItProvides(args[0])
	return nil
}

// PluginDisable turns a plugin off without uninstalling it.
func PluginDisable(args []string) error {
	if len(args) == 0 {
		return usageFor("plugin disable", "rho plugin disable <name>")
	}
	return Disable(args)
}

// reportWhatItProvides says what a plugin is for, so that turning it on is a
// decision rather than a guess.
func reportWhatItProvides(reference string) {
	_, name, _ := parseModuleRef(reference)
	home := deps.RhoHome()
	if home == "" {
		return
	}
	root := deps.InstalledPluginRoot(name)
	if root == "" {
		return
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil || manifest.Extends == nil {
		return
	}
	contracts := plugins.Source{RhoHome: home}.Contracts(manifest.Extends.Component)
	if len(contracts) == 0 {
		fmt.Printf("  it will be offered to every run of %s\n", manifest.Extends.Component)
		return
	}
	for _, contract := range contracts {
		offered := ""
		for _, provided := range manifest.Extends.Implements {
			if provided == contract.Name {
				offered = " (fills this)"
				break
			}
		}
		cardinality := "every enabled plugin fills this"
		if contract.One {
			cardinality = "exactly one plugin fills this"
		}
		fmt.Printf("  %s.%s: %s — %s%s\n",
			manifest.Extends.Component, contract.Name, contract.Description, cardinality, offered)
	}
}

/* -------------------------------------------------------------------------- */
/*  helpers                                                                    */
/* -------------------------------------------------------------------------- */

// parseModuleRef reads a reference, which is a name, a kind, and optionally a
// version.
//
// The kind is not part of a name because a name says what a thing is called; a
// person enabling something has a name in front of them and means the one that
// exists. Where both a component and a plugin carry the name, the kind is taken
// from which of them is installed.
func parseModuleRef(reference string) (kind, name, version string) {
	name, version, _ = strings.Cut(reference, "@")
	if kind := installedKindOf(name); kind != "" {
		return kind, name, version
	}
	return deps.KindComponent, name, version
}

func installedKindOf(name string) string {
	if len(deps.InstalledVersions(deps.KindPlugin, name)) > 0 {
		return deps.KindPlugin
	}
	return deps.KindComponent
}

// kindsArg is which kinds a listing covers, from an argument or all of them.
func kindsArg(args []string) []string {
	if len(args) > 0 {
		return []string{args[0]}
	}
	return []string{deps.KindComponent, deps.KindPlugin, deps.KindModule}
}

// usageFor is the message a command with nothing to act on gets.
func usageFor(name, line string) error {
	return fmt.Errorf("usage: %s", line)
}

// writeRows prints a table, aligned, so a listing is readable rather than a
// column of values a reader has to pair up themselves.
func writeRows(rows [][]string) {
	if len(rows) == 0 {
		return
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	for _, row := range rows {
		parts := make([]string, 0, len(row))
		for i, cell := range row {
			parts = append(parts, cell+strings.Repeat(" ", widths[i]-len(cell)))
		}
		fmt.Println(strings.TrimRight(strings.Join(parts, "  "), " "))
	}
}

// enabledStateFor is what to say about a module in a listing: whether the
// version named is the one a run uses.
func enabledStateFor(kind, name, version string) (string, error) {
	state, err := deps.LoadEnabled()
	if err != nil {
		return "", err
	}
	if state.IsEnabled(kind, name, version) {
		return "enabled", nil
	}
	return "not enabled", nil
}
