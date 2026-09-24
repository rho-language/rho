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
	"os/exec"
	"path/filepath"
	util "rho/cmd/rho-install/util"
	install "rho/cmd/rho-install/install"
	pkgcli "rho/cmd/rho-pkg/cli"
	modcli "rho/cmd/rho-module/cli"
	lspserver "rho/cmd/rho-lsp/server"
	src "rho/src"       // compiler orchestrator v3
	deps "rho/src/deps" // dependency system
)

/* -------------------------------------------------------------------------- */
/*  Entry point                                                               */
/* -------------------------------------------------------------------------- */

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: rho <command> [args]")
		return
	}

	// --------------------------------------------------------------------------
	// Verify Rho installation
	// --------------------------------------------------------------------------

	cmd := os.Args[1]

	// Commands that DO NOT require installation
	if cmd == "help" || cmd == "--help" || cmd == "-h" || cmd == "install" {
    	// skip installation check
		if cmd == "--help" || cmd == "-h" {
			cmd = "help"
		}
	} else {
    	// All other commands require Rho to be installed
    	if !install.IsInstalled() {
        	fmt.Println("Rho is not installed.")
        	fmt.Println("Run: rho install")
        	os.Exit(1)
    	}
	}

	switch cmd {

	case "run":
		runCLI(os.Args[2:])

	case "build":
		os.Exit(buildCLI(os.Args[2:]))

	case "help":
		printHelp()

	case "lsp":
		os.Exit(lspCLI(os.Args[2:]))

	case "test":
		os.Exit(testCLI(os.Args[2:]))

	case "pkg":
		os.Exit(pkgcli.Main(os.Args[2:]))

	case "module":
		os.Exit(modcli.Main(os.Args[2:]))

	case "install":
	    os.Exit(installCLI(os.Args[2:]))

	default:
		fmt.Println("Unknown command:", cmd)
		fmt.Println("Use: rho help")
		os.Exit(1)
	}
}

func buildRhoExecutables() error {
    binaries := []struct {
        name string
        path string
    }{
        {"rho", "./cmd/rho"},
        {"rho-fmt", "./cmd/rho-fmt"},
        {"rho-test", "./cmd/rho-test"},
        {"rho-deps", "./cmd/rho-deps"},
        {"rho-doc", "./cmd/rho-doc"},
    }

    if err := os.MkdirAll("./bin", 0o755); err != nil {
        return fmt.Errorf("cannot create ./bin: %w", err)
    }

    for _, b := range binaries {
        cmd := exec.Command("go", "build", "-o", filepath.Join("bin", b.name), b.path)
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr

        if err := cmd.Run(); err != nil {
            return fmt.Errorf("failed to build %s: %w", b.name, err)
        }
    }

    return nil
}


func installCLI(args []string) int {
    fmt.Println("Installing Rho ecosystem...")

	force := false
    if len(args) == 1 && args[0] == "--force" {
        force = true
    }

	if force {
	    home, err := install.GetRhoHome()
   	 if err != nil {
    	    fmt.Println("rho install:", err)
       	 return 1
   	 }
   	 os.RemoveAll(home)
	}

    steps := []struct {
        name string
        fn   func() error
    }{
        {"Creating directory structure", install.EnsurePaths},
		{"Building Rho executables", buildRhoExecutables},
        {"Installing binaries", func() error {
            // Binaries are expected to be in ./bin relative to rho
            return install.InstallAllBinaries("./bin")
        }},
        {"Initializing registry", install.InitLocalRegistry},
        {"Initializing cache", install.InitCache},
        {"Initializing config", install.InitConfig},
        {"Adding Rho to PATH", install.AddRhoToPath},
    }

    for _, step := range steps {
        fmt.Printf(" - %s...\n", step.name)
        if err := step.fn(); err != nil {
            fmt.Printf("   ERROR: %v\n", err)
            return 1
        }
    }

    fmt.Println("Rho installation complete.")
	fmt.Println()
	fmt.Println("To activate Rho in your current shell, run:")
	switch util.DetectShell() {
		case "bash":
    		fmt.Println("  source ~/.bashrc")
		case "zsh":
    		fmt.Println("  source ~/.zshrc")
		case "fish":
    		fmt.Println("  source ~/.config/fish/config.fish")
		default:
    		fmt.Println("  source ~/.profile")
	}
	fmt.Println()

    return 0
}


func testCLI(args []string) int {
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "Usage: rho test [path]")
		return 2
	}
	path := "."
	if len(args) == 1 {
		path = args[0]
	}
	command := exec.Command("go", append([]string{"run", "./cmd/rho-test", path}, args[1:]...)...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	command.Dir, _ = os.Getwd()
	if rho, err := filepath.Abs(filepath.Join(command.Dir, "rho")); err == nil {
		if _, statErr := os.Stat(rho); statErr == nil {
			command.Env = append(os.Environ(), "RHO_BIN="+rho)
		}
	}
	if err := command.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		return 1
	}
	return 0
}

func lspCLI(args []string) int {
	// Allow --stdio, -stdio, or empty args (ignore unexpected flags gracefully)
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			fmt.Fprintln(os.Stderr, "Usage: rho lsp [--stdio]")
			return 0
		}
		// Reject unexpected arguments
		fmt.Fprintln(os.Stderr, "Usage: rho lsp [--stdio]")
		return 2
	}

	// Start the LSP server
	if err := lspserver.New().Start(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "rho lsp:", err)
		return 1
	}
	return 0
}

/* -------------------------------------------------------------------------- */
/*  rho run                                                                   */
/* -------------------------------------------------------------------------- */

func runCLI(args []string) {
	if len(args) != 1 {
		fmt.Println("Usage: rho run <file.rho|project-dir>")
		os.Exit(2)
	}

	file := args[0]
	var dependencyRoots []string
	if info, err := os.Stat(file); err == nil && info.IsDir() {
		projectRoot, absErr := filepath.Abs(file)
		if absErr != nil {
			fmt.Fprintln(os.Stderr, "rho run:", absErr)
			os.Exit(1)
		}
		manifest, manifestErr := deps.LoadManifest(filepath.Join(projectRoot, "rho.mod"))
		if manifestErr != nil || manifest.Main == "" {
			if manifestErr == nil {
				manifestErr = fmt.Errorf("manifest does not declare main entry")
			}
			fmt.Fprintln(os.Stderr, "rho run:", manifestErr)
			os.Exit(1)
		}
		file = manifest.Main
		if !filepath.IsAbs(file) {
			file = filepath.Join(projectRoot, file)
		}
	}
	abs, _ := filepath.Abs(file)
	projectRoot := src.ProjectRoot(abs)
	if manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot)); err == nil {
		locator := &deps.DefaultLocator{
			ProjectRoot: projectRoot,
			GlobalCache: filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod"),
			LocalCache:  filepath.Join(projectRoot, ".rho-deps"),
		}
		graph, _, resolveErr := (&deps.Resolver{Locator: locator}).Resolve(manifest, projectRoot)
		if resolveErr != nil {
			fmt.Fprintln(os.Stderr, "rho run:", resolveErr)
			os.Exit(1)
		}
		for _, node := range graph.Nodes {
			if node.Type == deps.DepTypeRho {
				dependencyRoots = append(dependencyRoots, node.Path)
			}
		}
	}

	if err := RunFileWithDependencies(abs, dependencyRoots); err != nil {
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

	if fs.NArg() > 1 {
		fmt.Println("Usage: rho build [file.rho] [-o out.c]")
		return 2
	}

	in := ""
	projectArg := ""
	if fs.NArg() == 1 {
		candidate := fs.Arg(0)
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			projectArg, _ = filepath.Abs(candidate)
			manifest, manifestErr := deps.LoadManifest(filepath.Join(projectArg, "rho.mod"))
			if manifestErr != nil || manifest.Main == "" {
				if manifestErr == nil {
					manifestErr = fmt.Errorf("manifest does not declare main entry")
				}
				fmt.Fprintln(os.Stderr, "rho build:", manifestErr)
				return 1
			}
			in = manifest.Main
			if !filepath.IsAbs(in) {
				in = filepath.Join(projectArg, in)
			}
		} else {
			in = candidate
		}
	} else {
		projectRoot, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho build:", err)
			return 1
		}
		manifest, err := deps.LoadManifest(filepath.Join(projectRoot, "rho.mod"))
		if err != nil || manifest.Main == "" {
			if err == nil {
				err = fmt.Errorf("manifest does not declare main entry")
			}
			fmt.Fprintln(os.Stderr, "rho build:", err)
			return 1
		}
		in = filepath.Join(projectRoot, manifest.Main)
	}
	absIn, _ := filepath.Abs(in)

	/* ---------------------------------------------------------------------- */
	/*  1. Load manifest                                                      */
	/* ---------------------------------------------------------------------- */

	projectRoot := src.ProjectRoot(absIn)
	if projectArg != "" {
		projectRoot = projectArg
	}
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
	var rhoDependencyRoots []string

	for _, node := range graph.Nodes {
		if node.Type == deps.DepTypeRho {
			rhoDependencyRoots = append(rhoDependencyRoots, node.Path)
		}
		// C deps only
		if node.Type == deps.DepTypeC {
			includePaths = append(includePaths, node.IncludeDirs...)
			sourceFiles = append(sourceFiles, node.SourceFiles...)
		}
	}

	/* ---------------------------------------------------------------------- */
	/*  5. Compile Rho → C                                                    */
	/* ---------------------------------------------------------------------- */

	var outC string
	if *out != "" {
		outC, _ = filepath.Abs(*out)
	} else if projectArg != "" {
		outC = filepath.Join(projectRoot, "build", "main.c")
	} else {
		outC = replaceExt(absIn, ".c")
	}

	if err := src.CompileFileToWithDependencies(absIn, outC, rhoDependencyRoots); err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}

	generatedDir := filepath.Join(projectRoot, "build", "generated")
	if err := src.GenerateProjectWithDependencies(absIn, generatedDir, includePaths, sourceFiles, rhoDependencyRoots); err != nil {
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
	fmt.Println("  rho install           	   Install Rho programming language")
	fmt.Println("  rho pkg <command>           Package management (init, pack, unpack, inspect)")
	fmt.Println("  rho module <command>        Module management (init, validate, info, publish, download, install, remove, update, search)")
	fmt.Println("  rho lsp                     Start the Rho language server")
	fmt.Println("  rho test [path]             Run Rho tests")
	fmt.Println("  rho help                    Show this help")
}
