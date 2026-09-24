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

	data, err := os.ReadFile(modPath)
	if err != nil {
		return ModuleInfo{}, fmt.Errorf("cannot read module.yml: %w", err)
	}

	root, err := yaml.Load(string(data))
	if err != nil {
		return ModuleInfo{}, fmt.Errorf("invalid YAML in module.yml: %w", err)
	}

	mod := ModuleYAML{
		Module: ModuleSection{
			Name:    root.Map["module"].Map["name"].Str,
			Type:    root.Map["module"].Map["type"].Str,
			Version: root.Map["module"].Map["version"].Str,
		},
		Main:     root.Map["main"].Str,
		Deps:     map[string]string{},
		Features: []string{},
	}

	// deps
	if depsNode, ok := root.Map["deps"]; ok {
		for k, v := range depsNode.Map {
			mod.Deps[k] = v.Str
		}
	}

	// features
	if featNode, ok := root.Map["features"]; ok {
		for _, f := range featNode.List {
			mod.Features = append(mod.Features, f.Str)
		}
	}

	// Validate required fields
	if mod.Module.Name == "" {
		return ModuleInfo{}, fmt.Errorf("module.name missing")
	}
	if mod.Module.Type == "" {
		return ModuleInfo{}, fmt.Errorf("module.type missing")
	}
	if mod.Module.Version == "" {
		return ModuleInfo{}, fmt.Errorf("module.version missing")
	}
	if mod.Main == "" {
		return ModuleInfo{}, fmt.Errorf("main missing")
	}

	// Read hooks for plugins
	info := ModuleInfo{ModuleYAML: mod}
	if mod.Module.Type == "plugin" {
		hooksPath := filepath.Join(dir, "plugin", "hooks.yml")
		if hooksData, err := os.ReadFile(hooksPath); err == nil {
			hooksRoot, _ := yaml.Load(string(hooksData))
			if hooksNode, ok := hooksRoot.Map["hooks"]; ok {
				for _, h := range hooksNode.List {
					info.Hooks = append(info.Hooks, h.Str)
				}
			}
		}
	}

	return info, nil
}