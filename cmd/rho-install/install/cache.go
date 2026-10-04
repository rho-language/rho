//
// @file cache.go
// @brief Rho Install - Cache initialization subsystem.
//
// Responsibilities:
//   - Create ~/.rho/cache/*
//   - Provide deterministic initial cache structure
//   - Ensure compatibility with command short-circuit
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package install

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Cache Templates                                                            */
/* -------------------------------------------------------------------------- */

var (
    emptyRegistryCache = map[string]any{
        "modules":        []any{},
        "active_plugins": map[string]any{},
        "settings":       map[string]any{},
    }

    emptyModulesCache = map[string]any{
        "installed": []any{},
    }

    emptyActivePluginsCache = map[string]any{}
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// InitCache creates all cache files in ~/.rho/cache.
func InitCache() error {
    cachePath, err := GetMetadataCachePath()
    if err != nil {
        return err
    }

    files := map[string]map[string]any{
        "registry.json":       emptyRegistryCache,
        "modules.json":        emptyModulesCache,
        "active_plugins.json": emptyActivePluginsCache,
    }

    for name, content := range files {
        file := filepath.Join(cachePath, name)

        // Avoid overwriting existing cache
        if _, err := os.Stat(file); err == nil {
            continue
        }

        data, err := json.MarshalIndent(content, "", "  ")
        if err != nil {
            return fmt.Errorf("failed to marshal cache file %s: %w", name, err)
        }

        if err := os.WriteFile(file, data, 0o644); err != nil {
            return fmt.Errorf("failed to write cache file %s: %w", name, err)
        }
    }

    return nil
}

// CacheExists checks if all cache files exist.
func CacheExists() bool {
    cachePath, err := GetMetadataCachePath()
    if err != nil {
        return false
    }

    files := []string{
        "registry.json",
        "modules.json",
        "active_plugins.json",
    }

    for _, f := range files {
        if _, err := os.Stat(filepath.Join(cachePath, f)); os.IsNotExist(err) {
            return false
        }
    }

    return true
}
