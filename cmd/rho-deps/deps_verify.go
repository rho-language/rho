//
// @file deps_verify.go
// @brief Rho Dependency Verification - Enterprise.
//
// Implements `rho deps verify`, a deterministic consistency checker for Rho 0.1.
// It verifies that rho.lock matches rho.mod and that all dependencies exist.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/deps"
)

/* -------------------------------------------------------------------------- */
/*  rho deps verify                                                           */
/* -------------------------------------------------------------------------- */

func runDepsVerify() int {
	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps verify:", err)
		return 1
	}

	manifestPath := filepath.Join(projectRoot, "rho.mod")
	lockPath := filepath.Join(projectRoot, "rho.lock")

	manifest, err := deps.LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps verify:", err)
		return 1
	}

	lock, err := deps.LoadLockfile(lockPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps verify:", err)
		os.Exit(1)
	}

	ok := true

	// Build lookup table
	locked := map[string]deps.LockedPackage{}
	for _, p := range lock.Packages {
		locked[p.Name] = p
	}

	// Verify each manifest dependency
	for _, d := range manifest.Deps {
		lp, exists := locked[d.Name]
		if !exists {
			fmt.Printf("ERR: missing lock entry for %s\n", d.Name)
			ok = false
			continue
		}

		// Check version range
		cs, err := deps.ParseVersionRange(d.Version.Raw)
		if err != nil {
			fmt.Printf("ERR: invalid version range for %s: %s\n", d.Name, d.Version.Raw)
			ok = false
			continue
		}

		dependencyType := d.Type
		if dependencyType == "" {
			dependencyType = deps.DepTypeRho
		}
		if dependencyType == deps.DepTypeRho || lp.Version != "" {
			if v, parseErr := deps.ParseSemver(lp.Version); parseErr != nil {
				if dependencyType == deps.DepTypeRho {
					fmt.Printf("ERR: invalid semver in lockfile for %s: %s\n", d.Name, lp.Version)
					ok = false
				}
			} else if !deps.VersionInRange(v, cs) {
				fmt.Printf("ERR: %s version %s not in range %s\n", d.Name, lp.Version, d.Version.Raw)
				ok = false
			}
		}

		info, statErr := os.Stat(lp.Path)
		if statErr != nil || !info.IsDir() {
			fmt.Printf("ERR: missing dependency path for %s: %s\n", d.Name, lp.Path)
			ok = false
		} else if validSHA256(lp.Hash) && deps.HashPath(lp.Path) != lp.Hash {
			fmt.Printf("ERR: changed dependency contents for %s\n", d.Name)
			ok = false
		}

		if ok {
			fmt.Printf("OK: %s %s\n", d.Name, lp.Version)
		}
	}

	if !ok {
		return 1
	}

	fmt.Println("All dependencies verified.")
	return 0
}

func validSHA256(value string) bool {
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(value) == len("sha256:")+64
}
