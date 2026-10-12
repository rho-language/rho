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

// LoadInstalledModuleVersion loads one installed version of a module.
//
// Several versions of a module can be installed at once and only one of them
// runs, so loading "the module by name" is loading whichever directory a scan
// reached first — which is not the version anybody chose. A caller that has
// decided which version it means loads that one.
func LoadInstalledModuleVersion(name, version string) (*Module, error) {
	modulesDir, err := paths.GetModulesInstalledPath()
	if err != nil {
		return nil, err
	}
	for _, moduleType := range []string{"component", "plugin"} {
		modulePath := filepath.Join(modulesDir, moduleType, name+"@"+version)
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
	return nil, fmt.Errorf("module not installed: %s@%s", name, version)
}
