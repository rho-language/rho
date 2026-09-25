//
// @file info.go
// @brief Rho Module Info - Enterprise v3.
//
// Reads and returns module metadata from module.yml.
// Includes plugin hooks if present.
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

	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

// ModuleInfo represents complete module metadata including hooks
type ModuleInfo struct {
	ModuleYAML
	Hooks []string
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// ReadModuleInfo reads module.yml and plugin/hooks.yml (if plugin type)
func ReadModuleInfo(dir string) (ModuleInfo, error) {
	modPath := filepath.Join(dir, "module.yml")

	// Delegate to the validated loader so identity metadata cannot drift
	// from what build/pack stages see. This also keeps the command field.
	mod, err := LoadModuleYAML(modPath)
	if err != nil {
		return ModuleInfo{}, err
	}

	info := ModuleInfo{ModuleYAML: mod}
	if mod.Module.Type == "plugin" {
		hooksPath := filepath.Join(dir, "plugin", "hooks.yml")
		hooksData, readErr := os.ReadFile(hooksPath)
		if readErr != nil {
			return info, nil
		}
		hooksRoot, parseErr := yaml.Load(string(hooksData))
		if parseErr != nil {
			return ModuleInfo{}, fmt.Errorf("invalid plugin/hooks.yml: %w", parseErr)
		}
		if hooksNode, ok := hooksRoot.Map["hooks"]; ok {
			for _, h := range hooksNode.List {
				info.Hooks = append(info.Hooks, h.Str)
			}
		}
	}

	return info, nil
}