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
// SPDX-License-Identifier: MIT
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
	types "rho/src/types"
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
	return compileModule(*mod, outPath)
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
func GenerateTarget(path, outputDir, targetID string, includePaths, sourceFiles, dependencyRoots []string, compiled *ast.Module) error {
	irModule, err := compileToIR(compiled, path, dependencyRoots)
	if err != nil {
		return err
	}
	descriptor, err := ResolveDescriptor(ProjectRoot(path))
	if err != nil {
		return err
	}
	targets, err := descriptor.ResolveTargets([]string{targetID})
	if err != nil {
		return err
	}
	ctx := projectgen.Context{
		ModuleName:   irModule.Name,
		ProjectRoot:  ProjectRoot(path),
		OutputDir:    outputDir,
		IR:           irModule,
		IncludePaths: includePaths,
		SourceFiles:  sourceFiles,
		Config:       configBridge(path, outputDir),
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
func GenerateAllTargetsDetailed(path, outputRoot string, includePaths, sourceFiles, dependencyRoots []string, only string) ([]GeneratedTarget, error) {
	compiled, err := loadModuleGraphWithDependencies(path, dependencyRoots)
	if err != nil {
		return nil, err
	}
	irModule, err := compileToIR(compiled, path, dependencyRoots)
	if err != nil {
		return nil, err
	}

	projectRoot := ProjectRoot(path)
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
			ModuleName:   irModule.Name,
			ProjectRoot:  projectRoot,
			OutputDir:    dir,
			IR:           irModule,
			IncludePaths: includePaths,
			SourceFiles:  sourceFiles,
			Config:       configBridge(projectRoot, dir),
			Connection:   connection,
			Variant:      variants[target.ID],
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
func TargetOutputName(moduleName, targetID string) string {
	if moduleName != "" {
		return moduleName
	}
	return targetID
}

// DefaultBuildTarget is generated when a manifest names no targets.
const DefaultBuildTarget = "c"

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
		return ir.ModuleIR{}, fmt.Errorf("typechecking failed: %w", err)
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
func configBridge(projectRoot, outputDir string) *projectgen.Config {
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

func compileModule(mod ast.Module, outPath string) error {
	tc := types.New()
	if err := tc.CheckModule(mod); err != nil {
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
	components := dependencyComponents(projectRoot)
	visited := map[string]bool{}
	root, err := loadModuleFile(entry, projectRoot, dependencyRoots, components, visited)
	if err != nil {
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

func sourceDiagnostic(path, source, phase string, err error) error {
	message := err.Error()
	byteOffset := -1
	if marker := strings.Index(message, "byte "); marker >= 0 {
		start := marker + len("byte ")
		end := start
		for end < len(message) && message[end] >= '0' && message[end] <= '9' {
			end++
		}
		if end > start {
			if parsed, parseErr := strconv.Atoi(message[start:end]); parseErr == nil {
				byteOffset = parsed
			}
		}
	}
	line, column := 1, 1
	if byteOffset >= 0 {
		if byteOffset > len(source) {
			byteOffset = len(source)
		}
		prefix := source[:byteOffset]
		line = 1 + strings.Count(prefix, "\n")
		if last := strings.LastIndex(prefix, "\n"); last >= 0 {
			column = byteOffset - last
		} else {
			column = byteOffset + 1
		}
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
func dependencyComponents(projectRoot string) []installedComponent {
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil || manifest == nil {
		return nil
	}
	// A component type is loaded from a component source root; a Rho library from
	// the module cache. Only the component tree is searched here, because that
	// is where a component keeps its Rho sources alongside its command.
	found := []installedComponent{}
	for _, spec := range manifest.Deps {
		root, importPath, ok := componentRoot(projectRoot, spec)
		if !ok {
			continue
		}
		found = append(found, installedComponent{root: root, importPath: importPath})
	}
	return found
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
func componentRoot(projectRoot string, spec deps.DepSpec) (root string, importPath string, ok bool) {
	if spec.Path != "" || spec.GitURL != "" || spec.EnvVar != "" || spec.Type == deps.DepTypeC {
		return "", "", false
	}
	root = deps.InstalledComponentRoot(spec.Name)
	if root == "" {
		root = deps.VendoredComponentRoot(projectRoot, spec.Name)
	}
	if root == "" {
		return "", "", false
	}
	// A directory in a component tree is a component by construction, and its
	// own manifest says which import name it answers to: a component published
	// under one name can be imported under another, and a manifest that is not
	// a component at all does not answer a Rho import.
	componentManifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || componentManifest.ModuleType != "component" {
		return "", "", false
	}
	return root, componentManifest.ImportPath, true
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
	tc := types.New()
	if err := tc.CheckModule(*modAST); err != nil {
		return fmt.Errorf("typechecking failed: %w", err)
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
