//
// @file deps_status.go
// @brief Rho Dependency Status - Enterprise.
//
// Implements `rho deps status`, a deterministic diff viewer for Rho 0.1.
// It compares rho.mod, rho.lock, and the installed dependencies.
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
/*  rho deps status                                                           */
/* -------------------------------------------------------------------------- */

func runDepsStatus() {
	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps status:", err)
		os.Exit(1)
	}

	manifestPath := filepath.Join(projectRoot, "rho.mod")
	lockPath := filepath.Join(projectRoot, "rho.lock")

	manifest, err := deps.LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps status:", err)
		os.Exit(1)
	}

	lock, err := deps.LoadLockfile(lockPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps status:", err)
		os.Exit(1)
	}

	// Build lookup table
	locked := map[string]deps.LockedPackage{}
	for _, p := range lock.Packages {
		locked[p.Name] = p
	}

	fmt.Println("Dependency status:")
	fmt.Println()

	seen := map[string]bool{}

	// Check manifest deps
	for _, d := range manifest.Deps {
		seen[d.Name] = true

		lp, exists := locked[d.Name]
		if !exists {
			fmt.Printf("MISSING      %s   (not in lockfile)\n", d.Name)
			continue
		}

		// Check version range
		cs, err := deps.ParseVersionRange(d.Version.Raw)
		if err != nil {
			fmt.Printf("INVALID      %s   range=%s\n", d.Name, d.Version.Raw)
			continue
		}
		v, err := deps.ParseSemver(lp.Version)
		if err != nil {
			fmt.Printf("INVALID      %s   lock=%s\n", d.Name, lp.Version)
			continue
		}

		if !deps.VersionInRange(v, cs) {
			fmt.Printf("OUTDATED     %s   lock=%s range=%s\n",
				d.Name, lp.Version, d.Version.Raw)
			continue
		}

		// Check installation
		if _, err := os.Stat(lp.Path); err != nil {
			fmt.Printf("NOT INSTALLED %s   path=%s\n", d.Name, lp.Path)
			continue
		}

		fmt.Printf("OK           %s   %s\n", d.Name, lp.Version)
	}

	// Check extra deps in lockfile
	for name := range locked {
		if !seen[name] {
			fmt.Printf("EXTRA        %s   (in lockfile but not in manifest)\n", name)
		}
	}
}
