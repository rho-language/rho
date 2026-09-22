//
// @file deps_sync.go
// @brief Rho Dependency Sync - Enterprise.
//
// Implements `rho deps sync`, the deterministic dependency installer for Rho.
// It resolves all dependencies declared in rho.mod, installs them into
// `.rho-deps/`, and writes a reproducible rho.lock.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"rho/src/deps"
)

/* -------------------------------------------------------------------------- */
/*  rho deps sync                                                             */
/* -------------------------------------------------------------------------- */

func runDepsSync() {
	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps sync:", err)
		os.Exit(1)
	}

	manifestPath := filepath.Join(projectRoot, "rho.mod")
	manifest, err := deps.LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps sync:", err)
		os.Exit(1)
	}

	locator := &deps.DefaultLocator{
		ProjectRoot: projectRoot,
		GlobalCache: filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod"),
		LocalCache:  filepath.Join(projectRoot, ".rho-deps"),
	}

	resolver := &deps.Resolver{Locator: locator}
	_, lock, err := resolver.Resolve(manifest, projectRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps sync:", err)
		os.Exit(1)
	}

	// Write lockfile
	lockPath := filepath.Join(projectRoot, "rho.lock")
	if err := lock.Save(lockPath); err != nil {
		fmt.Fprintln(os.Stderr, "rho deps sync:", err)
		os.Exit(1)
	}

	fmt.Println("Dependencies synchronized:")
	for _, p := range lock.Packages {
		fmt.Printf("  - %s %s (%s)\n", p.Name, p.Version, p.Path)
	}
}
