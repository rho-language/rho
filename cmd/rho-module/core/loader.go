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
	"rho/src/deps"
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

// LoadInstalledModule loads the module a toolchain means when it names one:
// the enabled version if one is enabled, and the newest otherwise.
//
// Several versions can be installed at once and only one of them runs, so
// loading "the module by name" as whichever directory a scan reached first is
// loading a version nobody chose — an old one, kept beside the new one for a
// reason. A caller that knows which version it means uses
// LoadInstalledModuleVersion instead.
func LoadInstalledModule(name string) (*Module, error) {
	modulesDir, err := paths.GetModulesInstalledPath()
	if err != nil {
		return nil, err
	}

	candidates := []*Module{}
	kinds := []string{}
	for _, moduleType := range []string{"component", "plugin"} {
		typeDir := filepath.Join(modulesDir, moduleType)
		entries, readErr := os.ReadDir(typeDir)
		if readErr != nil {
			continue
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			// Check if directory name matches name@version pattern
			if len(e.Name()) > len(name) && e.Name()[:len(name)] == name && e.Name()[len(name)] == '@' {
				modulePath := filepath.Join(typeDir, e.Name())
				manifest, manifestErr := LoadModuleYAML(filepath.Join(modulePath, "module.yml"))
				if manifestErr != nil || manifest.Module.Name != name {
					continue
				}
				candidates = append(candidates, &Module{
					Name:     name,
					Version:  manifest.Module.Version,
					RootPath: modulePath,
					Manifest: manifest,
				})
				kinds = append(kinds, moduleType)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("module not installed: %s", name)
	}

	// The enabled version is the one a run resolves to, so it is the one the
	// toolchain means when it names the module.
	if state, stateErr := deps.LoadEnabled(); stateErr == nil && state != nil {
		for i, candidate := range candidates {
			if state.IsEnabled(kinds[i], name, candidate.Version) {
				return candidate, nil
			}
		}
	}

	// Nothing enabled: the newest, which is the one enabling would turn on.
	newest := candidates[0]
	for _, candidate := range candidates[1:] {
		if compareVersions(candidate.Version, newest.Version) > 0 {
			newest = candidate
		}
	}
	return newest, nil
}

// compareVersions orders two version strings; it lives with the registry that
// already needs it, so a name has one meaning wherever it is compared.

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
