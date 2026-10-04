//
// @file deps.go
// @brief Rho Dependency Tools - Enterprise.
//
// Implements `rho deps tree`, a deterministic dependency graph viewer
// for Rho 0.1. It loads rho.mod, resolves dependencies, and prints the
// full dependency tree.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"rho/src/deps"
)

/* -------------------------------------------------------------------------- */
/*  CLI Entry Point                                                           */
/* -------------------------------------------------------------------------- */

func main() {
	fs := flag.NewFlagSet("rho deps", flag.ExitOnError)
	fs.Parse(os.Args[1:])

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: rho deps <command>")
		os.Exit(2)
	}

	cmd := fs.Arg(0)

	switch cmd {
	case "tree":
		runDepsTree()

	case "sync":
		runDepsSync()

	case "add":
		runDepsAdd(fs.Args()[1:])

	case "remove":
		runDepsRemove(fs.Args()[1:])

	case "update":
		runDepsUpdate()

	case "verify":
		os.Exit(runDepsVerify())

	case "clean":
		runDepsClean()

	case "status":
		runDepsStatus()

	case "graph":
		runDepsGraph(fs.Args()[1:])

	case "purge":
		runDepsPurge()

	case "doctor":
		os.Exit(runDepsDoctor())

	default:
		fmt.Fprintln(os.Stderr, "unknown deps command:", cmd)
		fmt.Fprintln(os.Stderr, "available: tree, sync, add, remove, update, verify, clean, status, graph, purge, doctor")
		os.Exit(2)
	}
}

/* -------------------------------------------------------------------------- */
/*  rho deps tree                                                             */
/* -------------------------------------------------------------------------- */

func runDepsTree() {
	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps tree:", err)
		os.Exit(1)
	}

	manifestPath := filepath.Join(projectRoot, "rho.mod")
	manifest, err := deps.LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps tree:", err)
		os.Exit(1)
	}

	locator := &deps.DefaultLocator{
		ProjectRoot: projectRoot,
		GlobalCache: filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod"),
		LocalCache:  filepath.Join(projectRoot, ".rho-deps"),
	}

	resolver := &deps.Resolver{Locator: locator}
	graph, _, err := resolver.Resolve(manifest, projectRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps tree:", err)
		os.Exit(1)
	}

	deps.PrintTree(graph)
}
