//
// @file deps_doctor.go
// @brief Rho Dependency Doctor - Enterprise.
//
// Implements `rho deps doctor`, a full diagnostic tool for Rho 0.1.
// It checks manifest, lockfile, installation paths, version ranges,
// and reports all inconsistencies.
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
/*  rho deps doctor                                                           */
/* -------------------------------------------------------------------------- */

func runDepsDoctor() int {
	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps doctor:", err)
		return 1
	}

	manifestPath := filepath.Join(projectRoot, "rho.mod")
	lockPath := filepath.Join(projectRoot, "rho.lock")

	manifest, err := deps.LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps doctor:", err)
		return 1
	}

	lock, err := deps.LoadLockfile(lockPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps doctor:", err)
		return 1
	}

	// Build lookup table
	locked := map[string]deps.LockedPackage{}
	for _, p := range lock.Packages {
		locked[p.Name] = p
	}

	fmt.Println("Rho Dependency Doctor Report")
	fmt.Println("============================")
	fmt.Println()

	seen := map[string]bool{}
	ok := true

	/* ---------------------------------------------------------------------- */
	/*  Check manifest dependencies                                           */
	/* ---------------------------------------------------------------------- */

	fmt.Println("Manifest → Lockfile consistency:")
	for _, d := range manifest.Deps {
		seen[d.Name] = true

		lp, exists := locked[d.Name]
		if !exists {
			fmt.Printf("  MISSING       %s   (not in lockfile)\n", d.Name)
			ok = false
			continue
		}

		// Check version range
		cs, err := deps.ParseVersionRange(d.Version.Raw)
		if err != nil {
			fmt.Printf("  INVALID       %s   range=%s\n", d.Name, d.Version.Raw)
			ok = false
			continue
		}
		v, err := deps.ParseSemver(lp.Version)
		if err != nil {
			fmt.Printf("  INVALID       %s   lock=%s\n", d.Name, lp.Version)
			ok = false
			continue
		}

		if !deps.VersionInRange(v, cs) {
			fmt.Printf("  VERSION MISMATCH %s   lock=%s range=%s\n",
				d.Name, lp.Version, d.Version.Raw)
			ok = false
			continue
		}

		fmt.Printf("  OK            %s   %s\n", d.Name, lp.Version)
	}

	fmt.Println()

	/* ---------------------------------------------------------------------- */
	/*  Check extra deps in lockfile                                          */
	/* ---------------------------------------------------------------------- */

	fmt.Println("Lockfile → Manifest consistency:")
	for name := range locked {
		if !seen[name] {
			fmt.Printf("  EXTRA         %s   (in lockfile but not in manifest)\n", name)
			ok = false
		}
	}
	fmt.Println()

	/* ---------------------------------------------------------------------- */
	/*  Check installation paths                                              */
	/* ---------------------------------------------------------------------- */

	fmt.Println("Installation paths:")
	for _, lp := range lock.Packages {
		if _, err := os.Stat(lp.Path); err != nil {
			fmt.Printf("  NOT INSTALLED %s   path=%s\n", lp.Name, lp.Path)
			ok = false
		} else {
			fmt.Printf("  OK            %s   %s\n", lp.Name, lp.Path)
		}
	}
	fmt.Println()

	/* ---------------------------------------------------------------------- */
	/*  Final verdict                                                         */
	/* ---------------------------------------------------------------------- */

	if ok {
		fmt.Println("Doctor says: All good. System healthy.")
		return 0
	} else {
		fmt.Println("Doctor says: Issues detected. Run `rho deps sync` or `rho deps update`.")
		return 1
	}
}
