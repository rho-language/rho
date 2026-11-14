//
// @file validator.go
// @brief Rho Module Validator - Enterprise v3.
//
// Performs full validation of a Rho module or plugin:
//   - module.yml structure
//   - semantic version correctness
//   - main file existence
//   - plugin hooks validation
//   - directory structure
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"rho/src/integration"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/schema"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func ValidateModule(dir string) error {
	// Step 1: Read the manifest and validate it against the toolchain's schema
	// and, when the module's platform publishes one, its ecosystem contract.
	// Validation is strict here: a type the toolchain does not define is only
	// valid when the contract that defines it can be loaded.
	modPath := filepath.Join(dir, "module.yml")
	if _, err := os.Stat(modPath); err != nil {
		return fmt.Errorf("module.yml not found")
	}
	manifest, err := LoadModuleYAMLStrict(modPath)
	if err != nil {
		return err
	}

	// Step 3: Validate semver
	if !IsValidSemver(manifest.Module.Version) {
		return fmt.Errorf("invalid semantic version: %s", manifest.Module.Version)
	}

	// Step 4: Validate main file. Only a Rho library names a source file that
	// has to be there: a component's and a plugin's main is a binary built while
	// packing, and a native module has no main at all.
	if schema.IsRhoLibraryType(manifest.Module.Type) {
		mainPath := filepath.Join(dir, manifest.Main)
		if _, err := os.Stat(mainPath); err != nil {
			return fmt.Errorf("main file not found: %s", manifest.Main)
		}
	}

	// Step 5: Validate what a component offers, and what a plugin plugs into.
	//
	// The relation is read through the manifest loader rather than from the
	// identity metadata, so what is validated here and what a tool resolves
	// when it loads a plugin are the same declaration.
	relation, err := deps.LoadManifest(modPath)
	if err != nil {
		return err
	}
	if err := validateProvides(relation); err != nil {
		return err
	}
	if relation.ModuleType == "plugin" {
		if err := validatePlugin(dir, relation); err != nil {
			return err
		}
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/*  Plugin Validation                                                           */
/* -------------------------------------------------------------------------- */

/* -------------------------------------------------------------------------- */
/*  Plugin Validation                                                           */
/* -------------------------------------------------------------------------- */

// validatePlugin checks that a plugin is attached to something.
//
// A plugin is not a thing in itself: it answers hooks of a component, and a run
// of that component is the only thing that ever calls it. So the declaration is
// checked here, against the component it names: the component has to exist, it
// has to offer the hooks the plugin claims, and every command has to answer a
// hook the plugin declared. A plugin that passes is attached; one that does not
// is a module that nothing will ever call, and saying so at validation is
// cheaper than discovering it from a build log.
func validatePlugin(dir string, manifest *deps.Manifest) error {
	extension := manifest.Extends
	// A plugin that only integrates a platform's data (ports, bindings) for the
	// platforms it serves plugs into no component; its integration is checked
	// by the integration contract.
	if manifest.Integration != nil && extension != nil && len(extension.Platforms) > 0 && len(extension.All()) == 0 {
		if problems := integration.Validate(dir, manifest); len(problems) > 0 {
			return fmt.Errorf("integration plugin does not meet the contract:\n  - %s", strings.Join(problems, "\n  - "))
		}
		return nil
	}
	if extension == nil || len(extension.All()) == 0 {
		return fmt.Errorf("plugin must declare `extends:` naming the component it plugs into")
	}
	if len(extension.Commands) == 0 {
		return fmt.Errorf("plugin must declare at least one command")
	}

	// A plugin may extend more than one component, and each relation is checked
	// on its own: the component has to exist, it has to offer the hooks the
	// plugin claims there, and the contracts it implements there have to be
	// contracts that component offers.
	targets := extension.All()
	for _, target := range targets {
		if err := validateTarget(dir, target); err != nil {
			return err
		}
	}

	// Every command has to answer a hook of a component it belongs to. A
	// command that names its component is checked against that one; one that
	// names none may answer any of the components the plugin extends.
	for _, command := range extension.Commands {
		stage := command.Stage()
		if !answersAny(targets, command.Component, stage) {
			return fmt.Errorf("command %q answers hook %q, which the plugin does not declare", command.Name, stage)
		}
	}

	// An integration plugin, the one bridge between an ecosystem and the
	// toolchain, is held to the integration contract (src/integration).
	if problems := integration.Validate(dir, manifest); len(problems) > 0 {
		return fmt.Errorf("integration plugin does not meet the contract:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// answersAny reports whether a hook is declared by a target the command belongs
// to: the one it names, or any target when it names none.
func answersAny(targets []deps.ExtendsTarget, component, hook string) bool {
	for _, target := range targets {
		if component != "" && target.Component != component {
			continue
		}
		for _, offered := range target.Hooks {
			if offered == hook {
				return true
			}
		}
	}
	return false
}

// validateTarget checks one component a plugin extends.
func validateTarget(dir string, target deps.ExtendsTarget) error {
	if len(target.Hooks) == 0 {
		return fmt.Errorf("plugin must declare the hooks of %s it answers", target.Component)
	}

	// The named component has to be something that can run the hooks. It is
	// located the way a dependency is, so an installed component and a
	// component in the same tree are both found.
	component, err := findComponent(dir, target.Component)
	if err != nil {
		return err
	}
	if component.Provides == nil || len(component.Provides.Hooks) == 0 {
		return fmt.Errorf("%s offers no hooks; a plugin has nothing to answer", target.Component)
	}
	offered := map[string]bool{}
	for _, hook := range component.Provides.Hooks {
		offered[hook] = true
	}
	for _, hook := range target.Hooks {
		if !offered[hook] {
			return fmt.Errorf("hook %q is not offered by %s; it offers %s",
				hook, target.Component, strings.Join(component.Provides.Hooks, ", "))
		}
	}
	return validateContracts(target, component)
}

// validateContracts checks what a plugin says it is against what its component
// says it can be.
//
// A plugin that implements a contract the component does not offer is a plugin
// written against a component that has since changed: it will never be run, and
// finding that out at validation is much cheaper than finding it out from a
// pipeline that silently did less than the plugin was written to do.
//
// A plugin that names no contract is accepted. A component whose plugins are all
// of one kind has no reason to name them, and requiring it to would be a rule
// with no failure behind it.
func validateContracts(target deps.ExtendsTarget, component *deps.Manifest) error {
	if len(target.Implements) == 0 || component.Provides == nil {
		return nil
	}
	contracts := map[string]deps.Contract{}
	for _, contract := range component.Provides.Contracts {
		contracts[contract.Name] = contract
	}
	for _, named := range target.Implements {
		contract, offered := contracts[named]
		if !offered {
			available := make([]string, 0, len(contracts))
			for name := range contracts {
				available = append(available, name)
			}
			sort.Strings(available)
			if len(available) == 0 {
				return fmt.Errorf(
					"%s offers no contracts, so %s cannot implement %q; "+
						"a component says what a plugin is for under `provides: contracts:`",
					target.Component, target.Component, named)
			}
			return fmt.Errorf(
				"%q is not a contract of %s; it offers %s",
				named, target.Component, strings.Join(available, ", "))
		}
		// The contract names the hooks a plugin filling it answers, which makes
		// it checkable rather than a label. A plugin that answers a hook outside
		// the contract it declared is answering something nobody asked it for.
		if len(contract.Hooks) == 0 {
			continue
		}
		within := map[string]bool{}
		for _, hook := range contract.Hooks {
			within[hook] = true
		}
		for _, hook := range target.Hooks {
			if !within[hook] {
				return fmt.Errorf(
					"hook %q is outside the %q contract of %s, which covers %s",
					hook, contract.Name, target.Component, strings.Join(contract.Hooks, ", "))
			}
		}
	}
	return nil
}

// coreComponents are the toolchain's own commands, which are part of the
// installed toolchain rather than components installed under the module tree.
//
// A plugin may extend one of them (an integration plugin answers `rho-module`'s
// manifest contract with its ecosystem's module types), so validation has to know what the command offers without
// a module manifest on disk. The contracts here are the same ones the command
// documents; the data an ecosystem's answer carries is its own and is not
// declared here.
var coreComponents = map[string]*deps.Manifest{
	"rho-module": {
		ModuleName: "rho-module",
		ModuleType: "component",
		Provides: &deps.Provides{
			Hooks: []string{"contract"},
			Contracts: []deps.Contract{
				{Name: "module-manifest", Description: "The ecosystem's module types and the manifest structure each one is written to.", Hooks: []string{"contract"}},
			},
		},
	},
}

// findComponent loads the manifest of a named component, from the installation
// or from the tree the caller is in.
func findComponent(from, name string) (*deps.Manifest, error) {
	if component, ok := coreComponents[name]; ok {
		return component, nil
	}
	locator := &deps.DefaultLocator{ProjectRoot: from}
	root, err := locator.Locate(deps.DepSpec{Name: name})
	if err != nil {
		return nil, fmt.Errorf("plugin extends %q, which cannot be resolved: %w", name, err)
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil {
		return nil, fmt.Errorf("plugin extends %q, whose manifest cannot be read", name)
	}
	if manifest.ModuleType != "component" {
		return nil, fmt.Errorf("plugin extends %q, which is a %s; a plugin plugs into a component", name, manifest.ModuleType)
	}
	return manifest, nil
}

// validateProvides checks what a component offers, which is the list every
// plugin is measured against.
func validateProvides(manifest *deps.Manifest) error {
	if manifest.Provides == nil {
		return nil
	}
	// Only a component runs a tool a plugin can attach to, so a module of any
	// other type offering hooks is offering a place that nothing will ask for.
	// What any module may provide is the import it is used by, its exports and
	// a provider's settings.
	offersHooks := len(manifest.Provides.Hooks) > 0 || len(manifest.Provides.Contracts) > 0
	if !offersHooks && manifest.ModuleType != "component" {
		return nil
	}
	if manifest.ModuleType != "component" {
		return fmt.Errorf("a %s module cannot offer hooks; only a component runs a tool a plugin can extend", manifest.ModuleType)
	}
	seen := map[string]bool{}
	for _, hook := range manifest.Provides.Hooks {
		if hook == "" {
			return fmt.Errorf("provides an empty hook name")
		}
		if seen[hook] {
			return fmt.Errorf("provides hook %q more than once", hook)
		}
		seen[hook] = true
	}
	return nil
}
