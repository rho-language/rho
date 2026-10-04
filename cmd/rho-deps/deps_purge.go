//
// @file deps_purge.go
// @brief Rho Dependency Purge - Enterprise.
//
// Implements `rho deps purge`, the full dependency reset tool for Rho 0.1.
// It removes `.rho-deps/`, `rho.lock`, and the global cache `~/.rho/pkg/mod/`.
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
)

/* -------------------------------------------------------------------------- */
/*  rho deps purge                                                            */
/* -------------------------------------------------------------------------- */

func runDepsPurge() {
	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps purge:", err)
		os.Exit(1)
	}
	home, ok := os.LookupEnv("HOME")
	if !ok || home == "" {
		fmt.Fprintln(os.Stderr, "rho deps purge: HOME is not set; refusing to remove global cache")
		os.Exit(1)
	}

	localCache := filepath.Join(projectRoot, ".rho-deps")
	lockPath := filepath.Join(projectRoot, "rho.lock")
	globalCache := filepath.Join(home, ".rho", "pkg", "mod")

	// Remove local cache
	if err := os.RemoveAll(localCache); err != nil {
		fmt.Fprintln(os.Stderr, "rho deps purge:", err)
		os.Exit(1)
	}

	// Remove lockfile
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "rho deps purge:", err)
		os.Exit(1)
	}

	// Remove global cache
	if err := os.RemoveAll(globalCache); err != nil {
		fmt.Fprintln(os.Stderr, "rho deps purge:", err)
		os.Exit(1)
	}

	fmt.Println("Purged all dependency state:")
	fmt.Println("  - removed .rho-deps/")
	fmt.Println("  - removed rho.lock")
	fmt.Println("  - removed global cache ~/.rho/pkg/mod/")
	fmt.Println()
	fmt.Println("System reset. Run `rho deps sync` to reinstall dependencies.")
}
