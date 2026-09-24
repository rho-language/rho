//
// @file global.go
// @brief Rho Global Cache Manager - Enterprise v3.
//
// Manages the global module cache located at:
//   ~/.rho/pkg/mod
//
// Responsibilities:
//   - list globally cached modules
//   - check if a module version exists
//   - get path to cached module
//   - integrate with installer + updater + resolver
//
// Used by:
//   - installer.go
//   - update.go
//   - resolver.go
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cache

import (
	"os"
	"path/filepath"
	"strings"

	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

type GlobalEntry struct {
	Name    string
	Version string
	Path    string
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func GlobalCacheDir() string {
	path, _ := paths.GetCachePath()
	return path
}

func ListGlobalModules() ([]GlobalEntry, error) {
	base := GlobalCacheDir()

	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}

	out := []GlobalEntry{}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		// Format: name@version
		parts := strings.Split(e.Name(), "@")
		if len(parts) != 2 {
			continue
		}

		out = append(out, GlobalEntry{
			Name:    parts[0],
			Version: parts[1],
			Path:    filepath.Join(base, e.Name()),
		})
	}

	return out, nil
}

func GlobalModuleExists(name, version string) bool {
	path := filepath.Join(GlobalCacheDir(), name+"@"+version)
	_, err := os.Stat(path)
	return err == nil
}

func GlobalModulePath(name, version string) string {
	return filepath.Join(GlobalCacheDir(), name+"@"+version)
}
