//
// @file verify.go
// @brief Rho Install - Installation verification subsystem.
//
// Responsibilities:
//   - Verify ~/.rho directory structure
//   - Verify presence of Rho binaries
//   - Verify registry and cache existence
//   - Provide API for rho CLI to check installation status
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package install

import (
    "fmt"
    "os"
    "path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// IsInstalled performs a full installation check.
func IsInstalled() bool {
    if !pathsExist() {
        return false
    }
    if !binariesExist() {
        return false
    }
    if !RegistryExists() {
        return false
    }
    if !CacheExists() {
        return false
    }
    if !ConfigExists() {
        return false
    }
    return true
}

// VerifyInstallation returns an error if installation is incomplete.
func VerifyInstallation() error {
	if !pathsExist() {
		return fmt.Errorf("Rho is not installed: missing ~/.rho structure")
	}
	if !binariesExist() {
		return fmt.Errorf("Rho is not installed: missing binaries in ~/.rho/bin")
	}
	if !RegistryExists() {
		return fmt.Errorf("Rho is not installed: missing registry.yaml")
	}
	if !CacheExists() {
		return fmt.Errorf("Rho is not installed: missing cache files")
	}
	if !ConfigExists() {
		return fmt.Errorf("Rho is not installed: missing config files")
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Internal Helpers                                                           */
/* -------------------------------------------------------------------------- */

func pathsExist() bool {
    home, err := GetRhoHome()
    if err != nil {
        return false
    }

    dirs := []string{
        filepath.Join(home, "bin"),
        filepath.Join(home, "modules", "installed"),
        filepath.Join(home, "registry", "local"),
        filepath.Join(home, "cache"),
        filepath.Join(home, "config"),
    }

    for _, d := range dirs {
        if _, err := os.Stat(d); os.IsNotExist(err) {
            return false
        }
    }

    return true
}

func binariesExist() bool {
    binPath, err := GetBinPath()
    if err != nil {
        return false
    }

    binaries := []string{
        "rho",
        "rho-fmt",
        "rho-test",
        "rho-deps",
        "rho-doc",
    }

    for _, b := range binaries {
        if _, err := os.Stat(filepath.Join(binPath, b)); os.IsNotExist(err) {
            return false
        }
    }

    return true
}
