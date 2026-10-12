//
// @file update.go
// @brief Rho Module Updater - Enterprise v3.
//
// Updates dependencies by:
//   - Resolving latest compatible versions
//   - Comparing with rho.lock
//   - Downloading updated modules
//   - Installing updated modules into .rho-deps/
//   - Regenerating rho.lock
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
	"path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func UpdateDependencies(dir string) error {
	// Step 1: Load manifest
	modPath := filepath.Join(dir, "module.yml")
	manifest, err := LoadModuleYAML(modPath)
	if err != nil {
		return fmt.Errorf("cannot load module.yml: %w", err)
	}

	// Step 2: Load lockfile
	lock, err := loadLockfile(dir)
	if err != nil {
		return fmt.Errorf("cannot load rho.lock: %w", err)
	}

	// Step 3: Resolve latest compatible versions
	resolved, err := resolveDependencies(manifest.Deps)
	if err != nil {
		return fmt.Errorf("dependency resolution failed: %w", err)
	}

	// Step 4: Compare with lockfile
	updates := map[string]string{}
	for name, newVersion := range resolved {
		oldVersion := lock[name]
		if oldVersion != newVersion {
			updates[name] = newVersion
		}
	}

	// Nothing to update
	if len(updates) == 0 {
		return nil
	}

	// Step 5: Download updated modules, recording each package's digest
	hashes := LoadLockfileHashes(dir)
	if hashes == nil {
		hashes = map[string]string{}
	}
	for name, version := range updates {
		spec := fmt.Sprintf("%s@%s", name, version)
		digest, err := DownloadModuleDigest(spec)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", spec, err)
		}
		hashes[name] = digest
	}

	// Step 6: Install updated modules
	if err := installLocalCache(dir, resolved); err != nil {
		return fmt.Errorf("local cache error: %w", err)
	}

	// Step 7: Write updated lockfile
	if err := writeLockfile(dir, resolved, hashes); err != nil {
		return fmt.Errorf("cannot write rho.lock: %w", err)
	}

	return nil
}
