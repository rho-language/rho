//
// @file loader.go
// @brief Rho Module Loader - Enterprise v3.
//
// Loads installed modules from the local registry:
//   ~/.rho/registry/local/<name@version>/
//
// Responsibilities:
//   - locate module folder by name
//   - load module.yml
//   - expose Module struct for CLI dynamic dispatch
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"fmt"
	"os"
	"path/filepath"

	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

type Module struct {
	Name     string
	Version  string
	RootPath string
	Manifest ModuleYAML
}

/* -------------------------------------------------------------------------- */
/*  Load Installed Module                                                      */
/* -------------------------------------------------------------------------- */

func LoadInstalledModule(name string) (*Module, error) {
	// Scan ~/.rho/modules/installed/ to find the module
	modulesDir, err := paths.GetModulesInstalledPath()
	if err != nil {
		return nil, err
	}

	for _, moduleType := range []string{"component", "plugin"} {
		typeDir := filepath.Join(modulesDir, moduleType)
		if _, err := os.Stat(typeDir); err != nil {
			continue
		}

		entries, err := os.ReadDir(typeDir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}

			// Check if directory name matches name@version pattern
			if len(e.Name()) > len(name) && e.Name()[:len(name)] == name && e.Name()[len(name)] == '@' {
				modulePath := filepath.Join(typeDir, e.Name())
				manifestPath := filepath.Join(modulePath, "module.yml")

				manifest, err := LoadModuleYAML(manifestPath)
				if err != nil {
					continue
				}

				if manifest.Module.Name == name {
					return &Module{
						Name:     name,
						Version:  manifest.Module.Version,
						RootPath: modulePath,
						Manifest: manifest,
					}, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("module not installed: %s", name)
}
