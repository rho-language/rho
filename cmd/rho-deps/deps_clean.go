//
// @file deps_clean.go
// @brief Rho Dependency Cleaner - Enterprise.
//
// Implements `rho deps clean`, a deterministic cleanup tool for Rho 0.1.
// It removes `.rho-deps/` and `rho.lock`.
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
)

/* -------------------------------------------------------------------------- */
/*  rho deps clean                                                            */
/* -------------------------------------------------------------------------- */

func runDepsClean() {
	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps clean:", err)
		os.Exit(1)
	}

	localCache := filepath.Join(projectRoot, ".rho-deps")
	lockPath := filepath.Join(projectRoot, "rho.lock")

	// Remove .rho-deps
	if err := os.RemoveAll(localCache); err != nil {
		fmt.Fprintln(os.Stderr, "rho deps clean:", err)
		os.Exit(1)
	}

	// Remove rho.lock
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "rho deps clean:", err)
		os.Exit(1)
	}

	fmt.Println("Cleaned dependency state:")
	fmt.Println("  - removed .rho-deps/")
	fmt.Println("  - removed rho.lock")
}
