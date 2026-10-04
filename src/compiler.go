//
// @file compiler.go
// @brief Rho Compiler - Orchestrator v3.
//
// Enterprise v3: top-level Rho 0.1 compiler driver.
// Pipeline: lexer → parser → typechecker → IR → C codegen.
// Exposes a clean API for CLI tools: rho build, rho run.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rho

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"rho/src/codegen"
	"rho/src/deps"
	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
	"rho/src/ir"
	"rho/src/projectgen"
	"rho/src/types"
	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// CompileFile runs the full Rho pipeline on a single source file
// and emits a corresponding .c file next to it.
func CompileFile(path string) error {
	return CompileFileTo(path, replaceExt(path, ".c"))
}

// CompileFileTo compiles one Rho source file into the requested C output path.
func CompileFileTo(path, outPath string) error {
	return CompileFileToWithDependencies(path, outPath, nil)
}

// CompileFileToWithDependencies compiles a source file with additional Rho
// dependency roots available to import resolution.
func CompileFileToWithDependencies(path, outPath string, dependencyRoots []string) error {
	mod, err := loadModuleGraphWithDependencies(path, dependencyRoots)
	if err != nil {
		return err
	}
	return compileModule(*mod, outPath, path)
}

// CompileProjectTo compiles an entry file and all of its imports/includes.
func CompileProjectTo(path, outPath string) error {
	return CompileFileTo(path, outPath)
}

// GenerateProject emits a native C project for a Rho application using the
// default descriptor.
func GenerateProject(path, outputDir string, includePaths, sourceFiles []string) error {
	return GenerateProjectWithDependencies(path, outputDir, includePaths, sourceFiles, nil)
}

// GenerateProjectWithDependencies emits a native project with dependency
// roots available while loading imported Rho modules.
func GenerateProjectWithDependencies(path, outputDir string, includePaths, sourceFiles, dependencyRoots []string) error {
	mod, err := loadModuleGraphWithDependencies(path, dependencyRoots)
	if err != nil {
		return err
	}
	return GenerateTarget(path, outputDir, "c", includePaths, sourceFiles, dependencyRoots, mod)
}

// GenerateTarget emits one generated project for a descriptor target id.
// The compiled module may be supplied to avoid re-running the front end.
func GenerateTarget(path, outputDir, targetID string, includePaths, sourceFiles, dependencyRoots []string, compiled *ast.Module, shim ...[]string) error {
	// A C project is used as it stands: there is nothing to translate, so the
	// module stands empty and the generator writes a project with no Rho entry
	// wrapper of its own.
	native := strings.HasSuffix(path, ".c")
	var irModule ir.ModuleIR
	if native {
		irModule = ir.NewModuleIR(filepath.Base(ProjectRoot(path)))
	} else {
		converted, convertErr := compileToIR(compiled, path, dependencyRoots)
		if convertErr != nil {
			return convertErr
		}
		irModule = converted
	}
	descriptor, err := ResolveDescriptor(ProjectRoot(path))
	if err != nil {
		return err
	}
	targets, err := descriptor.ResolveTargets([]string{targetID})
	if err != nil {
		return err
	}
	enabledConfigs, guardedSources := resolutionOf(path, dependencyRoots)
	ctx := projectgen.Context{
		Native:         native,
		ModuleName:     irModule.Name,
		ProjectRoot:    ProjectRoot(path),
		ShimSources:    firstOf(shim),
		ShimHeader:     shimHeaderOf(firstOf(shim)),
		OutputDir:      outputDir,
		IR:             irModule,
		IncludePaths:   includePaths,
		SourceFiles:    sourceFiles,
		Config:         configBridge(path, outputDir, dependencyRoots...),
		EnabledConfigs: enabledConfigs,
		GuardedSources: guardedSources,
		SQL:            sqlBridge(ProjectRoot(path)),
	}
	for _, target := range targets {
		if _, err := projectgen.Generate(ctx, target); err != nil {
			return err
		}
	}
	return nil
}

// GenerateAllTargets generates one project per target the manifest requests,
// each under the descriptor's output root followed by the target id. It
// returns the directories written.
//
// When module.yml declares no build.targets the C target is generated, which
// preserves the behaviour of builds that predate multi-target support.
func GenerateAllTargets(path, outputRoot string, includePaths, sourceFiles, dependencyRoots []string) ([]string, error) {
	generated, err := GenerateAllTargetsDetailed(path, outputRoot, includePaths, sourceFiles, dependencyRoots, "")
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(generated))
	for _, target := range generated {
		dirs = append(dirs, target.Dir)
	}
	return dirs, nil
}

// GeneratedTarget is one generated project and how it is built.
type GeneratedTarget struct {
	// ID is the target name, for example "c" or "go-server".
	ID string
	// Dir is the directory the project was written to.
	Dir string
	// Emitter names the backend that produced it, for example "c" or "go".
	Emitter string
}

// GenerateAllTargetsDetailed generates one project per requested target and
// reports how each is built. An only value restricts the build to that single
// target, which is how a test module asks for the backend it can run.
func GenerateAllTargetsDetailed(path, outputRoot string, includePaths, sourceFiles, dependencyRoots []string, only string, shim ...[]string) ([]GeneratedTarget, error) {
	enabledConfigs, guardedSources := resolutionOf(path, dependencyRoots)
	projectRoot := ProjectRoot(path)
	// A project that declared SQL gets its statements generated before anything
	// is compiled, because the generated Rho is an input to the compile: a
	// handler calls a function that does not exist until this has run. Running
	// it here rather than as a separate command is what makes `rho build` mean
	// "build what this project declares" rather than "build what was already
	// written down".
	if err := runSQLGeneration(projectRoot); err != nil {
		return nil, err
	}

	// A C project is a passthrough: it is already C, so there is no module graph
	// to load and nothing to translate. An empty IR stands for it, and the
	// generator writes it a project with no Rho entry wrapper of its own.
	// A C project is a passthrough: there is no module graph to load and nothing
	// to translate, so the IR stays empty and the generator writes the project
	// without an entry wrapper of its own. Its own sources are copied in and
	// compiled there, which is what makes the generated project the whole program
	// rather than a wrapper around one.
	native := strings.HasSuffix(path, ".c")
	var compiled *ast.Module
	if !native {
		loaded, loadErr := loadModuleGraphWithDependencies(path, dependencyRoots)
		if loadErr != nil {
			return nil, loadErr
		}
		compiled = loaded
	}
	// The IR is what the program means. A C project has none of it here — it is
	// already C — so the module stands empty and the generator writes a project
	// with no Rho entry wrapper, which is what lets the project's own `main` be
	// the entry rather than a second one.
	irModule := ir.NewModuleIR(filepath.Base(ProjectRoot(path)))
	if !native {
		compiledIR, compileErr := compileToIR(compiled, path, dependencyRoots)
		if compileErr != nil {
			return nil, compileErr
		}
		irModule = compiledIR
	}

	descriptor, err := ResolveDescriptor(projectRoot)
	if err != nil {
		return nil, err
	}

	// A connected project has two kinds of build target: the emitters it
	// generates from (c, go) and the variants a person names, which are the
	// vendor library a build links. `--target stm32` is the second kind: it is
	// answered by the connection, and the project it generates is the C target
	// with that variant's library behind it.
	connection := cConnection(projectRoot)
	requested, err := BuildTargetIDs(projectRoot)
	if err != nil {
		return nil, err
	}
	if only != "" {
		requested = []string{only}
	}
	if len(requested) == 0 {
		requested = []string{DefaultBuildTarget}
	}
	// A variant is answered by the emitter it builds for, and the variant is
	// carried on the context so the backend can link what it names.
	emitters := make([]string, 0, len(requested))
	variants := map[string]string{}
	for _, name := range requested {
		if _, isVariant := connection.Variant(name); isVariant {
			emitters = append(emitters, ConnectionVariantEmitter)
			variants[ConnectionVariantEmitter] = name
			continue
		}
		emitters = append(emitters, name)
	}
	targets, err := descriptor.ResolveTargets(emitters)
	if err != nil {
		return nil, err
	}

	root := outputRoot
	if root == "" {
		root = filepath.Join(projectRoot, descriptor.OutputRoot(targets[0]))
	}

	written := make([]GeneratedTarget, 0, len(targets))
	for _, target := range targets {
		dir := filepath.Join(root, target.ID)
		ctx := projectgen.Context{
			Native:         native,
			ModuleName:     irModule.Name,
			ProjectRoot:    projectRoot,
			ShimSources:    firstOf(shim),
			ShimHeader:     shimHeaderOf(firstOf(shim)),
			OutputDir:      dir,
			IR:             irModule,
			IncludePaths:   includePaths,
			SourceFiles:    append(sourceFiles, path),
			Config:         configBridge(projectRoot, dir, dependencyRoots...),
			EnabledConfigs: enabledConfigs,
			GuardedSources: guardedSources,
			SQL:            sqlBridge(projectRoot),
			Connection:     connection,
			Variant:        variants[target.ID],
		}
		if _, err := projectgen.Generate(ctx, target); err != nil {
			return written, err
		}
		written = append(written, GeneratedTarget{ID: target.ID, Dir: dir, Emitter: target.Emitter})
	}
	return written, nil
}

// BuildGeneratedTargets compiles the generated projects that are real
// toolchain builds, currently the Go ones, and returns the binaries it
// produced. A target the Rho toolchain builds itself, such as C, is left to
// the native step.
func BuildGeneratedTargets(targets []GeneratedTarget, moduleName, outputDir string) ([]string, error) {
	binaries := []string{}
	for _, target := range targets {
		if target.Emitter != "go" {
			continue
		}
		// The binary is named after the module, not the target, so a project
		// has one executable however many Go targets it declares.
		binary, err := BuildGoProject(target.Dir, outputDir, TargetOutputName(moduleName, target.ID))
		if err != nil {
			return binaries, fmt.Errorf("target %q: %w", target.ID, err)
		}
		binaries = append(binaries, binary)
	}
	return binaries, nil
}

// ProjectName returns the Rho module name declared by a project's manifest.
func ProjectName(path string) string {
	manifest, err := deps.LoadManifest(deps.ManifestPath(ProjectRoot(path)))
	if err != nil || manifest.ModuleName == "" {
		return ""
	}
	return manifest.ModuleName
}

// BuildGoProject compiles a generated Go project into a single binary.
//
// A Go target is a real project with its own toolchain, so building it is a
// separate step from generating it. The binary is written to outputDir, which
// is the descriptor's build_dir, so a project has one obvious artifact.
func BuildGoProject(generatedDir, outputDir, outputName string) (string, error) {
	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("the go toolchain is required to build a Go target: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", err
	}
	binary := filepath.Join(outputDir, outputName)
	// A generated project that requires a module has to have its go.sum written
	// before it can build: `go build` does not download what a go.mod names but
	// has not verified. This is that step, and it is silent when it succeeds —
	// the output of a dependency resolution is not something a person asked to
	// read.
	if goModHasRequires(generatedDir) {
		resolve := exec.Command("go", "mod", "tidy")
		resolve.Dir = generatedDir
		resolve.Env = append(os.Environ(), "GOWORK=off")
		if out, err := resolve.CombinedOutput(); err != nil {
			return "", fmt.Errorf("the modules this project requires could not be resolved: %w\n%s", err, out)
		}
	}
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = generatedDir
	// A generated project stands on its own: it has no requirements beyond the
	// standard library, and it is built where it was generated, which may be
	// inside a repository that has a Go workspace. A workspace claims every
	// module under its root, so a generated project is taken out of it rather
	// than resolved against whatever that repository happens to be working on.
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build failed: %w\n%s", err, out)
	}
	return binary, nil
}

// TargetOutputName is the binary name a target's build produces.
// goModHasRequires reports whether a generated project's go.mod names a module
// to download.
//
// A project with only the standard library is built as it stands, and running a
// dependency resolution for it would be a step that does nothing.
func goModHasRequires(generatedDir string) bool {
	content, err := os.ReadFile(filepath.Join(generatedDir, "go.mod"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "require") {
			return true
		}
	}
	return false
}

func TargetOutputName(moduleName, targetID string) string {
	if moduleName != "" {
		return moduleName
	}
	return targetID
}

// DefaultBuildTarget is generated when a manifest names no targets.
const DefaultBuildTarget = "c"

// readSource is a file's text, for a report that has to count lines in it.
//
// A file that cannot be read yields nothing rather than stopping the report: the
// mistake is still worth saying, and saying where it is not is better than
// saying nothing at all.
func readSource(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	return string(data)
}

// compileToIR runs typechecking and codegen validation, reusing a preloaded
// module when one is supplied.
func compileToIR(compiled *ast.Module, path string, dependencyRoots []string) (ir.ModuleIR, error) {
	if compiled == nil {
		loaded, err := loadModuleGraphWithDependencies(path, dependencyRoots)
		if err != nil {
			return ir.ModuleIR{}, err
		}
		compiled = loaded
	}
	tc := types.New()
	if err := tc.CheckModule(*compiled); err != nil {
		return ir.ModuleIR{}, sourceDiagnostic(path, readSource(path), "typechecking", err)
	}
	irModule := tc.ToIR(*compiled)
	if err := codegen.ValidateModule(irModule); err != nil {
		return ir.ModuleIR{}, fmt.Errorf("codegen validation failed: %w", err)
	}
	// The manifest version travels with the IR so a generated service can
	// report which build it is.
	if manifest, manifestErr := deps.LoadManifest(deps.ManifestPath(ProjectRoot(path))); manifestErr == nil {
		irModule.Version = manifest.Version
	}
	return irModule, nil
}

// ResolveDescriptor returns the project's own project.yml when present, and
// the built-in descriptor otherwise.
func ResolveDescriptor(projectRoot string) (*projectgen.Descriptor, error) {
	candidate := filepath.Join(projectRoot, projectgen.DescriptorFileName)
	if _, err := os.Stat(candidate); err == nil {
		return projectgen.ParseDescriptorFile(candidate)
	}
	return projectgen.DefaultDescriptor()
}

// BuildTargetIDs returns the target ids a project requests through
// build.targets in its manifest. A project with no manifest, or with no
// targets declared, returns an empty list so the caller can fall back to the
// default. A manifest that exists but is malformed is reported.
func BuildTargetIDs(projectRoot string) ([]string, error) {
	manifestPath := deps.ManifestPath(projectRoot)
	if _, err := os.Stat(manifestPath); err != nil {
		return nil, nil
	}
	manifest, err := deps.LoadManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read project manifest: %w", err)
	}
	return manifest.BuildTargets, nil
}

// ConnectionVariantEmitter is the target a variant is built as. A variant names
// the vendor library a build links, not a different backend, so it is answered
// by the emitter of the language it compiles.
const ConnectionVariantEmitter = "c"

// cConnection is the native library a project links, or nil when it declared
// none.
func cConnection(projectRoot string) *deps.CConnection {
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil || manifest == nil {
		return nil
	}
	return manifest.Connection
}

// ProjectRoot returns the directory containing rho.mod, or the source file's
// directory when the file is not part of a project.
// configBridge is the Kconfig bridge a project declared, carried into
// generation.
//
// The fragments are copied into the generated project rather than referenced in
// place, because a generated project is a directory someone may build on its own
// and move: it has to carry what it needs. A project that declared no bridge
// gets none, and its build is what it was.
func configBridge(projectRoot, outputDir string, dependencyRoots ...string) *projectgen.Config {
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil || manifest == nil || manifest.Config == nil {
		return nil
	}
	declared := manifest.Config
	bridge := &projectgen.Config{
		Dialect:   declared.Dialect,
		OutputDir: outputDir,
	}
	if bridge.Dialect == "" {
		// The Linux-style .config and its header are what the C target already
		// speaks, so a project that says nothing gets them.
		bridge.Dialect = "linux"
	}

	// Fragments keep the order they were declared in, because two fragments may
	// define the same symbol and the later one is the one that says what a
	// project gets.
	fragments := []string{}
	copied := filepath.Join(outputDir, "kconfig")
	for _, fragment := range declared.Fragments {
		source := filepath.Join(projectRoot, filepath.FromSlash(fragment))
		data, readErr := os.ReadFile(source)
		if readErr != nil {
			// A fragment that cannot be read is reported by the merge, which
			// runs where the error is useful: in front of a build.
			continue
		}
		if err := os.MkdirAll(copied, 0o755); err != nil {
			return nil
		}
		name := filepath.Base(filepath.FromSlash(fragment))
		if err := os.WriteFile(filepath.Join(copied, name), data, 0o644); err != nil {
			return nil
		}
		fragments = append(fragments, filepath.Join("kconfig", name))
	}

	// Then the fragments the dependencies declare, because a symbol a project sets
	// has to be declared by whoever owns it. The application's `CONFIG_RH_HAL_UART`
	// means nothing unless robohub-common is the one that declares `RH_HAL_UART`,
	// and a merge that saw only the project's own fragment would not know the
	// symbol existed — let alone whether it were set.
	for _, root := range cDependencyChain(projectRoot, dependencyRoots) {
		dependency, depErr := deps.LoadManifest(deps.ManifestPath(root))
		if depErr != nil || dependency == nil {
			continue
		}
		for _, fragment := range kconfigFragmentsOf(root, dependency.Config) {
			data, readErr := os.ReadFile(fragment.path)
			if readErr != nil {
				continue
			}
			if os.MkdirAll(copied, 0o755) != nil {
				return nil
			}
			// Named after the module that declared it, not after the file.
			//
			// Every module writes its own `Kconfig`, and copying them under the
			// name each already had meant one silently overwrote another: the
			// application's own fragment was the one that survived, and the
			// platform's symbols were absent rather than missing, which is the
			// harder of the two to notice.
			name := dependency.ModuleName + "-" + strings.ReplaceAll(fragment.name, "/", "_")
			if os.WriteFile(filepath.Join(copied, name), data, 0o644) != nil {
				return nil
			}
			fragments = append(fragments, filepath.Join("kconfig", name))
		}
	}

	// A file the project writes its symbols in is read where it is.
	//
	// Not copied, unlike the fragments above. A fragment is part of what the
	// program can be built against — it declares symbols — so a generated project
	// that was handed to somebody has to carry them. The values are this project's
	// own choices, and the project they belong to is right here: naming the file is
	// how the build finds it, and copying it would be a second copy of a list that
	// has to be kept in step with the first.
	if declared.ValuesFile != "" {
		path := filepath.Join(projectRoot, filepath.FromSlash(declared.ValuesFile))
		if _, statErr := os.Stat(path); statErr == nil {
			absolute, absErr := filepath.Abs(path)
			if absErr != nil {
				absolute = path
			}
			bridge.Defconfig = filepath.ToSlash(absolute)
		}
	}

	// The values a project sets are written as a defconfig, which is the format
	// every dialect already reads, so a project needs no syntax of its own.
	if len(declared.Values) > 0 {
		if err := os.MkdirAll(copied, 0o755); err == nil {
			names := make([]string, 0, len(declared.Values))
			for name := range declared.Values {
				names = append(names, name)
			}
			sort.Strings(names)
			var defconfig strings.Builder
			for _, name := range names {
				fmt.Fprintf(&defconfig, "%s=%s\n", name, declared.Values[name])
			}
			path := filepath.Join(copied, "rho-defconfig")
			if err := os.WriteFile(path, []byte(defconfig.String()), 0o644); err == nil {
				bridge.Defconfig = filepath.Join("kconfig", "rho-defconfig")
			}
		}
	}
	// A Rho module of constants is a fragment too: the merge reads it with the
	// compiler's own front end, so a value written in Rho is one the compiler
	// would accept.
	if declared.Module != "" {
		fragments = append(fragments, filepath.Join("..", filepath.FromSlash(declared.Module)))
	}
	bridge.Fragments = fragments
	bridge.ValuesFile = ".config"
	return bridge
}

// runSQLGeneration generates a project's statements from the SQL it declared.
//
// The generation is the rho-sql component's work, and it is run as a process
// for the same reason the documentation generator is: the component is a
// separate program reached through the local registry, and the build should not
// be tied to a SQL reader it does not have. The engine that types the statements
// is a plugin the component reaches in turn, so a build that asks an engine for
// its columns gets an engine's answer rather than a reading of a select list.
//
// A project that declared no SQL does nothing here, which is a fact rather than
// a failure: most modules are libraries.
func runSQLGeneration(projectRoot string) error {
	declared := sqlBridge(projectRoot)
	if !declared.Configured() {
		return nil
	}
	// A project whose generated code calls into a package it never named is a
	// build that works on the machine that generated it and nowhere else. The
	// requirement is what anyone building it later reads, so a project that
	// declared a database and no requirement is told once, and the build goes on:
	// a project generated only to be read is a project that never needed one.
	warnMissingRequirement(projectRoot, declared)
	binary, err := componentBinary(SQLComponentName)
	if err != nil {
		return err
	}
	command := exec.Command(binary, "generate", projectRoot)
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("the statements this project declared could not be generated: %w", err)
	}
	return nil
}

// warnMissingRequirement says so when a project declared a database and named
// nothing its generated code links against.
//
// The `sql.driver` and `requires:` are different names on purpose — one is the
// engine plugin that answers a statement, the other the runtime package the
// code calls into — so this does not compare them. It asks only whether the
// project named any requirement at all, which is the fact the schema documents
// as the one a build has to record.
func warnMissingRequirement(projectRoot string, declared *projectgen.SQL) {
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil || manifest == nil {
		return
	}
	if len(manifest.Requires) > 0 {
		return
	}
	fmt.Fprintf(os.Stderr,
		"rho build: this project declares a %s database and no `requires:`; the "+
			"generated code calls into a runtime package, and a project that does "+
			"not name it builds only where it was generated\n",
		declared.Driver)
}

// SQLComponentName is the component that turns SQL into Rho.
const SQLComponentName = "rho-sql"

// SQLiteEnginePlugin is the engine that types a project's statements when it
// declared the sqlite one, which is the engine this repository's own projects
// use.
const SQLiteEnginePlugin = "rho-sql-sqlite"

// componentBinary is the program an installed component runs, which is the file
// its manifest names under `main:`.
//
// A component whose manifest names no binary is a library rather than a
// component, and asking it to run is a mistake worth a message rather than a
// path that does not exist.
func componentBinary(name string) (string, error) {
	root := deps.InstalledComponentRoot(name)
	if root == "" {
		// A component that is installed and not enabled is not the same as one
		// that was never installed, and saying so is the difference between a
		// reader who has to install something and one who has to turn something
		// on. The record knows which it is.
		if versions := deps.InstalledVersions(deps.KindComponent, name); len(versions) > 0 {
			return "", &deps.NotEnabledError{
				Kind:      deps.KindComponent,
				Name:      name,
				Installed: versions,
			}
		}
		return "", fmt.Errorf(
			"this project declared SQL, which needs the %s component, and it is "+
				"not installed; publish it with: rho module publish %s",
			name, filepath.Join("modules", "components", name))
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest.Main == "" {
		return "", fmt.Errorf("the installed %s component does not say what to run", name)
	}
	return filepath.Join(root, manifest.Main), nil
}

// sqlBridge reads the database a project declared, or nil when it declared none.
//
// The generated statements are written by the rho-sql component, which is a
// separate program reached the way every other component is. This reads the
// declaration so the generator knows a project has a database and can supply the
// port its generated code calls into; it does not read the SQL itself.
func sqlBridge(projectRoot string) *projectgen.SQL {
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil || manifest == nil || !manifest.SQL.Declared() {
		return nil
	}
	declared := manifest.SQL
	bridge := &projectgen.SQL{
		Driver:  declared.Driver,
		Schema:  declared.SchemaPath(projectRoot),
		Queries: declared.QueriesPath(projectRoot),
		Out:     declared.OutPath(projectRoot),
		Package: declared.Package,
	}
	if bridge.Package == "" {
		// The generated file belongs to the project, so it is named and
		// packaged as the project is and a handler imports it the way it
		// imports anything else.
		bridge.Package = ProjectName(projectRoot)
	}
	if bridge.Package == "" {
		bridge.Package = "queries"
	}
	return bridge
}

func ProjectRoot(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Dir(path)
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for current := abs; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "module.yml")); err == nil {
			return current
		}
		if _, err := os.Stat(filepath.Join(current, "rho.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs
		}
	}
}

// ProjectOutputPath returns <build_dir>/<entry>.c for a project and a sibling
// .c path for a standalone source file. The descriptor may relocate build_dir.
func ProjectOutputPath(path string) string {
	root := ProjectRoot(path)
	if _, err := os.Stat(filepath.Join(root, "module.yml")); err != nil {
		if _, legacyErr := os.Stat(filepath.Join(root, "rho.mod")); legacyErr != nil {
			return replaceExt(path, ".c")
		}
	}

	buildDir := projectgen.DefaultBuildDir
	// A project that overrides the build location in its descriptor is
	// honoured; a descriptor that fails to parse falls back to the default so
	// the path helper never becomes the source of a build error.
	if descriptor, err := ResolveDescriptor(root); err == nil {
		buildDir = descriptor.BuildRoot()
	}
	return filepath.Join(root, buildDir, filepath.Base(replaceExt(path, ".c")))
}

func compileModule(mod ast.Module, outPath string, from ...string) error {
	tc := types.New()
	if err := tc.CheckModule(mod); err != nil {
		// The module was handed over rather than read here, so when the caller
		// knows which file it came from the report points at that file and at the
		// line; when it does not, the declaration the error names is all there is.
		if len(from) > 0 && from[0] != "" {
			return sourceDiagnostic(from[0], readSource(from[0]), "typechecking", err)
		}
		return fmt.Errorf("typechecking failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := codegen.EmitCToFile(outPath, tc.ToIR(mod)); err != nil {
		return fmt.Errorf("codegen failed: %w", err)
	}
	return nil
}

func loadModuleGraph(entry string) (*ast.Module, error) {
	return loadModuleGraphWithDependencies(entry, nil)
}

func loadModuleGraphWithDependencies(entry string, dependencyRoots []string) (*ast.Module, error) {
	projectRoot := ProjectRoot(entry)
	// The declared components are resolved once for the whole graph. Every
	// import asks the same question, and answering it per import re-reads the
	// manifests for a result that cannot differ inside one load.
	components, err := dependencyComponents(projectRoot)
	if err != nil {
		return nil, err
	}
	visited := map[string]bool{}
	root, loadErr := loadModuleFile(entry, projectRoot, dependencyRoots, components, visited)
	if loadErr != nil {
		return nil, loadErr
	}
	if err := loadDatabaseSurface(root, entry, projectRoot, dependencyRoots, components, visited); err != nil {
		return nil, err
	}
	if manifest, manifestErr := deps.LoadManifest(filepath.Join(projectRoot, "rho.mod")); manifestErr == nil {
		root.Features = append(root.Features, manifest.Features...)
	}
	if err := loadFeatureModules(root, entry, projectRoot, dependencyRoots, components, visited); err != nil {
		return nil, err
	}
	return root, nil
}

// loadDatabaseSurface brings the database surface into a project that declared
// one.
//
// A project that declares `sql:` gets a port generated into it, and the port is
// written in the vocabulary the standard library declares: a handle, a value, a
// statement. Those have to be in scope for the generated program to compile, and
// the project does not import them itself — it declared a database, which is the
// statement that it has one.
//
// So the surface is imported for it, the same way a feature's modules are: it is
// a consequence of a declaration rather than something a project repeats. A
// project that does import it is unaffected, because a module already in the
// graph is not loaded twice.
func loadDatabaseSurface(module *ast.Module, sourcePath, projectRoot string, dependencyRoots []string, components []installedComponent, visited map[string]bool) error {
	if !sqlBridge(projectRoot).Configured() {
		return nil
	}
	importPath, err := resolveImport(DatabaseSurfaceModule, sourcePath, projectRoot, dependencyRoots, components)
	if err != nil {
		return fmt.Errorf("the database surface %q could not be found: %w", DatabaseSurfaceModule, err)
	}
	imported, err := loadModuleFile(importPath, projectRoot, dependencyRoots, components, visited)
	if err != nil {
		return err
	}
	markModuleOwnership(imported, imported.Name)
	module.Decls = append(module.Decls, imported.Decls...)
	module.Features = append(module.Features, imported.Features...)
	return nil
}

// DatabaseSurfaceModule is the standard library module that declares the
// database a Rho program reaches.
const DatabaseSurfaceModule = "std.sql"

func loadFeatureModules(module *ast.Module, sourcePath, projectRoot string, dependencyRoots []string, components []installedComponent, visited map[string]bool) error {
	for _, feature := range module.Features {
		if !strings.HasPrefix(feature, "math.") {
			continue
		}
		name := "std." + feature
		featurePath, err := resolveImport(name, sourcePath, projectRoot, dependencyRoots, components)
		if err != nil {
			return fmt.Errorf("feature %q: %w", feature, err)
		}
		featureModule, err := loadModuleFile(featurePath, projectRoot, dependencyRoots, components, visited)
		if err != nil {
			return fmt.Errorf("load feature %q: %w", feature, err)
		}
		module.Decls = append(module.Decls, featureModule.Decls...)
	}
	return nil
}

func loadModuleFile(path, projectRoot string, dependencyRoots []string, components []installedComponent, visited map[string]bool) (*ast.Module, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve source path: %w", err)
	}
	if visited[abs] {
		return &ast.Module{}, nil
	}
	visited[abs] = true

	source, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read source %q: %w", abs, err)
	}
	toks, err := lexer.Lex(string(source))
	if err != nil {
		return nil, sourceDiagnostic(abs, string(source), "lexing", err)
	}
	mod, err := parser.Parse(toks)
	if err != nil {
		return nil, sourceDiagnostic(abs, string(source), "parsing", err)
	}

	merged := &ast.Module{Name: mod.Name, Imports: nil, Includes: nil, Features: append([]string{}, mod.Features...), Decls: append([]ast.Decl{}, mod.Decls...)}
	// A prelude generic is a language type, so `Slice[u8]` resolves whether or
	// not the file imported the module that declares it. That is a name with no
	// declaration behind it, and the specialisation of a type nobody declared is
	// a type nobody emits — which is why a program using one reached the backend
	// with a type name it had never heard of.
	//
	// The declaration is therefore loaded when the type is used. Importing a
	// module only to obtain a struct the language already provides is not a thing
	// a program should have to do, and the module is loaded once however many
	// times it is applied.
	for _, name := range preludeGenericsUsedBy(mod) {
		loadStdModuleFor(merged, name, abs, projectRoot, dependencyRoots, components, visited)
	}
	for _, include := range mod.Includes {
		includePath := include.Path
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(filepath.Dir(abs), includePath)
		}
		included, err := loadModuleFile(includePath, projectRoot, dependencyRoots, components, visited)
		if err != nil {
			return nil, fmt.Errorf("include %q from %q: %w", include.Path, abs, err)
		}
		merged.Decls = append(merged.Decls, included.Decls...)
		merged.Features = append(merged.Features, included.Features...)
	}
	for _, imp := range mod.Imports {
		importPath, err := resolveImport(imp.Name, abs, projectRoot, dependencyRoots, components)
		if err != nil {
			return nil, fmt.Errorf("import %q from %q: %w", imp.Name, abs, err)
		}
		imported, err := loadModuleFile(importPath, projectRoot, dependencyRoots, components, visited)
		if err != nil {
			return nil, err
		}
		markModuleOwnership(imported, imported.Name)
		merged.Decls = append(merged.Decls, imported.Decls...)
		merged.Features = append(merged.Features, imported.Features...)
	}
	if err := loadFeatureModules(merged, abs, projectRoot, dependencyRoots, components, visited); err != nil {
		return nil, err
	}
	return merged, nil
}

// sourceDiagnostic is a failure reported against a place in a source file.
//
// The place comes from the error, which carries one. It used to be scraped out
// of the error's text, which worked until a message was reworded and then failed
// quietly — a report with no line rather than a wrong one, and no way to tell
// which it was.
func sourceDiagnostic(path, source, phase string, err error) error {
	// A type error is looked for through whatever is wrapped around it: an
	// error that has been through a `%w` on the way here is still the error a
	// position belongs to, and asserting the outermost type misses it.
	if positioned := types.AsError(err); positioned != nil {
		if !positioned.Position.Valid() {
			// A check that could not place its own failure. The declaration it was
			// checking is still worth naming, and a report without a line is
			// honest where a guessed one is not.
			if positioned.Scope != "" {
				return fmt.Errorf("%s %s: %s: %w", phase, path, positioned.Scope, positioned)
			}
			return fmt.Errorf("%s %s: %w", phase, path, positioned)
		}
		return fmt.Errorf("%s %s:%d:%d: %w",
			phase, path, positioned.Position.Line, positioned.Position.Column, positioned)
	}

	// A parser error still says where in the text it is, because the parser
	// reports an offset inside a message rather than as an error value. That is
	// the one place an offset is read out of a message, and it is read from one
	// this repository writes.
	byteOffset := -1
	message := err.Error()
	if marker := strings.Index(message, "byte "); marker >= 0 {
		begin := marker + len("byte ")
		stop := begin
		for stop < len(message) && message[stop] >= '0' && message[stop] <= '9' {
			stop++
		}
		if stop > begin {
			if parsed, parseErr := strconv.Atoi(message[begin:stop]); parseErr == nil {
				byteOffset = parsed
			}
		}
	}
	if byteOffset < 0 {
		return fmt.Errorf("%s %s: %w", phase, path, err)
	}
	if byteOffset > len(source) {
		byteOffset = len(source)
	}
	prefix := source[:byteOffset]
	line := 1 + strings.Count(prefix, "\n")
	column := byteOffset + 1
	if last := strings.LastIndex(prefix, "\n"); last >= 0 {
		column = byteOffset - last
	}
	return fmt.Errorf("%s %s:%d:%d: %w", phase, path, line, column, err)
}

func markModuleOwnership(module *ast.Module, owner string) {
	for index, decl := range module.Decls {
		switch declaration := decl.(type) {
		case ast.FuncDecl:
			if declaration.Owner == "" {
				declaration.Owner = owner
				module.Decls[index] = declaration
			}
		case ast.StructDecl:
			if declaration.Owner == "" {
				declaration.Owner = owner
				module.Decls[index] = declaration
			}
		}
	}
}

// installedComponent is a component dependency, with the import name it
// answers to.
type installedComponent struct {
	root       string
	importPath string
}

// matchesImport reports whether an import name reaches a component. A dotted
// name matches itself, and a shorter prefix matches the component it names, so
// `import ext.probe` and `import ext.probe.util` both work.
func matchesImport(importPath, wanted string) bool {
	if importPath == wanted {
		return true
	}
	return strings.HasPrefix(wanted, importPath+".")
}

// dependencyComponents returns the component dependencies a project declared,
// whether they came from this repository or were installed from elsewhere.
//
// Only declared dependencies are returned: a component is a dependency by being
// named in the manifest, which is also what installs it, so an import and the
// install list cannot disagree.
func dependencyComponents(projectRoot string) ([]installedComponent, error) {
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil || manifest == nil {
		return nil, nil
	}
	// A component type is loaded from a component source root; a Rho library from
	// the module cache. Only the component tree is searched here, because that
	// is where a component keeps its Rho sources alongside its command.
	found := []installedComponent{}
	for _, spec := range manifest.Deps {
		root, importPath, err := componentRoot(projectRoot, spec)
		if err != nil {
			// A dependency that is installed and not active is a stop, not a
			// dependency that happens to be missing: the reader has it, and the
			// build is refusing to use what they did not enable.
			if unavailable, strict := err.(*deps.NotEnabledError); strict {
				return found, unavailable
			}
			continue
		}
		if root == "" {
			continue
		}
		found = append(found, installedComponent{root: root, importPath: importPath})
	}
	return found, nil
}

// componentRoot returns the directory a declared dependency is read from as a
// component, together with the import name that directory answers to. It
// reports false when the dependency is not a component of this project.
//
// A dependency that names a source of its own is never answered from a
// component tree: that source is where its code comes from, and the Rho sources
// the compiler reads have to be the copy the build links. The installation
// answers next, and only the version the dependency resolver would pick, so a
// project with two versions installed does not compile the older one. A project
// with nothing installed falls back to the component in the working tree of the
// repository it lives in, which is what lets a checkout build on a machine that
// has never installed anything.
func componentRoot(projectRoot string, spec deps.DepSpec) (root string, importPath string, err error) {
	if spec.Path != "" || spec.GitURL != "" || spec.EnvVar != "" || spec.Type == deps.DepTypeC {
		return "", "", nil
	}
	root = deps.InstalledComponentRoot(spec.Name)
	if root == "" {
		// An import that names a component is consumed as one, and a component
		// that is installed but not active is not consumed at all. Falling
		// through to the tree here would quietly build against a copy the
		// person never chose, which is the failure this record exists to stop.
		if versions := deps.InstalledVersions(deps.KindComponent, spec.Name); len(versions) > 0 {
			return "", "", &deps.NotEnabledError{
				Kind:      deps.KindComponent,
				Name:      spec.Name,
				Installed: versions,
			}
		}
		root = deps.VendoredComponentRoot(projectRoot, spec.Name)
	}
	if root == "" {
		return "", "", nil
	}
	// A directory in a component tree is a component by construction, and its
	// own manifest says which import name it answers to: a component published
	// under one name can be imported under another, and a manifest that is not
	// a component at all does not answer a Rho import.
	componentManifest, manifestErr := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if manifestErr != nil || componentManifest.ModuleType != "component" {
		return "", "", nil
	}
	return root, componentManifest.ImportPath, nil
}

func resolveImport(name, sourcePath, projectRoot string, dependencyRoots []string, components []installedComponent) (string, error) {
	stdImport := strings.HasPrefix(name, "std.")
	moduleName := strings.TrimPrefix(name, "std.")
	relative := filepath.FromSlash(strings.ReplaceAll(moduleName, ".", "/"))
	candidates := []string{
		filepath.Join(projectRoot, "src", relative+".rho"),
		filepath.Join(projectRoot, relative+".rho"),
		filepath.Join(filepath.Dir(sourcePath), relative+".rho"),
	}
	// A component declared as a dependency contributes its Rho sources too, so
	// `import rho.log` reaches a component rather than only the project tree.
	// The dependency has to be declared, so an import cannot pull in a component
	// the project did not ask for.
	for _, component := range components {
		// The component states the import name it answers to, because that
		// cannot be derived from a published module name.
		imported := component.importPath
		if imported == "" {
			continue
		}
		if !matchesImport(imported, moduleName) {
			continue
		}
		// The full dotted path is tried first, so a component exporting several
		// packages keeps them apart. A component exporting one package may
		// simply call the file after the last segment, which is what
		// `ext.probe` mapping to `src/probe.rho` means.
		componentRelative := filepath.FromSlash(strings.ReplaceAll(imported, ".", "/"))
		lastSegment := imported
		if index := strings.LastIndex(imported, "."); index >= 0 {
			lastSegment = imported[index+1:]
		}
		candidates = append(candidates,
			filepath.Join(component.root, "src", componentRelative+".rho"),
			filepath.Join(component.root, componentRelative+".rho"),
			filepath.Join(component.root, "src", componentRelative, "index.rho"),
			filepath.Join(component.root, "src", lastSegment+".rho"),
		)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if stdImport {
		for current := projectRoot; ; current = filepath.Dir(current) {
			candidate := filepath.Join(current, "std", relative+".rho")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
		}
		if workingDirectory, err := os.Getwd(); err == nil {
			for current := workingDirectory; ; current = filepath.Dir(current) {
				candidate := filepath.Join(current, "std", relative+".rho")
				if _, err := os.Stat(candidate); err == nil {
					return candidate, nil
				}
				parent := filepath.Dir(current)
				if parent == current {
					break
				}
			}
		}
		// The installation the toolchain was installed into, which is where a
		// program outside any checkout finds the library. This is the step that
		// makes a deployed toolchain able to read the standard library at all:
		// before it, `std` was only reachable by walking up from a source tree,
		// so a machine that installed Rho could not compile a program that
		// imported anything.
		if home := deps.RhoHome(); home != "" {
			candidate := filepath.Join(home, "std", relative+".rho")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	for _, dependencyRoot := range dependencyRoots {
		candidates := []string{
			filepath.Join(dependencyRoot, "src", relative+".rho"),
			filepath.Join(dependencyRoot, relative+".rho"),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("file not found for module %q", name)
}

// CompileSource compiles a Rho source string into a C output file.
func CompileSource(source string, outPath string) error {
	// 1. Lexing
	toks, err := lexer.Lex(source)
	if err != nil {
		return fmt.Errorf("lexing failed: %w", err)
	}

	// 2. Parsing
	modAST, err := parser.Parse(toks)
	if err != nil {
		return fmt.Errorf("parsing failed: %w", err)
	}

	// 3. Typechecking
	//
	// The source is the text that was compiled rather than a file, so the report
	// is against that text — the line a reader would see is the line they wrote.
	tc := types.New()
	if err := tc.CheckModule(*modAST); err != nil {
		return sourceDiagnostic("<source>", source, "typechecking", err)
	}

	// 4. AST → IR
	modIR := tc.ToIR(*modAST)

	// 5. IR → C
	if err := codegen.EmitCToFile(outPath, modIR); err != nil {
		return fmt.Errorf("codegen failed: %w", err)
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/*  CLI Entry Point                                                           */
/* -------------------------------------------------------------------------- */

// RunCLI is a thin, testable wrapper around flag parsing and CompileFile.
// It is intended to be called from cmd/rho/main.go.
func RunCLI(args []string) int {
	fs := flag.NewFlagSet("rho", flag.ContinueOnError)
	out := fs.String("o", "", "output C file (optional, default: <input>.c)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "rho: flag parse error:", err)
		return 2
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: rho [options] <file.rho>")
		return 2
	}

	in := fs.Arg(0)
	inAbs, _ := filepath.Abs(in)

	if *out != "" {
		outAbs, _ := filepath.Abs(*out)
		if err := CompileFileTo(inAbs, outAbs); err != nil {
			fmt.Fprintln(os.Stderr, "rho:", err)
			return 1
		}
		return 0
	}

	if err := CompileFile(inAbs); err != nil {
		fmt.Fprintln(os.Stderr, "rho:", err)
		return 1
	}

	return 0
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

func replaceExt(path, newExt string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[:i] + newExt
		}
		if path[i] == '/' || path[i] == '\\' {
			break
		}
	}
	return path + newExt
}

// ResolveEntryPoint finds the Rho source a command should act on.
//
// The argument may be a source file, a project directory, or omitted, in which
// case the working directory is used. A directory is resolved through its
// manifest, so `rho build applications/registry` and `rho run` from inside
// that project both find the entry point the manifest declares.
func ResolveEntryPoint(arg string) (entry string, projectRoot string, err error) {
	candidate := arg
	if candidate == "" {
		candidate = "."
	}
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", err
	}

	info, statErr := os.Stat(absolute)
	if statErr != nil {
		return "", "", fmt.Errorf("no such file or project: %s", candidate)
	}
	if !info.IsDir() {
		root := ProjectRoot(absolute)
		return absolute, root, nil
	}

	// A directory: the manifest names the entry point.
	manifest, manifestErr := deps.LoadManifest(deps.ManifestPath(absolute))
	if manifestErr != nil {
		return "", "", fmt.Errorf("%s is not a Rho project: %w", candidate, manifestErr)
	}
	if manifest.Main == "" {
		return "", "", fmt.Errorf("%s does not declare a main entry point", candidate)
	}
	entry = manifest.Main
	if !filepath.IsAbs(entry) {
		entry = filepath.Join(absolute, entry)
	}
	return entry, absolute, nil
}

// BuiltBinary returns the executable a project's build produced. The Go
// targets build into the descriptor's build_dir, so a project has one obvious
// artifact to run.
func BuiltBinary(entry string) (string, bool) {
	projectRoot := ProjectRoot(entry)
	descriptor, err := ResolveDescriptor(projectRoot)
	if err != nil {
		return "", false
	}
	name := ProjectName(entry)
	if name == "" {
		return "", false
	}
	candidate := filepath.Join(projectRoot, descriptor.BuildRoot(), TargetOutputName(name, ""))
	if _, statErr := os.Stat(candidate); statErr != nil {
		return "", false
	}
	return candidate, true
}

/* -------------------------------------------------------------------------- */
/*  Prelude generics                                                          */
/* -------------------------------------------------------------------------- */

// preludeGenerics maps a type the language provides to the standard library
// module that declares it.
//
// The language knows these names without an import, so a file can use one with no
// declaration of it anywhere. What the compiler still needs is the declaration,
// because that is what a specialisation is built from: `Slice[u8]` is an
// expanded copy of `struct Slice[T]`, and a copy of nothing is a type the
// backend has never heard of.
var preludeGenerics = map[string]string{
	"Slice":  "core.slice",
	"Ptr":    "core.ptr",
	"Option": "core.option",
	"Result": "core.result",
	"Array":  "collections.array",
	"Map":    "collections.map",
	"String": "core.string",
}

// preludeGenericsUsedBy are the language's own generics this module applies.
//
// The body is scanned as well as the signature, because the shape the language
// is most often written in — a local declared as `let s: Slice[u8]` — mentions
// the type only there.
func preludeGenericsUsedBy(module *ast.Module) []string {
	used := []string{}
	seen := map[string]bool{}
	note := func(written string) {
		name, _, found := strings.Cut(written, "[")
		if !found {
			return
		}
		module_, known := preludeGenerics[name]
		if !known || seen[name] {
			return
		}
		seen[name] = true
		used = append(used, module_)
	}
	var walkExpr func(expr ast.Expr)
	walkExpr = func(expr ast.Expr) {
		switch e := expr.(type) {
		case nil:
		case ast.CallExpr:
			// A callee is an expression, and a call to a type is a conversion, so
			// the name is taken by walking rather than by asserting: a conversion
			// and a call look the same here and are not the same thing.
			walkExpr(e.Callee)
			for _, argument := range e.Args {
				walkExpr(argument)
			}
		case ast.IdentifierExpr:
			note(e.Name)
		case ast.StructLiteralExpr:
			note(e.Type.Name)
			for _, value := range e.Values {
				walkExpr(value)
			}
		case ast.MemberExpr:
			walkExpr(e.Object)
		case ast.IndexExpr:
			walkExpr(e.Object)
			walkExpr(e.Index)
		case ast.BinaryExpr:
			walkExpr(e.Left)
			walkExpr(e.Right)
		case ast.UnaryExpr:
			walkExpr(e.Value)
		}
	}
	var walkStmt func(stmt ast.Stmt)
	walkStmt = func(stmt ast.Stmt) {
		switch s := stmt.(type) {
		case ast.LetStmt:
			note(s.Type.String())
			walkExpr(s.Value)
		case ast.AssignStmt:
			walkExpr(s.Target)
			walkExpr(s.Value)
		case ast.ReturnStmt:
			walkExpr(s.Value)
		case ast.ExprStmt:
			walkExpr(s.Expr)
		case ast.IfStmt:
			walkExpr(s.Condition)
			for _, inner := range s.Then {
				walkStmt(inner)
			}
			for _, inner := range s.Else {
				walkStmt(inner)
			}
		case ast.WhileStmt:
			walkExpr(s.Condition)
			for _, inner := range s.Body {
				walkStmt(inner)
			}
		}
	}
	for _, decl := range module.Decls {
		switch d := decl.(type) {
		case ast.FuncDecl:
			note(d.ReturnType)
			for _, param := range d.Params {
				note(param.Type)
			}
			for _, stmt := range d.Body {
				walkStmt(stmt)
			}
		case ast.StructDecl:
			for _, field := range d.Fields {
				note(field.Type)
			}
		}
	}
	return used
}

// loadStdModuleFor loads the standard library module that declares a prelude
// generic, unless the file already has it.
//
// A file that imported the module itself keeps that import, and the declarations
// are not loaded twice: a module read once and merged twice would declare every
// name in it twice, which is a second kind of duplicate rather than a fix.
func loadStdModuleFor(module *ast.Module, name, from, projectRoot string, dependencyRoots []string, components []installedComponent, visited map[string]bool) {
	standard := "std." + name
	if strings.HasPrefix(module.Name, "std.") && module.Name == standard {
		return
	}
	path, err := resolveImport(standard, from, projectRoot, dependencyRoots, components)
	if err != nil {
		// A type the language provides and whose declaration is not installed is a
		// reason to say so where the type is used, not a reason to fail a file
		// that may not use it. The checker's own message is the one a reader gets.
		return
	}
	imported, err := loadModuleFile(path, projectRoot, dependencyRoots, components, visited)
	if err != nil {
		return
	}
	markModuleOwnership(imported, imported.Name)
	module.Decls = append(module.Decls, imported.Decls...)
	module.Features = append(module.Features, imported.Features...)
}

// firstOf returns the first of an optional list, which is how a caller that does
// not have one says so without every other caller having to pass an empty slice.
func firstOf(values [][]string) []string {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

// shimHeaderOf is the header a generated application's own source has to include
// to call the foreign interface.
//
// What a Rho program calls is not the library's symbols but the generated
// wrappers rho-cimport wrote for the declarations Rho cannot make itself, and the
// connection's headers do not declare those. So the header that says what those
// calls look like is the one written beside the shim, and a project that does not
// include it compiles every one of them as an implicit declaration — which is
// accepted, resolves to whatever the symbol happens to be, and turns a signature
// that changed into a failure at run time.
//
// Found by looking beside the source rather than assumed: a shim source is any C
// this build compiles, and a preset's vendor sources are listed there too without a
// header of their own.
func shimHeaderOf(sources []string) string {
	for _, source := range sources {
		if !strings.HasSuffix(source, ".c") {
			continue
		}
		header := strings.TrimSuffix(source, ".c") + ".h"
		if _, err := os.Stat(header); err == nil {
			return filepath.Base(header)
		}
	}
	return ""
}

// namedFragment is a Kconfig file found in a dependency, with the name it should
// be copied under.
type namedFragment struct {
	path string
	name string
}

// kconfigFragmentsOf returns the Kconfig files a module declares, found under its
// own `kconfig` directory.
//
// A module declares where its fragments are; a directory of them named by
// convention would be a second thing to keep right, and a dependency that kept
// its symbols somewhere else would simply have none.
func kconfigFragmentsOf(root string, config *deps.ConfigManifest) []namedFragment {
	if config == nil || len(config.Fragments) == 0 {
		return nil
	}
	found := []namedFragment{}
	for _, fragment := range config.Fragments {
		path := filepath.Join(root, filepath.FromSlash(fragment))
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		found = append(found, namedFragment{path: path, name: filepath.Base(filepath.FromSlash(fragment))})
	}
	return found
}

// CDependencyChain returns every C module reachable from a project, the ones it
// declares first and then theirs.
//
// Exported because a build needs it outside this package: which sources a
// configuration governs is decided from the manifests of the whole chain, not the
// project's alone.
func CDependencyChain(projectRoot string, roots []string) []string {
	return cDependencyChain(projectRoot, roots)
}

// cDependencyChain returns every C module reachable from a project, the ones it
// declares first and then theirs.
//
// Transitive because the module that owns the symbols is usually not the one the
// project depends on. A program depends on a platform, and the platform depends
// on the library whose headers declare `RH_HAL_UART` — so a merge that saw only
// the project's own fragment would not know the symbol existed, and a program that
// set it would be told it had set nothing.
//
// Read from the document rather than through `deps.Manifest`, because a manifest
// writes `depends:` as a list carrying a type and a path, and that reader expects
// the older form where each entry was a version constraint. Asking the other one
// finds no dependencies at all, which looks exactly like a project that has none.
func cDependencyChain(projectRoot string, roots []string) []string {
	ordered := []string{}
	seen := map[string]bool{}

	var walk func(base string, depth int)
	walk = func(base string, depth int) {
		if depth > 16 {
			return
		}
		document, err := yaml.Load(readTextOrEmpty(filepath.Join(base, "module.yml")))
		if err != nil {
			document, err = yaml.Load(readTextOrEmpty(filepath.Join(base, "rho.mod")))
			if err != nil {
				return
			}
		}
		for _, item := range document.Map["depends"].List {
			if item.Kind != yaml.KindMap {
				continue
			}
			if readText(item.Map["type"]) != "c" {
				continue
			}
			path := readText(item.Map["path"])
			if path == "" {
				continue
			}
			root := path
			if !filepath.IsAbs(root) {
				root = filepath.Join(base, root)
			}
			if seen[root] {
				continue
			}
			seen[root] = true
			ordered = append(ordered, root)
			walk(root, depth+1)
		}
	}

	walk(projectRoot, 0)
	for _, root := range roots {
		if seen[root] || !filepath.IsAbs(root) {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "module.yml")); err != nil {
			continue
		}
		seen[root] = true
		ordered = append(ordered, root)
		walk(root, 0)
	}
	return ordered
}

// readTextOrEmpty reads a file, treating an unreadable one as empty: a module
// with no manifest has no fragments to contribute, which is not a failure.
func readTextOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// readText is the text of a mapping's scalar value, or "".
func readText(value yaml.Value) string {
	if value.Str == "" {
		return ""
	}
	return value.Str
}

// resolutionOf returns the configuration symbols a project resolved to and the
// map from a source file to the symbol that governs it.
//
// It is resolved here as well as in the command, because generation has to know:
// a driver that is switched off is not a driver that is unused, and the only
// moment a source can still be left out is before the target is written.
func resolutionOf(entryPath string, dependencyRoots []string) (map[string]bool, map[string]string) {
	// The manifest and the dependencies belong to the project, not to the file the
	// build was pointed at. Handed a `.rho` path, both look inside a folder that
	// does not exist, which is not an error — there is simply nothing to read, so
	// a module silently loses its guards.
	projectRoot := ProjectRoot(entryPath)

	configured := map[string]bool{}
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err == nil && manifest != nil && manifest.Config != nil {
		out := filepath.Join(projectRoot, manifest.BuildDirectory(), "config-check")
		if data, readErr := os.ReadFile(filepath.Join(out, ".config")); readErr == nil {
			for _, line := range strings.Split(string(data), "\n") {
				key, value, found := strings.Cut(strings.TrimSpace(line), "=")
				if !found {
					continue
				}
				key = strings.TrimPrefix(strings.TrimSpace(key), "CONFIG_")
				configured[key] = strings.TrimSpace(value) == "y"
			}
		}
	}
	guarded := map[string]string{}
	for _, root := range CDependencyChain(projectRoot, dependencyRoots) {
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
		return configured, nil
	}
	return configured, guarded
}
