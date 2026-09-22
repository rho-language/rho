//
// @file main.go
// @brief Rho CLI - Enterprise v3.
//
// Top-level CLI for the Rho compiler toolchain.
// Commands:
//   rho run <file.rho>
//   rho build <file.rho> [-o out.c]
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	src "rho/src"       // compiler orchestrator v3
	deps "rho/src/deps" // dependency system
)

/* -------------------------------------------------------------------------- */
/*  Entry point                                                               */
/* -------------------------------------------------------------------------- */

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: rho <command> [args]")
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {

	case "run":
		runCLI(os.Args[2:])

	case "build":
		os.Exit(buildCLI(os.Args[2:]))

	case "help":
		printHelp()

	default:
		fmt.Println("Unknown command:", cmd)
		fmt.Println("Use: rho help")
		os.Exit(1)
	}
}

/* -------------------------------------------------------------------------- */
/*  rho run                                                                   */
/* -------------------------------------------------------------------------- */

func runCLI(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: rho run <file.rho>")
		os.Exit(2)
	}

	file := args[0]
	abs, _ := filepath.Abs(file)

	if err := RunFile(abs); err != nil {
		fmt.Fprintln(os.Stderr, "rho run:", err)
		os.Exit(1)
	}
}

/* -------------------------------------------------------------------------- */
/*  rho build (with full dependency integration)                              */
/* -------------------------------------------------------------------------- */

func buildCLI(args []string) int {
	fs := flag.NewFlagSet("rho build", flag.ExitOnError)
	out := fs.String("o", "", "output C file (optional)")
	fs.Parse(args)

	if fs.NArg() != 1 {
		fmt.Println("Usage: rho build <file.rho> [-o out.c]")
		return 2
	}

	in := fs.Arg(0)
	absIn, _ := filepath.Abs(in)

	/* ---------------------------------------------------------------------- */
	/*  1. Load manifest                                                      */
	/* ---------------------------------------------------------------------- */

	projectRoot := filepath.Dir(absIn)
	manifestPath := filepath.Join(projectRoot, "rho.mod")

	manifest, err := deps.LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}

	/* ---------------------------------------------------------------------- */
	/*  2. Resolve dependencies                                               */
	/* ---------------------------------------------------------------------- */

	locator := &deps.DefaultLocator{
		ProjectRoot: projectRoot,
		GlobalCache: filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod"),
		LocalCache:  filepath.Join(projectRoot, ".rho-deps"),
	}

	resolver := &deps.Resolver{Locator: locator}
	graph, lock, err := resolver.Resolve(manifest, projectRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}

	/* ---------------------------------------------------------------------- */
	/*  3. Write lockfile                                                     */
	/* ---------------------------------------------------------------------- */

	lockPath := filepath.Join(projectRoot, "rho.lock")
	if err := lock.Save(lockPath); err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}

	/* ---------------------------------------------------------------------- */
	/*  4. Collect C include paths + C sources from dependencies              */
	/* ---------------------------------------------------------------------- */

	var includePaths []string
	var sourceFiles []string

	for _, node := range graph.Nodes {
		// C deps only
		if node.Type == deps.DepTypeC {
			inc := filepath.Join(node.Path, "include")
			srcDir := filepath.Join(node.Path, "src")

			if _, err := os.Stat(inc); err == nil {
				includePaths = append(includePaths, inc)
			}

			if _, err := os.Stat(srcDir); err == nil {
				entries, _ := os.ReadDir(srcDir)
				for _, e := range entries {
					if !e.IsDir() && filepath.Ext(e.Name()) == ".c" {
						sourceFiles = append(sourceFiles, filepath.Join(srcDir, e.Name()))
					}
				}
			}
		}
	}

	/* ---------------------------------------------------------------------- */
	/*  5. Compile Rho → C                                                    */
	/* ---------------------------------------------------------------------- */

	var outC string
	if *out != "" {
		outC, _ = filepath.Abs(*out)
	} else {
		outC = replaceExt(absIn, ".c")
	}

	if err := src.CompileFileTo(absIn, outC); err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}

	/* ---------------------------------------------------------------------- */
	/*  6. Compile C → native executable                                      */
	/* ---------------------------------------------------------------------- */

	exe := outC + "_bin"

	argsGCC := []string{"-O2", "-o", exe, outC}

	for _, inc := range includePaths {
		argsGCC = append(argsGCC, "-I"+inc)
	}

	for _, csrc := range sourceFiles {
		argsGCC = append(argsGCC, csrc)
	}

	cmd := commandRunner("gcc", argsGCC...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}

	fmt.Println("Build successful:", exe)
	return 0
}

/* -------------------------------------------------------------------------- */
/*  Help                                                                      */
/* -------------------------------------------------------------------------- */

func printHelp() {
	fmt.Println("Rho Programming Language CLI")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  rho run <file.rho>          Run a Rho program")
	fmt.Println("  rho build <file.rho> [-o]   Compile Rho → C (+ deps)")
	fmt.Println("  rho help                    Show this help")
}
