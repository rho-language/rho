//
// @file local.go
// @brief Rho Local Registry - Enterprise v3.
//
// Provides access to the local registry stored at:
//   ~/.rho/registry/local
//
// Responsibilities:
//   - list installed modules
//   - read module metadata
//   - check available versions
//   - integrate with resolver
//
// Used by:
//   - resolver.go
//   - installer.go
//   - update.go
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package registry

import (
	"os"
	"path/filepath"
	"strings"

	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

type LocalEntry struct {
	Name    string
	Version string
	Path    string
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func ListLocalModules() ([]LocalEntry, error) {
	base, err := paths.GetRegistryPath()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}

	out := []LocalEntry{}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		// Format: name@version
		parts := strings.Split(e.Name(), "@")
		if len(parts) != 2 {
			continue
		}

		out = append(out, LocalEntry{
			Name:    parts[0],
			Version: parts[1],
			Path:    filepath.Join(base, e.Name()),
		})
	}

	return out, nil
}

/* -------------------------------------------------------------------------- */
/*  Get Versions for Module                                                    */
/* -------------------------------------------------------------------------- */

func LocalVersions(name string) ([]string, error) {
	modules, err := ListLocalModules()
	if err != nil {
		return nil, err
	}

	out := []string{}
	for _, m := range modules {
		if m.Name == name {
			out = append(out, m.Version)
		}
	}

	return out, nil
}
