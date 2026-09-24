//
// @file config_test.go
// @brief Tests for Rho Install - Global configuration subsystem.
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
/*  InitConfig                                                                 */
/* -------------------------------------------------------------------------- */

func TestInitConfig(t *testing.T) {
    // Ensure ~/.rho exists
    if err := EnsurePaths(); err != nil {
        t.Fatalf("EnsurePaths failed: %v", err)
    }

    // Create config files
    if err := InitConfig(); err != nil {
        t.Fatalf("InitConfig failed: %v", err)
    }

    configPath, _ := GetConfigPath()

    expected := []string{
        "settings.toml",
        "paths.toml",
    }

    for _, f := range expected {
        file := filepath.Join(configPath, f)
        if _, err := os.Stat(file); os.IsNotExist(err) {
            t.Fatalf("config file missing: %s", file)
        }
    }
}

/* -------------------------------------------------------------------------- */
/*  ConfigExists                                                               */
/* -------------------------------------------------------------------------- */

func TestConfigExists(t *testing.T) {
    EnsurePaths()
    InitConfig()

    if !ConfigExists() {
        t.Fatalf("ConfigExists returned false but config files exist")
    }
}
