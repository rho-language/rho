//
// @file remove.go
// @brief Rho Module Remover - Enterprise v3.
//
// Removes a dependency from the local project cache (.rho-deps/) and
// updates the rho.lock file.
//
// Steps:
//   1. Parse spec (name[@version])
//   2. Load rho.lock
//   3. Determine installed version
//   4. Remove directory from .rho-deps/
//   5. Update rho.lock
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func RemoveDependency(dir, spec string) error {
	name, version := parseSpec(spec)

	// Step 1: Load lockfile
	lock, err := loadLockfile(dir)
	if err != nil {
		return fmt.Errorf("cannot load rho.lock: %w", err)
	}

	// Step 2: Determine installed version
	installed, ok := lock[name]
	if !ok {
		return fmt.Errorf("dependency not found in lockfile: %s", name)
	}

	// If user specified version, check match
	if version != "" && version != installed {
		return fmt.Errorf("version mismatch: installed=%s, requested=%s", installed, version)
	}

	// Step 3: Remove directory
	depDir := filepath.Join(dir, ".rho-deps", fmt.Sprintf("%s@%s", name, installed))
	if err := os.RemoveAll(depDir); err != nil {
		return fmt.Errorf("cannot remove dependency directory: %w", err)
	}

	// Step 4: Update lockfile, keeping the digests of what remains
	hashes := LoadLockfileHashes(dir)
	delete(hashes, name)
	delete(lock, name)
	if err := writeLockfile(dir, lock, hashes); err != nil {
		return fmt.Errorf("cannot update rho.lock: %w", err)
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/*  Lockfile Loader                                                            */
/* -------------------------------------------------------------------------- */

func loadLockfile(dir string) (map[string]string, error) {
	path := filepath.Join(dir, "rho.lock")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	out := map[string]string{}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "lock:") || line == "" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		version := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		out[name] = version
	}

	return out, nil
}
