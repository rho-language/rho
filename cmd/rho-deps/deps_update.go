//
// @file deps_update.go
// @brief Rho Dependency Update - Enterprise.
//
// Implements `rho deps update`, the deterministic dependency updater for Rho.
// It re-resolves all dependencies declared in rho.mod, re-installs them into
// `.rho-deps/`, and writes a fresh rho.lock.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"rho/src/deps"
)

/* -------------------------------------------------------------------------- */
/*  rho deps update                                                           */
/* -------------------------------------------------------------------------- */

func runDepsUpdate() {
	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps update:", err)
		os.Exit(1)
	}

	manifestPath := filepath.Join(projectRoot, "rho.mod")
	manifest, err := deps.LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps update:", err)
		os.Exit(1)
	}

	// Clean local cache to force re-clone
	localCache := filepath.Join(projectRoot, ".rho-deps")
	os.RemoveAll(localCache)

	locator := &deps.DefaultLocator{
		ProjectRoot: projectRoot,
		GlobalCache: filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod"),
		LocalCache:  localCache,
	}

	resolver := &deps.Resolver{Locator: locator}
	_, lock, err := resolver.Resolve(manifest, projectRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps update:", err)
		os.Exit(1)
	}

	// Write new lockfile
	lockPath := filepath.Join(projectRoot, "rho.lock")
	if err := lock.Save(lockPath); err != nil {
		fmt.Fprintln(os.Stderr, "rho deps update:", err)
		os.Exit(1)
	}

	fmt.Println("Dependencies updated:")
	for _, p := range lock.Packages {
		fmt.Printf("  - %s %s (%s)\n", p.Name, p.Version, p.Path)
	}
}
