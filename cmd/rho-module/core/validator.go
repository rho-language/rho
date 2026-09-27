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
// SPDX-License-Identifier: MIT
//

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/deps"
	"rho/src/schema"
	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func ValidateModule(dir string) error {
	// Step 1: Schema-validate module.yml
	modPath := filepath.Join(dir, "module.yml")
	if _, err := os.Stat(modPath); err != nil {
		return fmt.Errorf("module.yml not found")
	}

	data, err := os.ReadFile(modPath)
	if err != nil {
		return fmt.Errorf("cannot read module.yml: %w", err)
	}

	root, err := yaml.Load(string(data))
	if err != nil {
		return fmt.Errorf("invalid YAML in module.yml: %w", err)
	}

	// Determine module type for type-aware validation
	moduleType := root.Map["module"].Map["type"].Str

	result := schema.ValidateModuleManifest(root, moduleType)
	if result.HasErrors() {
		return fmt.Errorf("schema validation failed:\n%s", result.ErrorString())
	}

	// Step 2: Load via existing loader (also validates required fields)
	manifest, err := LoadModuleYAML(modPath)
	if err != nil {
		return err
	}

	// Step 3: Validate semver
	if !IsValidSemver(manifest.Module.Version) {
		return fmt.Errorf("invalid semantic version: %s", manifest.Module.Version)
	}

	// Step 4: Validate main file (skip for component/plugin - binary is built during pack)
	if manifest.Module.Type != "component" && manifest.Module.Type != "plugin" {
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
	if extension == nil {
		return fmt.Errorf("plugin must declare `extends:` naming the component it plugs into")
	}
	if len(extension.Hooks) == 0 {
		return fmt.Errorf("plugin must declare the hooks of %s it answers", extension.Component)
	}
	if len(extension.Commands) == 0 {
		return fmt.Errorf("plugin must declare at least one command")
	}

	// The named component has to be something that can run the hooks. It is
	// located the way a dependency is, so an installed component and a
	// component in the same tree are both found.
	component, err := findComponent(dir, extension.Component)
	if err != nil {
		return err
	}
	if component.Provides == nil || len(component.Provides.Hooks) == 0 {
		return fmt.Errorf("%s offers no hooks; a plugin has nothing to answer", extension.Component)
	}
	offered := map[string]bool{}
	for _, hook := range component.Provides.Hooks {
		offered[hook] = true
	}
	for _, hook := range extension.Hooks {
		if !offered[hook] {
			return fmt.Errorf("hook %q is not offered by %s; it offers %s",
				hook, extension.Component, strings.Join(component.Provides.Hooks, ", "))
		}
	}
	for _, command := range extension.Commands {
		stage := command.Stage()
		if !extension.Answers(stage) {
			return fmt.Errorf("command %q answers hook %q, which the plugin does not declare", command.Name, stage)
		}
	}
	return nil
}

// findComponent loads the manifest of a named component, from the installation
// or from the tree the caller is in.
func findComponent(from, name string) (*deps.Manifest, error) {
	locator := &deps.DefaultLocator{ProjectRoot: from}
	root, err := locator.Locate(deps.DepSpec{Name: name})
	if err != nil {
		return nil, fmt.Errorf("plugin extends %q, which cannot be resolved: %w", name, err)
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil {
		return nil, fmt.Errorf("plugin extends %q, whose manifest cannot be read", name)
	}
	if manifest.ModuleType != "component" && manifest.ModuleType != "plugin" {
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
