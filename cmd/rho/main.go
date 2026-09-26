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
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	install "rho/cmd/rho-install/install"
	util "rho/cmd/rho-install/util"
	modcli "rho/cmd/rho-module/cli"
	modcore "rho/cmd/rho-module/core"
	pkgcli "rho/cmd/rho-pkg/cli"

	modremote "rho/cmd/rho-module/remote"
	src "rho/src"       // compiler orchestrator v3
	deps "rho/src/deps" // dependency system
	"rho/src/rhotest"   // test runner
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

	case "test":
		os.Exit(testCLI(os.Args[2:]))

	case "config":
		os.Exit(configCLI(os.Args[2:]))

	case "pkg":
		os.Exit(pkgcli.Main(os.Args[2:]))

	case "module":
		os.Exit(modcli.Main(os.Args[2:]))

	case "install":
		os.Exit(installCLI(os.Args[2:]))

	default:
		if !runDynamicModule(cmd, os.Args[2:]) {
			fmt.Println("Unknown command:", cmd)
			fmt.Println("Use: rho help")
			os.Exit(1)
		}
	}
}

func runDynamicModule(cmd string, args []string) bool {
	// First, try to find a module with matching command
	registryPath := filepath.Join(os.Getenv("HOME"), ".rho", "registry", "local", "registry.yaml")

	// Read registry to find modules with this command
	if _, err := os.Stat(registryPath); err == nil {
		// Parse registry to find module with matching command
		// We need to check each installed module's manifest for the command field
		modulesDir := filepath.Join(os.Getenv("HOME"), ".rho", "modules", "installed")

		for _, moduleType := range []string{"component", "plugin"} {
			typeDir := filepath.Join(modulesDir, moduleType)
			if _, err := os.Stat(typeDir); err != nil {
				continue
			}

			entries, err := os.ReadDir(typeDir)
			if err != nil {
				continue
			}

			for _, e := range entries {
				if !e.IsDir() {
					continue
				}

				modPath := filepath.Join(typeDir, e.Name(), "module.yml")
				manifest, err := modcore.LoadModuleYAML(modPath)
				if err != nil {
					continue
				}

				// Check if this module's command matches
				if manifest.Module.Command == cmd {
					// Found it! Load and run
					mod, err := modcore.LoadInstalledModule(manifest.Module.Name)
					if err != nil {
						return false
					}

					entry := mod.Entry()
					if entry == nil {
						fmt.Fprintln(os.Stderr, "rho:", "module has no entrypoint:", manifest.Module.Name)
						os.Exit(1)
					}

					if err := entry.Run(args, os.Stdin, os.Stdout); err != nil {
						fmt.Fprintln(os.Stderr, "rho:", err)
						os.Exit(1)
					}

					return true
				}
			}
		}
	}

	// Fallback: try to load by module name (for backwards compatibility)
	mod, err := modcore.LoadInstalledModule(cmd)
	if err != nil {
		return false
	}

	entry := mod.Entry()
	if entry == nil {
		fmt.Fprintln(os.Stderr, "rho:", "module has no entrypoint:", cmd)
		os.Exit(1)
	}

	if err := entry.Run(args, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "rho:", err)
		os.Exit(1)
	}

	return true
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

// configCLI reads and writes the remote registry configuration, so a
// deployment can be pointed at without editing files by hand.
func configCLI(args []string) int {
	if len(args) == 0 {
		fmt.Println("Usage: rho config get | rho config set <key> <value>")
		return 2
	}

	switch args[0] {
	case "get":
		cfg, err := modremote.LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho config:", err)
			return 1
		}
		fmt.Println("remote.url      " + cfg.Remote.URL)
		// The token is a secret, so only whether one is set is shown.
		if cfg.Remote.Token == "" {
			fmt.Println("remote.token    (not set)")
		} else {
			fmt.Println("remote.token    (set)")
		}
		return 0

	case "set":
		if len(args) != 3 {
			fmt.Println("Usage: rho config set <remote.url|remote.token> <value>")
			return 2
		}
		var err error
		switch args[1] {
		case "remote.url":
			err = modremote.SetRemoteRegistryURL(args[2])
		case "remote.token":
			err = modremote.SetRemoteRegistryToken(args[2])
		default:
			fmt.Println("Usage: rho config set <remote.url|remote.token> <value>")
			return 2
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho config:", err)
			return 1
		}
		fmt.Println("Set", args[1])
		return 0
	}

	fmt.Println("Usage: rho config get | rho config set <key> <value>")
	return 2
}

func testCLI(args []string) int {
	if len(args) > 2 {
		fmt.Fprintln(os.Stderr, "Usage: rho test [path] [target]")
		return 2
	}

	// The runner lives in src/rhotest and is called in process, so there is no
	// separate test binary to locate. The only thing still resolved is this
	// rho, which the runner needs in order to build the tests.
	options := rhotest.Options{Path: argOrCwd(args), Target: rhotest.DefaultTarget}
	if len(args) == 2 {
		options.Target = args[1]
	} else if len(args) == 1 && isTargetName(args[0]) {
		// A lone target name, as in `rho test go`, tests the project in the
		// working directory on that backend.
		if _, statErr := os.Stat(args[0]); statErr != nil {
			options.Path = "."
			options.Target = args[0]
		}
	}
	if rho, err := os.Executable(); err == nil {
		if _, statErr := os.Stat(rho); statErr == nil {
			options.RhoBinary = rho
		}
	}
	return rhotest.Run(options)
}

/* -------------------------------------------------------------------------- */
/*  rho run                                                                   */
/* -------------------------------------------------------------------------- */

func runCLI(args []string) {
	if len(args) > 1 {
		fmt.Println("Usage: rho run [file.rho|project-dir]")
		os.Exit(2)
	}

	// The argument may be a source file, a project directory, or omitted to
	// run the project in the working directory.
	abs, projectRoot, entryErr := src.ResolveEntryPoint(argOrCwd(args))
	if entryErr != nil {
		fmt.Fprintln(os.Stderr, "rho run:", entryErr)
		os.Exit(1)
	}
	var dependencyRoots []string
	var includePaths, sourceFiles []string
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

	// A Go or server target is a real project with its own toolchain, so it is
	// built and run rather than compiled to a native binary.
	if runGeneratedTarget(abs, projectRoot, dependencyRoots, includePaths, sourceFiles, nil) {
		return
	}

	if err := RunFileWithDependencies(abs, dependencyRoots); err != nil {
		fmt.Fprintln(os.Stderr, "rho run:", err)
		os.Exit(1)
	}
}

// runGeneratedTarget builds and runs a generated project with its own
// toolchain, which is how a Go or server target is executed. It reports
// whether it handled the run.
func runGeneratedTarget(abs, projectRoot string, dependencyRoots, includePaths, sourceFiles []string, passthrough []string) bool {
	descriptor, err := src.ResolveDescriptor(projectRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho run:", err)
		os.Exit(1)
	}

	targets, err := src.GenerateAllTargetsDetailed(abs, "", includePaths, sourceFiles, dependencyRoots, "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho run:", err)
		os.Exit(1)
	}

	// The first Go target is the one to run: it is the one the manifest asked
	// for first.
	for _, target := range targets {
		if target.Emitter != "go" {
			continue
		}
		buildDir := filepath.Join(projectRoot, descriptor.BuildRoot())
		binary, err := src.BuildGoProject(target.Dir, buildDir, src.TargetOutputName(src.ProjectName(abs), target.ID))
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho run:", err)
			os.Exit(1)
		}
		runBinary(binary, buildDir, passthrough)
		return true
	}
	return false
}

// isTargetName reports whether a word names a target rather than a path.
func isTargetName(word string) bool {
	switch word {
	case "c", "go", "go-server", "riscv", "wasm":
		return true
	default:
		return false
	}
}

// argOrCwd returns the command's path argument, defaulting to the working
// directory so `rho build` and `rho run` work from inside a project.
func argOrCwd(args []string) string {
	if len(args) > 0 && args[0] != "" {
		return args[0]
	}
	return "."
}

// runBinary executes a built binary in its own directory, forwarding the
// process streams and exiting with its status.
func runBinary(binary, dir string, args []string) {
	cmd := commandRunner(binary, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
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
	// target restricts the build to a single descriptor target, which is how
	// a test module asks for exactly the backend it can run.
	target := fs.String("target", "", "build only this target (c, go, ...)")
	fs.Parse(args)

	if fs.NArg() > 1 {
		fmt.Println("Usage: rho build [file.rho|project-dir] [-o out.c] [-target id]")
		return 2
	}

	// The argument may be a source file, a project directory, or omitted to
	// build the project in the working directory.
	entry, projectRoot, entryErr := src.ResolveEntryPoint(fs.Arg(0))
	if entryErr != nil {
		fmt.Fprintln(os.Stderr, "rho build:", entryErr)
		return 1
	}
	// ResolveEntryPoint already handled a directory or an omitted argument.
	in := entry

	absIn := in
	if !filepath.IsAbs(absIn) {
		absIn, _ = filepath.Abs(absIn)
	}

	/* ---------------------------------------------------------------------- */
	/*  1. Load manifest                                                      */
	/* ---------------------------------------------------------------------- */

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
	} else {
		// A project keeps every generated artefact under build/; only a
		// standalone source file is compiled next to itself.
		outC = src.ProjectOutputPath(absIn)
	}

	// The descriptor decides where generated projects live, and which targets
	// were actually produced. Generation runs first so the native compile step
	// can follow the targets the project asked for.
	targets, err := src.GenerateAllTargetsDetailed(absIn, "", includePaths, sourceFiles, rhoDependencyRoots, *target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}

	moduleName := src.ProjectName(absIn)
	generated := map[string]bool{}
	for _, target := range targets {
		rel, relErr := filepath.Rel(projectRoot, target.Dir)
		if relErr != nil {
			rel = target.Dir
		}
		fmt.Println("Generated project:", filepath.ToSlash(rel))
		generated[target.ID] = true
	}

	/* ---------------------------------------------------------------------- */
	/*  6. Build each generated project                                        */
	/* ---------------------------------------------------------------------- */

	// A Go target is a self-contained project, so it is compiled with its own
	// toolchain rather than with the native step below.
	descriptor, descErr := src.ResolveDescriptor(projectRoot)
	if descErr != nil {
		fmt.Fprintln(os.Stderr, "rho build:", descErr)
		return 1
	}
	built, err := src.BuildGeneratedTargets(targets, moduleName, filepath.Join(projectRoot, descriptor.BuildRoot()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}
	for _, binary := range built {
		rel, relErr := filepath.Rel(projectRoot, binary)
		if relErr != nil {
			rel = binary
		}
		fmt.Println("Built binary:", filepath.ToSlash(rel))
	}

	/* ---------------------------------------------------------------------- */
	/*  7. Compile the native target                                           */
	/* ---------------------------------------------------------------------- */

	// A project that only asks for a Go or server target has no native build
	// step; the generated project above is the whole deliverable.
	if !generated["c"] {
		return 0
	}

	if err := src.CompileFileToWithDependencies(absIn, outC, rhoDependencyRoots); err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}

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
