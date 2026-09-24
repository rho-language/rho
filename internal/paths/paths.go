//
// @file paths.go
// @brief Rho Shared Paths - Enterprise v3.
//
// Provides deterministic access to all Rho paths.
// Single source of truth for test isolation via SetTestRhoHome.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

/* -------------------------------------------------------------------------- */
/*  Constants                                                                  */
/* -------------------------------------------------------------------------- */

const (
	RhoHomeDirName = ".rho"
)

var (
	testRhoHome string
	testMu      sync.Mutex
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// SetTestRhoHome overrides the Rho home directory for testing.
// Call with empty string to reset.
func SetTestRhoHome(path string) {
	testMu.Lock()
	defer testMu.Unlock()
	testRhoHome = path
}

// GetRhoHome returns the absolute path to ~/.rho.
func GetRhoHome() (string, error) {
	testMu.Lock()
	defer testMu.Unlock()
	if testRhoHome != "" {
		return testRhoHome, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user home directory: %w", err)
	}
	return filepath.Join(home, RhoHomeDirName), nil
}

// GetRegistryPath returns ~/.rho/registry/local.
func GetRegistryPath() (string, error) {
	home, err := GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "registry", "local"), nil
}

// GetModulesInstalledPath returns ~/.rho/modules/installed.
func GetModulesInstalledPath() (string, error) {
	home, err := GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "modules", "installed"), nil
}

// GetCachePath returns ~/.rho/pkg/mod.
func GetCachePath() (string, error) {
	home, err := GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "pkg", "mod"), nil
}

// EnsurePaths creates the full ~/.rho directory structure.
func EnsurePaths() error {
	rhoHome, err := GetRhoHome()
	if err != nil {
		return err
	}

	dirs := []string{
		filepath.Join(rhoHome, "bin"),
		filepath.Join(rhoHome, "modules", "installed"),
		filepath.Join(rhoHome, "registry", "local"),
		filepath.Join(rhoHome, "cache"),
		filepath.Join(rhoHome, "config"),
		filepath.Join(rhoHome, "pkg", "mod"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}

	return nil
}