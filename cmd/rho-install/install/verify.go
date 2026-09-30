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
		filepath.Join(home, "auth"),
		filepath.Join(home, "pkg", "mod"),
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

	// The same set an install puts in place, and the same distinction between
	// what a toolchain is and what an editor needs. A component is installed as
	// a module and reached through the command its manifest publishes rather
	// than through a binary sitting in the bin directory — but the language
	// server is built with the toolchain, so it is checked here too.
	//
	// A missing optional binary does not make this false, and it must not: the
	// CLI asks this before running anything, so an installation without a
	// language server reported as "not installed" would be an installation where
	// no command runs at all.
	for _, b := range toolchainBinaries {
		if _, err := os.Stat(filepath.Join(binPath, b.name)); os.IsNotExist(err) {
			if b.required {
				return false
			}
		}
	}

	return true
}
