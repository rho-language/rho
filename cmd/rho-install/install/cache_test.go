//
// @file cache_test.go
// @brief Tests for Rho Install - Cache initialization subsystem.
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package install

import (
    "os"
    "path/filepath"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  InitCache                                                                  */
/* -------------------------------------------------------------------------- */

func TestInitCache(t *testing.T) {
    // Ensure ~/.rho exists
    if err := EnsurePaths(); err != nil {
        t.Fatalf("EnsurePaths failed: %v", err)
    }

    // Create cache
    if err := InitCache(); err != nil {
        t.Fatalf("InitCache failed: %v", err)
    }

    cachePath, _ := GetCachePath()

    expected := []string{
        "registry.json",
        "modules.json",
        "active_plugins.json",
    }

    for _, f := range expected {
        file := filepath.Join(cachePath, f)
        if _, err := os.Stat(file); os.IsNotExist(err) {
            t.Fatalf("cache file missing: %s", file)
        }
    }
}

/* -------------------------------------------------------------------------- */
/*  CacheExists                                                                */
/* -------------------------------------------------------------------------- */

func TestCacheExists(t *testing.T) {
    // Ensure ~/.rho exists
    EnsurePaths()
    InitCache()

    if !CacheExists() {
        t.Fatalf("CacheExists returned false but cache files exist")
    }
}
