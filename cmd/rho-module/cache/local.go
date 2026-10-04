//
// @file local.go
// @brief Rho Local Cache Manager - Enterprise v3.
//
// Manages the local project cache located at:
//   <project>/.rho-deps
//
// Responsibilities:
//   - list locally installed modules
//   - check if a module version exists locally
//   - get path to local module
//   - integrate with installer + updater + remover
//
// Used by:
//   - installer.go
//   - update.go
//   - remove.go
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cache

import (
    "os"
    "path/filepath"
    "strings"
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

func LocalCacheDir(dir string) string {
    return filepath.Join(dir, ".rho-deps")
}

func ListLocalModules(dir string) ([]LocalEntry, error) {
    base := LocalCacheDir(dir)

    entries, err := os.ReadDir(base)
    if err != nil {
        return nil, err
    }

    out := []LocalEntry{}

    for _, e := range entries {
        if !e.IsDir() {
            continue
        }

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

func LocalModuleExists(dir, name, version string) bool {
    path := filepath.Join(LocalCacheDir(dir), name+"@"+version)
    _, err := os.Stat(path)
    return err == nil
}

func LocalModulePath(dir, name, version string) string {
    return filepath.Join(LocalCacheDir(dir), name+"@"+version)
}
