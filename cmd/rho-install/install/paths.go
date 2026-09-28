//
// @file paths.go
// @brief Rho Install - Paths subsystem.
//
// Responsibilities:
//   - Determine ~/.rho home directory
//   - Create required folder structure
//   - Provide deterministic access to all Rho paths
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
	"sync"
)

/* -------------------------------------------------------------------------- */
/*  Constants                                                                  */
/* -------------------------------------------------------------------------- */

const (
	RhoHomeDirName = ".rho"
	// RhoHomeEnvVar names the installation somewhere other than the user's home.
	// It is the same variable internal/paths reads, because an installation and
	// the checks that look for it have to be answering about one directory.
	RhoHomeEnvVar = "RHO_HOME"
)

var (
	testRhoHome string
	testMu      sync.Mutex
)

// SetTestRhoHome overrides the Rho home directory for testing.
// Call with empty string to reset.
func SetTestRhoHome(path string) {
	testMu.Lock()
	defer testMu.Unlock()
	testRhoHome = path
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// GetRhoHome returns the absolute path to the local registry: ~/.rho, or the
// directory RHO_HOME names when it is set.
//
// It honours RHO_HOME for the same reason internal/paths does — a sandbox, a CI
// runner or a container points the installation somewhere else rather than
// writing into a real user home — and it is the same variable, so the two
// answers agree. They disagreed once, and the effect was that an installation
// under RHO_HOME looked absent to the check that decides whether to run at all.
func GetRhoHome() (string, error) {
	testMu.Lock()
	defer testMu.Unlock()
	if testRhoHome != "" {
		return testRhoHome, nil
	}
	if named := os.Getenv(RhoHomeEnvVar); named != "" {
		return named, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user home directory: %w", err)
	}
	return filepath.Join(home, RhoHomeDirName), nil
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
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}

	return nil
}

// GetBinPath returns ~/.rho/bin.
func GetBinPath() (string, error) {
	home, err := GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "bin"), nil
}

// GetRegistryPath returns ~/.rho/registry/local.
func GetRegistryPath() (string, error) {
	home, err := GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "registry", "local"), nil
}

// GetCachePath returns ~/.rho/cache.
func GetCachePath() (string, error) {
	home, err := GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "cache"), nil
}

// GetConfigPath returns ~/.rho/config.
func GetConfigPath() (string, error) {
	home, err := GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config"), nil
}

/* ------------------------------------------------------------------------- */
/*  What the tool home holds                                                  */
/* ------------------------------------------------------------------------- */

// HomeContents lists the entries a Rho home is made of, so a caller removing it
// can say what it is removing rather than deleting a directory wholesale.
func HomeContents() []string {
	return []string{"bin", "modules", "registry", "cache", "config", "auth"}
}

// IsUserData reports whether an entry holds something the user would miss: the
// registry, the installed modules, the module cache and the credentials. The
// toolchain's own binaries are not user data.
func IsUserData(name string) bool {
	switch name {
	case "modules", "registry", "cache", "auth":
		return true
	default:
		return false
	}
}
