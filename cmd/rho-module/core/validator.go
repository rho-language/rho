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

	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func ValidateModule(dir string) error {
    // Load manifest
    modPath := filepath.Join(dir, "module.yml")
    manifest, err := LoadModuleYAML(modPath)
    if err != nil {
        return err
    }

    // Validate semver
    if !IsValidSemver(manifest.Module.Version) {
        return fmt.Errorf("invalid semantic version: %s", manifest.Module.Version)
    }

    // Validate main file (skip for component/plugin - binary is built during pack)
    if manifest.Module.Type != "component" && manifest.Module.Type != "plugin" {
        mainPath := filepath.Join(dir, manifest.Main)
        if _, err := os.Stat(mainPath); err != nil {
            return fmt.Errorf("main file not found: %s", manifest.Main)
        }
    }

    // Validate plugin structure
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
