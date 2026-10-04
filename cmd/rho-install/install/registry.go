//
// @file registry.go
// @brief Rho Install - Local registry initialization subsystem.
//
// Responsibilities:
//   - Create ~/.rho/registry/local/registry.yaml
//   - Provide deterministic initial registry structure
//   - Ensure compatibility with Rho Module System
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package install

import (
    "fmt"
    "os"
    "path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Registry Template                                                          */
/* -------------------------------------------------------------------------- */

const registryTemplate = `components:
plugins:
active_components:
active_plugins:
settings: {}
`

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// InitLocalRegistry creates the initial registry.yaml file.
func InitLocalRegistry() error {
    regPath, err := GetRegistryPath()
    if err != nil {
        return err
    }

    file := filepath.Join(regPath, "registry.yaml")

    // Avoid overwriting existing registry
    if _, err := os.Stat(file); err == nil {
        return nil
    }

    if err := os.WriteFile(file, []byte(registryTemplate), 0o644); err != nil {
        return fmt.Errorf("failed to write registry.yaml: %w", err)
    }

    return nil
}

// RegistryExists checks if registry.yaml exists.
func RegistryExists() bool {
    regPath, err := GetRegistryPath()
    if err != nil {
        return false
    }

    file := filepath.Join(regPath, "registry.yaml")
    _, err = os.Stat(file)
    return err == nil
}
