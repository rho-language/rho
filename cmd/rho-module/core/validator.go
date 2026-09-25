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
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

	// Step 5: Validate plugin structure
	if manifest.Module.Type == "plugin" {
		if err := validatePlugin(dir); err != nil {
			return err
		}
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/*  Plugin Validation                                                           */
/* -------------------------------------------------------------------------- */

func validatePlugin(dir string) error {
	pluginDir := filepath.Join(dir, "plugin")
	hooksPath := filepath.Join(pluginDir, "hooks.yml")

	if _, err := os.Stat(pluginDir); err != nil {
		return errors.New("plugin directory missing")
	}

	if _, err := os.Stat(hooksPath); err != nil {
		return errors.New("plugin/hooks.yml missing")
	}

	// Schema-validate hooks.yml
	result, err := schema.ValidateHooksFile(hooksPath)
	if err != nil {
		return fmt.Errorf("cannot validate hooks.yml: %w", err)
	}
		if result.HasErrors() {
		return fmt.Errorf("hooks.yml schema validation failed:\n%s", result.ErrorString())
	}

	// Load hooks.yml
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		return fmt.Errorf("cannot read hooks.yml: %w", err)
	}

	root, err := yaml.Load(string(data))
	if err != nil {
		return fmt.Errorf("invalid hooks.yml: %w", err)
	}

	hooks := root.Map["hooks"].List
	if len(hooks) == 0 {
		return errors.New("plugin has no hooks defined")
	}

	// Validate hook names
	for _, h := range hooks {
		if h.Str == "" {
			return errors.New("empty hook name in hooks.yml")
		}
	}

	return nil
}
