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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	install "rho/cmd/rho-install/install"
	modcli "rho/cmd/rho-module/cli"
	modcore "rho/cmd/rho-module/core"
	pkgcli "rho/cmd/rho-pkg/cli"
	"strings"
	"time"

	modremote "rho/cmd/rho-module/remote"
	src "rho/src" // compiler orchestrator v3
	clicore "rho/src/cli"
	deps "rho/src/deps" // dependency system
	"rho/src/schema"
)

/* -------------------------------------------------------------------------- */
/*  Entry point                                                               */
/* -------------------------------------------------------------------------- */

func main() {
	// A component is a separate process, so it needs to be told which rho
	// invoked it. Without this a component can pick up a different, possibly
	// older, rho from PATH and fail on a flag this one has.
	if os.Getenv("RHO_BIN") == "" {
		if self, err := os.Executable(); err == nil {
			_ = os.Setenv("RHO_BIN", self)
		}
	}

	// The module client presents a token issued by `rho auth login` when one
	// is stored, so the two ways of configuring a registry stay in step.
	modremote.SetTokenReader(issuedToken)

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
		// Every other command needs an installation, because every other command
		// reads or writes the local registry: it is where an installed module
		// lives, and it is the only place the toolchain dispatches to or resolves
		// a Rho import against. Without it there is nothing to dispatch to, so the
		// command is refused with the one thing that fixes it rather than with a
		// failure from deeper in.
		if !install.IsInstalled() {
			fmt.Println("The local registry is not installed.")
			fmt.Printf("It is where installed modules live, and every command but help needs it.\n")
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

	case "config":
		os.Exit(configCLI(os.Args[2:]))

	case "pkg":
		os.Exit(pkgcli.Main(os.Args[2:]))

	case "module":
		os.Exit(modcli.Main(os.Args[2:]))

	// Plugins are modules, so `rho module enable` is where one is turned on;
	// `rho plugin` is a shorter way of saying the same thing for the case where
	// the plugins are what the question is about.
	case "plugin":
		os.Exit(modcli.Main(append([]string{"plugin"}, os.Args[2:]...)))

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

// runDynamicModule dispatches a command a module published, so a component is
// reachable by the name it installed itself under.
func runDynamicModule(cmd string, args []string) bool {
	// First, try to find a module with matching command
	rhoHome := deps.RhoHome()
	registryPath := filepath.Join(rhoHome, "registry", "local", "registry.yaml")

	// Read registry to find modules with this command
	if _, err := os.Stat(registryPath); err == nil {
		// Parse registry to find module with matching command
		// We need to check each installed module's manifest for the command field
		modulesDir := filepath.Join(rhoHome, "modules", "installed")

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
					// Found it. A module that is installed and not enabled is
					// not run: a command that reached a component nobody
					// enabled is a component that is in use without anyone
					// having decided it should be. The refusal names the command
					// that decides, because the reader is looking at a failure
					// and the fix is one command away.
					notEnabled, err := deps.NotEnabledHere(moduleType, manifest.Module.Name, e.Name())
					if err != nil {
						fmt.Fprintln(os.Stderr, "rho:", err)
						os.Exit(1)
					}
					if notEnabled != "" {
						fmt.Fprintln(os.Stderr, "rho:", notEnabled)
						os.Exit(1)
					}

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
		// The core toolchain only. The formatter, the test runner, the logger
		// and the credential manager are components: separate modules with
		// their own go.mod, published like any other module and dispatched to
		// at run time. Building them here would put them back in the core and
		// defeat the point of late binding.
		//
		// rho-lsp is an exception: it is built from source during install so
		// a standalone deployment carries the LSP without needing a registry.
		{"rho", "./cmd/rho"},
		{"rho-deps", "./cmd/rho-deps"},
		{"rho-lsp", "./modules/components/rho-lsp/src"},
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
	force := false
	update := false
	for _, arg := range args {
		switch arg {
		case "--force":
			force = true
		case "--update", "-u":
			update = true
		default:
			fmt.Printf("rho install: unknown option %q\n", arg)
			fmt.Println("usage: rho install [--force] [--update]")
			return 1
		}
	}

	if update {
		// Updating is what a developer does after changing the toolchain, and it
		// is the reason a machine ends up running yesterday's `rho` while the
		// source in front of it is today's.
		//
		// It replaces the toolchain and nothing else. The registry, the installed
		// modules, the module cache, the credentials and the configuration all
		// belong to the user, and an update that touched them would be an update
		// that could lose work.
		return updateCLI()
	}

	if force {
		// --force is the destructive one, and it is destructive on purpose: it
		// takes the installation away and puts it back, so that a machine which
		// has been changed by hand is returned to a state where every part of it
		// came from one install. Everything the user has — what they published,
		// what they fetched, who they logged in as — goes with it, which is why
		// it is a flag and not the default.
		home, err := install.GetRhoHome()
		if err != nil {
			fmt.Println("rho install:", err)
			return 1
		}
		fmt.Printf("Removing %s and everything in it...\n", home)
		if err := os.RemoveAll(home); err != nil {
			fmt.Println("rho install:", err)
			return 1
		}
	}

	fmt.Println("Installing Rho ecosystem...")
	for _, step := range installSteps() {
		fmt.Printf(" - %s...\n", step.name)
		if err := step.fn(); err != nil {
			fmt.Printf("   ERROR: %v\n", err)
			return 1
		}
	}
	fmt.Println("Rho installation complete.")
	return 0
}

// updateCLI replaces the toolchain in place.
//
// The order is the whole of it: the binaries are built first and copied over
// second. An install that removed them first and then failed to build left a
// machine with no toolchain at all and no way back — which is why building
// happens before anything is replaced, here and in a full install alike.
// installBinaries puts the built toolchain in place, replacing what is there if
// there is anything.
func installBinaries() error {
	if install.IsInstalled() {
		return install.UpdateBinaries("./bin")
	}
	return install.InstallAllBinaries("./bin")
}

func updateCLI() int {
	steps := installSteps()
	for _, step := range steps {
		if !step.updates {
			continue
		}
		fmt.Printf(" - %s...\n", step.name)
		if err := step.fn(); err != nil {
			fmt.Printf("   ERROR: %v\n", err)
			fmt.Println("   The installed toolchain has been left as it was.")
			return 1
		}
	}
	fmt.Println("Toolchain updated.")
	return 0
}

// installStep is one thing an install does, and whether an update does it too.
type installStep struct {
	name string
	fn   func() error
	// updates is whether this step is part of an update. A step that writes the
	// user's registry, credentials or modules is not one.
	updates bool
}

// installSteps is everything an install does, in the order it does it.
//
// Everything that writes the toolchain is marked as an update's business, and
// everything that writes the user's data is not.
func installSteps() []installStep {
	return []installStep{
		{"Creating directory structure", install.EnsurePaths, true},
		{"Installing the standard library", func() error { return install.InstallStd(".") }, true},
		{"Installing the documentation", func() error { return install.InstallDocs(".") }, true},
		{"Building Rho executables", buildRhoExecutables, true},
		// The binaries are what an update replaces, and what an install puts in
		// place for the first time. Which of the two is happening is the only
		// difference between them, and it is decided by whether there is already
		// a toolchain to replace.
		{"Installing binaries", installBinaries, true},
		// The registry, the cache and the configuration hold what the user has
		// done — what they published, what they fetched, who they logged in as.
		// An update does not touch them, because a toolchain is not theirs to
		// reset and nothing about a new binary makes their data stale.
		{"Registering built-in components", func() error { return install.RegisterBuiltinComponents(".") }, true},
		{"Initializing registry", install.InitLocalRegistry, false},
		{"Initializing cache", install.InitCache, false},
		{"Initializing config", install.InitConfig, false},
		{"Adding Rho to PATH", install.AddRhoToPath, false},
	}
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

/* -------------------------------------------------------------------------- */
/*  configCLI                                                                  */
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
	if manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot)); err == nil && manifest != nil {
		// The C a project compiles is named in its manifest: the shim a
		// generated binding needs, and the host C a Rho program writes for the
		// things the language has no type for.
		sourceFiles = append(sourceFiles, manifest.BuildSources...)
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

// issuedToken returns the credential the auth component stored for a registry.
//
// The file is read here rather than through the component, because the core must
// not depend on a component: that is the whole point of late binding. The
// format is therefore a published contract the auth component owns, and this is
// the only place in the core that knows it.
func issuedToken(registry string) string {
	home := deps.RhoHome()
	if home == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, "auth", "credentials.json"))
	if err != nil {
		return ""
	}
	var file struct {
		Credentials []struct {
			Registry  string    `json:"registry"`
			Method    string    `json:"method"`
			Secret    string    `json:"secret"`
			ExpiresAt time.Time `json:"expires_at"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return ""
	}
	for _, credential := range file.Credentials {
		if credential.Registry != registry {
			continue
		}
		if !credential.ExpiresAt.IsZero() && !time.Now().Before(credential.ExpiresAt) {
			return ""
		}
		return credential.Secret
	}
	return ""
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
	// Everything a build generates is disposable by construction, so removing it
	// is always safe. That is worth a command rather than `rm -rf build`: a
	// generated tree holds CMake caches with absolute paths in them, and a build
	// directory half-removed by hand fails in ways that read like the toolchain is
	// at fault.
	clean := fs.Bool("clean", false, "remove this project's build directory and stop")
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
	/*  2. Remove the previous build, when asked                              */
	/* ---------------------------------------------------------------------- */

	if *clean {
		buildDir := filepath.Join(projectRoot, manifest.BuildDirectory())
		if err := os.RemoveAll(buildDir); err != nil {
			fmt.Fprintln(os.Stderr, "rho build --clean:", err)
			return 1
		}
		fmt.Println("Removed:", filepath.ToSlash(buildDir))
		return 0
	}

	/* ---------------------------------------------------------------------- */
	/* ---------------------------------------------------------------------- */
	/*  3. Resolve dependencies                                               */
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
	/*  4. Write lockfile                                                     */
	/* ---------------------------------------------------------------------- */

	lockPath := filepath.Join(projectRoot, "rho.lock")
	if err := lock.Save(lockPath); err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}

	/* ---------------------------------------------------------------------- */
	/*  5. Collect C include paths + C sources from dependencies              */
	/* ---------------------------------------------------------------------- */

	var includePaths []string
	var sourceFiles []string
	var rhoDependencyRoots []string

	// The C a project compiles is named in its manifest: the shim a generated
	// binding needs, and the host C a Rho program writes for what the language
	// has no type for. The manifest already declared it and the C backend already
	// emitted it into the target; what was missing was passing it on.
	if manifest, manifestErr := deps.LoadManifest(deps.ManifestPath(projectRoot)); manifestErr == nil && manifest != nil {
		sourceFiles = append(sourceFiles, manifest.BuildSources...)
	}

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
	/*  6. Resolve the target, then generate what it declares                 */
	/* ---------------------------------------------------------------------- */

	// The target is resolved before anything is generated, because it decides
	// two separate things: what the foreign interface is generated against, and
	// which generated projects to emit. The interface has to know which board it
	// is for — the sources a preset declares differ between two of them — so this
	// cannot wait until the generation step.
	targetName, descriptorTarget, err := resolveBuildTarget(*target, manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 2
	}

	// Before anything is compiled, because a project that imports an interface
	// this step has yet to write cannot be parsed far enough to say what is
	// missing.
	if err := generateForeignInterface(projectRoot, targetName); err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}
	// The interface was just written there, and a project that imports it is
	// asking for a module the compiler has to be able to find. It is a
	// dependency root for this build only: it holds generated code, so it is
	// read as a module and nothing else is expected of it.
	//
	// Absolute, because a dependency root is joined onto a module name and
	// stat'ed, and a relative one would be read against the working directory —
	// which is the directory a person happened to run `rho build` from, not the
	// project being built.
	generatedModules := filepath.Join(manifest.BuildDirectory(), "generated", "modules")
	if !filepath.IsAbs(generatedModules) {
		generatedModules = filepath.Join(projectRoot, generatedModules)
	}
	rhoDependencyRoots = append(rhoDependencyRoots, generatedModules)

	/* ---------------------------------------------------------------------- */
	/*  7. Compile Rho → C                                                    */
	/* ---------------------------------------------------------------------- */

	// What this project compiles beyond its own sources: the shim the foreign
	// declarations call through, and the vendor sources the selected preset
	// declares — the startup assembly and the system file, for a board.
	//
	// Both were written by steps that have already run, so they are read from
	// where those put them rather than worked out again.
	generatedTree := filepath.Join(projectRoot, manifest.BuildDirectory(), "generated")
	// What the configuration resolved to, resolved here rather than at build time.
	//
	// A driver belongs to a peripheral, and whether a program uses it is the
	// program's answer. That answer has to be known before the sources are chosen,
	// so the merge runs here: a build that decided it later would have a source
	// list already written. The merge still runs again in the build, because a
	// header that can change without regenerating the project is worth having.
	configured, configErr := resolvedConfiguration(projectRoot, manifest, targetName)
	if configErr != nil {
		fmt.Fprintln(os.Stderr, "rho build:", configErr)
		return 1
	}
	// The configuration is resolved here so that the sources can be chosen; the
	// generator reads the result from the same place.
	_ = configured

	shimSources := []string{}
	for _, candidate := range []string{
		filepath.Join(generatedTree, "c", "ffi_shim.c"),
		filepath.Join(generatedTree, "modules", "preset-sources.txt"),
	} {
		if _, statErr := os.Stat(candidate); statErr == nil {
			shimSources = append(shimSources, candidate)
		}
	}
	shimSources = append(shimSources, readLines(filepath.Join(generatedTree, "modules", "preset-sources.txt"))...)
	// The board's own sources, for the same reason: they were declared by the board
	// and read by the step that resolved the board, and nothing compiled them.
	shimSources = append(shimSources, readLines(filepath.Join(generatedTree, "modules", "preset-sources-board-sources.txt"))...)

	// The include directories the selected preset declares, read from where the
	// step that resolved the preset recorded them. A vendor source compiled
	// without the headers it includes cannot be built, and the preset is what says
	// which directories of which repository are headers.
	includePaths = append(includePaths, readLines(filepath.Join(generatedTree, "modules", "preset-sources-includes.txt"))...)
	// The directory the vendor's own configuration header was produced in. The
	// vendor's headers include that header by name, so a build that compiled them
	// without its directory fails on a line that says nothing about the build that
	// could have produced it.
	includePaths = append(includePaths, readLines(filepath.Join(generatedTree, "modules", "preset-sources-config-includes.txt"))...)

	targets, err := src.GenerateAllTargetsDetailed(absIn, "", includePaths, sourceFiles, rhoDependencyRoots, descriptorTarget, shimSources)
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
	/*  8. Build each generated project                                        */
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
	/*  9. Build the native target                                              */
	/* ---------------------------------------------------------------------- */

	// A project that only asks for a Go or server target has no native build
	// step; the generated project above is the whole deliverable.
	if !generated["c"] {
		return 0
	}

	// The generated C project is a real CMake project, and it is built as one.
	// The transpiled module is not written here. It used to be, next to the CMake
	// project that generates the same thing as its application source, which gave
	// every project two `main.c` files and two answers to what the build is: one
	// written by the generator and one left beside it by this step.
	//
	// What `-o` names is still honoured — a caller that asked for a translation
	// unit in a particular place gets one there — but a build that generates a
	// project writes the project, and nothing beside it.
	if *out != "" {
		outC, _ := filepath.Abs(*out)
		if err := src.CompileFileToWithDependencies(absIn, outC, rhoDependencyRoots); err != nil {
			fmt.Fprintln(os.Stderr, "rho build:", err)
			return 1
		}
	}

	// The target was resolved above, before generation, because it decides what
	// gets generated as well as what it is built for. A value that named a
	// backend rather than hardware is already accounted for: generation filtered
	// by it, and there is nothing left to build for.
	if descriptorTarget != "" {
		return 0
	}

	// Where the transpiled module went, for the message, and where the build
	// that was actually run is: the generated project's directory, because that
	// is the project that is compiled.
	generatedDir := ""
	for _, t := range targets {
		if t.ID == "c" {
			generatedDir = t.Dir
			break
		}
	}
	if generatedDir == "" {
		fmt.Fprintln(os.Stderr, "rho build: the project generated no C project to build")
		return 1
	}

	if err := buildTarget(generatedDir, projectRoot, targetName); err != nil {
		fmt.Fprintln(os.Stderr, "rho build:", err)
		return 1
	}

	fmt.Println("Build successful:", filepath.Join(generatedDir, "build", targetName))
	return 0
}

// resolveBuildTarget decides what a `-target` value means.
//
// The flag carries two kinds of name and they have to be told apart, because
// both are already in use: a descriptor backend (`c`, `go`) says which
// generated project to emit, and a target (`native`, `stm32f4`, a board) says
// what to build it for. The descriptor's backends are a closed list this file
// knows; a target's names belong to rho-target, which is a component and not
// something this toolchain carries a list of.
//
// So the backends win the ones that are theirs, and anything else is a target.
// That is not a guess about which was meant: `c` and `go` are the only names
// that can be a backend, and a target that happens to be called `go` would have
// to be reached by its manifest declaration instead.
func resolveBuildTarget(flagValue string, manifest *deps.Manifest) (target string, descriptor string, err error) {
	if flagValue != "" {
		if isDescriptorBackend(flagValue) {
			return "", flagValue, nil
		}
		return flagValue, "", nil
	}
	// A manifest may declare the target, which is what makes a project built for
	// a board be built for it without anything being passed.
	if manifest != nil && manifest.BuildToolchain != "" {
		return manifest.BuildToolchain, "", nil
	}
	// Nothing named a target: the host. It is named rather than assumed, so that
	// the default is a target somebody can see and override.
	return "native", "", nil
}

// isDescriptorBackend reports whether a name is one of the backends the
// descriptor can emit.
func isDescriptorBackend(name string) bool {
	for _, candidate := range schema.ValidBuildTargets {
		if candidate == name {
			return true
		}
	}
	return false
}

// buildTarget builds a generated project for a target, by handing the work to
// the component that knows what targets are.
//
// It is a component call rather than a library call on purpose. Which compiler
// runs, where a program's memory is and how it is written to a board are
// rho-target's facts and are extended by its plugins, so reaching into it from
// here would make this toolchain carry a copy of a list that a plugin is meant
// to be able to add to. The component is located the same way every other
// command locates one, which also means a target nobody enabled is refused here
// for the same reason it is refused everywhere else.
func buildTarget(generatedDir, projectRoot, target string) error {
	module, err := modcore.LoadInstalledModule("rho-target")
	if err != nil {
		return fmt.Errorf("this build needs the rho-target component: %w", err)
	}
	entry := module.Entry()
	if entry == nil {
		return fmt.Errorf("the rho-target component has no entry point")
	}
	// Its output is the build's output, so it goes where this process's output
	// goes rather than being captured and re-printed.
	// `--project` is the generated tree that gets built; `--for` is the
	// application, whose manifest is where the platform that declares the target
	// is named. Passing only the first is what made a platform's own board look
	// like a board that did not exist.
	return entry.Run([]string{
		"build",
		"--target", target,
		"--project", generatedDir,
		"--for", projectRoot,
	}, os.Stdin, os.Stdout)
}

// generateForeignInterface produces the Rho side of a project's C connection
// before anything that imports it is compiled.
//
// A project says it has one by declaring a `c_connection` naming the headers it
// may call. That declaration is the whole of the trigger: there is no feature to
// enable and nothing to be found on disk, so a project either asks for a
// foreign interface or it has none.
//
// It is the cimport component rather than a call into its library, for the same
// reason rho-target is: what a foreign interface is made of is cimport's to
// change and its plugins' to extend, and a copy of it inside this command would
// be a second one that nobody extends.
func generateForeignInterface(projectRoot, preset string) error {
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil {
		// A project with an unreadable manifest was already reported, and it is
		// not this step's to report again.
		return nil
	}
	if manifest.Connection == nil || len(manifest.Connection.Headers) == 0 {
		return nil
	}

	module, err := modcore.LoadInstalledModule("rho-cimport")
	if err != nil {
		return fmt.Errorf("this project declares a C connection, so the build needs the rho-cimport component: %w", err)
	}
	entry := module.Entry()
	if entry == nil {
		return fmt.Errorf("the rho-cimport component has no entry point")
	}
	// The sources the selected preset declares are recorded beside the interface.
	// They are what makes the image runnable — the startup assembly is the entry
	// point — and this is the only step that has read the preset, so this is where
	// the build learns what to compile.
	recorded := filepath.Join(projectRoot, manifest.BuildDirectory(), "generated", "modules", "preset-sources.txt")
	arguments := []string{"generate", "--force", "--record-sources", recorded}
	if preset != "" {
		arguments = append(arguments, "--preset", preset)
	}
	arguments = append(arguments, projectRoot)
	return entry.Run(arguments, os.Stdin, os.Stdout)
}

/* -------------------------------------------------------------------------- */
/*  Help                                                                      */
/* -------------------------------------------------------------------------- */

func printHelp() {
	fmt.Println("Rho Programming Language CLI")
	fmt.Println()

	// What is on offer is read, not written down.
	//
	// The toolchain's own commands are inside this binary and are the same
	// everywhere. A component's command exists only where the component is
	// installed and has been turned on, so a machine with nothing installed is
	// told only what the toolchain can do by itself — which is a shorter list,
	// and the true one.
	core, components := clicore.All()
	width := 0
	for _, command := range core {
		if len(command.Invocation) > width {
			width = len(command.Invocation)
		}
	}
	fmt.Println("Commands:")
	for _, command := range core {
		fmt.Printf("  %-*s  %s\n", width, "rho "+command.Invocation, command.Description)
	}
	if len(components) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("From installed components:")
	width = 0
	for _, command := range components {
		if len(command.Invocation) > width {
			width = len(command.Invocation)
		}
	}
	for _, command := range components {
		fmt.Printf("  %-*s  %s\n", width, "rho "+command.Invocation, command.Description)
	}
}

// readLines reads a file written as one path per line, ignoring a file that is
// not there.
//
// Absent rather than fatal, because the file is written by a step that reports
// its own problems: a build with nothing to list simply has nothing to list, and
// a project that compiles none of them is not a project that failed.
func readLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

/* -------------------------------------------------------------------------- */
/*  Configuration deciding what gets compiled                                   */
/* -------------------------------------------------------------------------- */

// resolvedConfiguration runs the project's configuration merge and returns the
// symbols that ended up set.
//
// It runs here, before the sources are chosen, because that is the only point at
// which a decision can still be made. The generated project runs the merge again
// when it builds, so the header a program is compiled against stays current
// without regenerating anything; what it cannot do there is remove a source from
// a target that already exists.
func resolvedConfiguration(projectRoot string, manifest *deps.Manifest, targetName string) (map[string]bool, error) {
	if manifest == nil || manifest.Config == nil {
		return nil, nil
	}
	buildDir := filepath.Join(projectRoot, manifest.BuildDirectory())
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return nil, err
	}
	out := filepath.Join(buildDir, "config-check")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	merge := exec.Command("rho", "kconfig", "merge", "--quiet",
		"--dialect", manifest.Config.Dialect,
		"--root", projectRoot,
		"--out", out,
		"--force")
	if manifest.Config.ValuesFile != "" {
		// Attached to the flag rather than following it.
		//
		// This program re-reads its own arguments before dispatching, and a value
		// that arrives as its own argument does not survive that. The failure
		// names the flag and not the reason, so it reads as a typo.
		merge.Args = append(merge.Args, "--defconfig="+
			filepath.Join(projectRoot, filepath.FromSlash(manifest.Config.ValuesFile)))
	}
	fragments := configFragments(projectRoot, manifest)
	for _, fragment := range fragments {
		merge.Args = append(merge.Args, fragment)
	}
	if output, err := merge.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("the configuration could not be resolved: %w\n%s", err, output)
	}

	set := map[string]bool{}
	data, err := os.ReadFile(filepath.Join(out, ".config"))
	if err != nil {
		return set, nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		key = strings.TrimPrefix(strings.TrimSpace(key), "CONFIG_")
		set[key] = strings.TrimSpace(value) == "y"
	}
	_ = targetName
	return set, nil
}

// configFragments is every Kconfig file a project and its dependencies declare.
func configFragments(projectRoot string, manifest *deps.Manifest) []string {
	fragments := []string{}
	for _, fragment := range manifest.Config.Fragments {
		path := filepath.Join(projectRoot, filepath.FromSlash(fragment))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fragments = append(fragments, path)
		}
	}
	for _, root := range src.CDependencyChain(projectRoot, nil) {
		dependency, err := deps.LoadManifest(deps.ManifestPath(root))
		if err != nil || dependency == nil || dependency.Config == nil {
			continue
		}
		for _, fragment := range dependency.Config.Fragments {
			path := filepath.Join(root, filepath.FromSlash(fragment))
			if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
				fragments = append(fragments, path)
			}
		}
	}
	return fragments
}

// guardSources removes the sources a module said belong to a symbol that is not
// set.
//
// The rule lives with the module that owns the sources, because only that module
// knows which file implements which driver, and it is matched by file name
// because those files live in a vendor repository that moves.
//
// A driver left out of the configuration is not merely switched off: its
// implementation is not compiled, so it is not in the image. A program that
// wanted it would fail to link, which is the honest outcome — better than a
// binary that carried a driver it never asked for.
func guardSources(sources []string, configured map[string]bool, roots []string) []string {
	guarded := map[string]string{} // file name -> symbol
	for _, root := range roots {
		module, err := deps.LoadManifest(deps.ManifestPath(root))
		if err != nil || module == nil {
			continue
		}
		for _, guard := range module.BuildGuardedSources {
			for _, name := range guard.Sources {
				guarded[filepath.Base(name)] = guard.Config
			}
		}
	}
	if len(guarded) == 0 {
		return sources
	}

	kept := make([]string, 0, len(sources))
	for _, source := range sources {
		symbol, isGuarded := guarded[filepath.Base(source)]
		if isGuarded && !configured[symbol] {
			continue
		}
		kept = append(kept, source)
	}
	return kept
}

// guardedSources collects which symbol governs which source file, from the
// manifests of everything the project depends on.
//
// The rule lives with the module that owns the source, because only that module
// knows which file implements which driver. It is keyed by file name because
// those files live in a vendor repository that moves, and a rule written against
// a path would break the first time it did.
func guardedSources(roots []string) map[string]string {
	guarded := map[string]string{}
	for _, root := range roots {
		module, err := deps.LoadManifest(deps.ManifestPath(root))
		if err != nil || module == nil {
			continue
		}
		for _, guard := range module.BuildGuardedSources {
			for _, name := range guard.Sources {
				guarded[filepath.Base(name)] = guard.Config
			}
		}
	}
	if len(guarded) == 0 {
		return nil
	}
	return guarded
}
