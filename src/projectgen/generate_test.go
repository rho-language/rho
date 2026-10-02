//
// @file generate_test.go
// @brief Tests for descriptor-driven project generation.
//
// Validates:
//   - the C and Go backends populate the descriptor's roles
//   - files land at the paths the descriptor names, including folders
//   - dir entries create folders without writing files
//   - missing role content and unknown emitters are reported
//   - a custom backend can be registered without touching the generator
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package projectgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                  */
/* -------------------------------------------------------------------------- */

// sampleIR is a small module exercising structs, functions and a main entry.
func sampleIR() ir.ModuleIR {
	return ir.ModuleIR{
		Name:     "demo",
		Features: []string{"math"},
		Structs: []ir.StructIR{
			{
				Name:  "Point",
				CName: "Point",
				Fields: []ir.FieldIR{
					{Name: "x", Type: ir.Type{Kind: ir.TypeF64}},
					{Name: "y", Type: ir.Type{Kind: ir.TypeF64}},
				},
			},
		},
		Funcs: []ir.FuncIR{
			{
				Name:       "main",
				CName:      "main",
				ReturnType: ir.Type{Kind: ir.TypeVoid},
			},
		},
	}
}

func newContext(t *testing.T) Context {
	t.Helper()
	root := t.TempDir()
	return Context{
		ModuleName:  "demo",
		ProjectRoot: root,
		OutputDir:   filepath.Join(root, "out"),
		IR:          sampleIR(),
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: C Backend                                                           */
/* -------------------------------------------------------------------------- */

func TestEmitCProjectFillsEveryRole(t *testing.T) {
	artifacts, err := EmitCProject(newContext(t))
	if err != nil {
		t.Fatalf("EmitCProject failed: %v", err)
	}
	for role, content := range map[string]string{
		"implementation": artifacts.Implementation,
		"interface":      artifacts.Interface,
		"runtime":        artifacts.Runtime,
		"entrypoint":     artifacts.Entrypoint,
		"build":          artifacts.Build,
	} {
		if content == "" {
			t.Fatalf("C backend produced no %s content", role)
		}
	}
	if !strings.Contains(artifacts.Interface, "int app_main(void);") {
		t.Error("C interface should declare app_main")
	}
	if !strings.Contains(artifacts.Entrypoint, "int main(void)") {
		t.Error("C entrypoint should define main")
	}
	if !strings.Contains(artifacts.Build, "add_executable") {
		t.Error("C build file should declare an executable")
	}
}

// The C backend must keep splitting the runtime helpers into their own unit.
func TestEmitCProjectSplitsRuntime(t *testing.T) {
	artifacts, err := EmitCProject(newContext(t))
	if err != nil {
		t.Fatalf("EmitCProject failed: %v", err)
	}
	if !strings.Contains(artifacts.Runtime, "rho_concat") {
		t.Error("runtime should carry the rho_concat helper")
	}
	if strings.Contains(artifacts.Implementation, "static char* rho_concat(const char* a, const char* b) {") {
		t.Error("implementation should not keep the runtime helper definition")
	}
}

func TestEmitCProjectUsesCMakeSafeName(t *testing.T) {
	ctx := newContext(t)
	ctx.ModuleName = "my-app.v2"
	artifacts, err := EmitCProject(ctx)
	if err != nil {
		t.Fatalf("EmitCProject failed: %v", err)
	}
	if !strings.Contains(artifacts.Build, "my_app_v2") {
		t.Errorf("expected an underscore-safe project name, got: %s", artifacts.Build)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: sources a dependency does not compile                               */
/* -------------------------------------------------------------------------- */

// writeManifestFile puts a manifest where the generator will find one.
func writeManifestFile(t *testing.T, root, content string) {
	t.Helper()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("create %s: %v", root, err)
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("write manifest in %s: %v", root, err)
	}
}

// platformLayout lays out a repository the way a platform repository is: the
// platform at its root, with an application beside the code it is written for.
//
// The application is inside the platform's directory on purpose, because that is
// where an example lives, and it is the arrangement that decides whether a build
// can tell the application's own sources from the platform's.
func platformLayout(t *testing.T) (platform, project string) {
	t.Helper()
	repo := t.TempDir()
	platform = filepath.Join(repo, "robohub-stm32")
	project = filepath.Join(platform, "examples", "hello_world")

	for _, dir := range []string{
		filepath.Join(platform, "src"),
		filepath.Join(project, "src"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(platform, "CMakeLists.txt"), []byte("# platform\n"), 0644); err != nil {
		t.Fatalf("write platform CMakeLists.txt: %v", err)
	}
	return platform, project
}

// chdirInto moves the process into a directory for the duration of a test, which
// is how a person runs a build: from the project they are building. A generator
// that resolves a declared source against this rather than against the project
// reads a different file depending on where the command was typed.
func chdirInto(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

// What a C dependency provides is compiled by the dependency's own build file, so
// it is left out of the executable this project declares.
func TestASourceADependencyProvidesIsNotCompiledHere(t *testing.T) {
	platform, project := platformLayout(t)
	writeManifestFile(t, project, "module:\n  name: demo\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n"+
		"depends:\n  - name: robohub-stm32\n    type: c\n    path: "+filepath.ToSlash(platform)+"\n")

	ctx := Context{
		ModuleName:  "demo",
		ProjectRoot: project,
		OutputDir:   filepath.Join(project, "build", "generated", "c"),
		IR:          sampleIR(),
	}
	ctx.SourceFiles = []string{"src/main.rho.go", filepath.Join(platform, "src", "driver.c")}
	artifacts, err := EmitCProject(ctx)
	if err != nil {
		t.Fatalf("EmitCProject failed: %v", err)
	}
	if !strings.Contains(artifacts.Build, "src/main.rho.go") {
		t.Error("the project's own source should be compiled by its executable")
	}
	if strings.Contains(artifacts.Build, "driver.c") {
		t.Error("a source a dependency provides is compiled by that dependency, not here")
	}
}

// A project may live inside the repository of the platform it is written for —
// an example beside the platform. Every one of its sources is then under a
// dependency root, and a build that read that as ownership would compile nothing
// of it and report a missing entry point rather than a project whose own file
// had been left out.
func TestAProjectInsideADependencyStillCompilesItsOwnSources(t *testing.T) {
	platform, project := platformLayout(t)
	if err := os.WriteFile(filepath.Join(project, "src", "main.c"), []byte("int main(void){return 0;}\n"), 0644); err != nil {
		t.Fatalf("write main.c: %v", err)
	}
	writeManifestFile(t, project, "module:\n  name: hello-world\n  type: module\n  version: 0.1.0\nmain: src/main.c\n"+
		"build:\n  language: c\n  sources:\n    - src/main.c\n"+
		"depends:\n  - name: robohub-stm32\n    type: c\n    path: "+filepath.ToSlash(platform)+"\n")

	ctx := Context{
		ModuleName:  "hello-world",
		ProjectRoot: project,
		OutputDir:   filepath.Join(project, "build", "generated", "c"),
		IR:          sampleIR(),
		// A C project brings its own entry point, so the sources it declares are
		// the only ones that can define `main`.
		Native:      true,
		SourceFiles: []string{"src/main.c"},
	}
	// Run from the project, as a person does.
	chdirInto(t, project)
	artifacts, err := EmitCProject(ctx)
	if err != nil {
		t.Fatalf("EmitCProject failed: %v", err)
	}
	own := filepath.ToSlash(filepath.Join(ctx.ProjectRoot, "src", "main.c"))
	if !strings.Contains(artifacts.Build, own) {
		t.Errorf("a source the project wrote should be compiled even from inside a dependency:\n%s", artifacts.Build)
	}
	if strings.Contains(artifacts.Build, "app_main") {
		t.Error("a C project brings its own main, so the Rho entry wrapper should be left out")
	}
	if content, ok := artifacts.Extra["src/main.c"]; !ok || !strings.Contains(content, "int main") {
		t.Error("a declared source travels with the generated project, so it can be built on its own")
	}
}

// A configured project declares sources as well as a Kconfig bridge, and gets
// both.
func TestAConfiguredProjectKeepsItsOwnSources(t *testing.T) {
	ctx := newContext(t)
	ctx.Native = true
	if err := os.MkdirAll(filepath.Join(ctx.ProjectRoot, "src"), 0755); err != nil {
		t.Fatalf("create project src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ctx.ProjectRoot, "src", "main.c"), []byte("int main(void){return 0;}\n"), 0644); err != nil {
		t.Fatalf("write main.c: %v", err)
	}
	ctx.SourceFiles = []string{"src/main.c"}
	ctx.Config = &Config{Dialect: "linux", ValuesFile: "project.conf"}

	artifacts, err := EmitCProject(ctx)
	if err != nil {
		t.Fatalf("EmitCProject failed: %v", err)
	}
	if _, ok := artifacts.Extra["rho-config.cmake"]; !ok {
		t.Error("a configured project should get the bridge that resolves its configuration")
	}
	if content, ok := artifacts.Extra["src/main.c"]; !ok || !strings.Contains(content, "int main") {
		t.Error("resolving the configuration should not cost the project the C it wrote")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: the foreign interface                                               */
/* -------------------------------------------------------------------------- */

// What a Rho program calls is a generated wrapper, not a symbol of the library it
// was generated from, so the library's headers do not declare it. The header that
// does is the one written beside the shim, and an application that leaves it out
// compiles every foreign call as an implicit declaration — accepted by the
// compiler, resolved by the linker to whatever the symbol is, and a changed
// signature that only fails at run time.
func TestTheApplicationIncludesWhatTheForeignInterfaceDeclares(t *testing.T) {
	ctx := newContext(t)
	ctx.ShimHeader = "ffi_shim.h"
	artifacts, err := EmitCProject(ctx)
	if err != nil {
		t.Fatalf("EmitCProject failed: %v", err)
	}
	if !strings.Contains(artifacts.Implementation, "#include \"ffi_shim.h\"") {
		t.Errorf("the generated application should include the shim's header:\n%s", artifacts.Implementation)
	}
}

// A project that calls nothing foreign has no shim, and includes nothing for one.
func TestAnApplicationWithoutAForeignInterfaceIncludesNoShim(t *testing.T) {
	ctx := newContext(t)
	artifacts, err := EmitCProject(ctx)
	if err != nil {
		t.Fatalf("EmitCProject failed: %v", err)
	}
	if strings.Contains(artifacts.Implementation, "ffi_shim") {
		t.Errorf("a project with no foreign interface should not include a shim header:\n%s", artifacts.Implementation)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Go Backend                                                           */
/* -------------------------------------------------------------------------- */

func TestEmitGoProjectCLIRole(t *testing.T) {
	ctx := newContext(t)
	artifacts, err := EmitGoProject(ctx)
	if err != nil {
		t.Fatalf("EmitGoProject failed: %v", err)
	}
	if !strings.Contains(artifacts.Implementation, "package main") {
		t.Error("Go implementation should declare package main")
	}
	if !strings.Contains(artifacts.Implementation, "func AppMain()") {
		t.Error("a Rho main should become AppMain")
	}
	if !strings.Contains(artifacts.Entrypoint, "func main()") {
		t.Error("Go entrypoint should define main")
	}
	if !strings.Contains(artifacts.Build, "go 1.22") {
		t.Error("go.mod should declare a Go version")
	}
	if !strings.Contains(artifacts.Build, "module demo") {
		t.Errorf("go.mod should carry the module path, got: %s", artifacts.Build)
	}
}

// A generated Go project carries a go.work naming only itself, so the go tool
// stops looking upwards for a workspace that does not list it. A generated
// project is written into somebody else's checkout often enough that a go.work
// above it would otherwise own the build and refuse its packages — which makes
// the documented `cd build/generated/go && go run .` fail for a reason that has
// nothing to do with the project.
func TestEmitGoProjectIsItsOwnWorkspace(t *testing.T) {
	for _, target := range []Target{
		{ID: "go", Emitter: "go", Kind: "cli"},
		{ID: "go-server", Emitter: "go", Kind: "server"},
	} {
		ctx := newContext(t)
		ctx.Target = target
		artifacts, err := EmitGoProject(ctx)
		if err != nil {
			t.Fatalf("%s: %v", target.ID, err)
		}

		work, ok := artifacts.Extra["go.work"]
		if !ok {
			t.Fatalf("%s: expected a go.work, got extras %v", target.ID, artifacts.Extra)
		}
		if !strings.Contains(work, "use .") {
			t.Errorf("%s: go.work should use the project itself, got: %s", target.ID, work)
		}
		if strings.Contains(work, "=>") || strings.Contains(work, "replace") {
			t.Errorf("%s: go.work should carry no replacements, got: %s", target.ID, work)
		}
	}
}

func TestEmitGoProjectServerRole(t *testing.T) {
	ctx := newContext(t)
	ctx.Target = Target{ID: "go-server", Emitter: "go", Kind: "server"}
	artifacts, err := EmitGoProject(ctx)
	if err != nil {
		t.Fatalf("EmitGoProject failed: %v", err)
	}
	if !strings.Contains(artifacts.Server, "func StartServer(addr string) error") {
		t.Error("server should expose StartServer")
	}
	if !strings.Contains(artifacts.Server, "/healthz") {
		t.Error("server should expose a health route")
	}
	if !strings.Contains(artifacts.Entrypoint, "StartServer(addr)") {
		t.Error("a server entrypoint should start the server")
	}
}

func TestEmitGoProjectHonoursModulePath(t *testing.T) {
	ctx := newContext(t)
	ctx.Target = Target{ID: "api", Emitter: "go", ModulePath: "example.com/org/api"}
	artifacts, err := EmitGoProject(ctx)
	if err != nil {
		t.Fatalf("EmitGoProject failed: %v", err)
	}
	if !strings.Contains(artifacts.Build, "module example.com/org/api") {
		t.Errorf("expected the configured module path, got: %s", artifacts.Build)
	}
}

func TestEmitGoProjectHonoursRequires(t *testing.T) {
	ctx := newContext(t)
	ctx.Target = Target{
		ID:       "api",
		Emitter:  "go",
		Requires: []string{"github.com/go-chi/chi/v5"},
	}
	artifacts, err := EmitGoProject(ctx)
	if err != nil {
		t.Fatalf("EmitGoProject failed: %v", err)
	}
	if !strings.Contains(artifacts.Build, "require (") ||
		!strings.Contains(artifacts.Build, "github.com/go-chi/chi/v5") {
		t.Errorf("expected a require block, got: %s", artifacts.Build)
	}
}

// A main that returns a value has no Go equivalent, and that must be reported
// rather than silently producing an unbuildable project.
func TestEmitGoProjectRejectsNonVoidMain(t *testing.T) {
	ctx := newContext(t)
	ctx.IR.Funcs[0].ReturnType = ir.Type{Kind: ir.TypeI32}
	if _, err := EmitGoProject(ctx); err == nil {
		t.Fatal("expected an error for a main with a return type")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Generate                                                            */
/* -------------------------------------------------------------------------- */

func TestGenerateWritesDescriptorPaths(t *testing.T) {
	descriptor, err := DefaultDescriptor()
	if err != nil {
		t.Fatalf("DefaultDescriptor failed: %v", err)
	}
	target, _ := descriptor.Target("go")

	ctx := newContext(t)
	if _, err := Generate(ctx, target); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	for _, file := range target.Files {
		path := filepath.Join(ctx.OutputDir, filepath.FromSlash(file.Path))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to be written: %v", file.Path, err)
		}
	}
}

// A file path may name folders, which the generator must create.
func TestGenerateCreatesNestedFolders(t *testing.T) {
	target := Target{
		ID:      "nested",
		Emitter: "go",
		Files: []DescriptorFile{
			{Path: "internal/api/handlers.go", Role: "implementation"},
		},
	}
	ctx := newContext(t)
	if _, err := Generate(ctx, target); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ctx.OutputDir, "internal", "api", "handlers.go")); err != nil {
		t.Fatalf("expected the nested file to exist: %v", err)
	}
}

// A dir entry creates a folder and writes nothing into it.
func TestGenerateCreatesDirectories(t *testing.T) {
	target := Target{
		ID:      "withdirs",
		Emitter: "go",
		Kind:    "server",
		Files: []DescriptorFile{
			{Path: "main.go", Role: "entrypoint"},
			{Path: "static", Kind: "dir"},
			{Path: "templates", Kind: "dir"},
		},
	}
	ctx := newContext(t)
	if _, err := Generate(ctx, target); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	for _, dir := range []string{"static", "templates"} {
		info, err := os.Stat(filepath.Join(ctx.OutputDir, dir))
		if err != nil {
			t.Fatalf("expected the %s directory: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", dir)
		}
		entries, err := os.ReadDir(filepath.Join(ctx.OutputDir, dir))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("expected %s to be empty, got %d entries", dir, len(entries))
		}
	}
}

func TestGenerateRejectsUnknownEmitter(t *testing.T) {
	target := Target{
		ID:      "rust",
		Emitter: "rust",
		Files:   []DescriptorFile{{Path: "main.rs", Role: "implementation"}},
	}
	_, err := Generate(newContext(t), target)
	if err == nil {
		t.Fatal("expected an error for an unknown emitter")
	}
	if !strings.Contains(err.Error(), "rust") {
		t.Fatalf("expected the emitter named in the error, got %v", err)
	}
}

func TestGenerateRejectsUnknownRole(t *testing.T) {
	target := Target{
		ID:      "c",
		Emitter: "c",
		Files:   []DescriptorFile{{Path: "mystery.c", Role: "mystery"}},
	}
	if _, err := Generate(newContext(t), target); err == nil {
		t.Fatal("expected an error for an unknown role")
	}
}

// A required role with no content would leave a hole in the generated
// project, so generation must fail rather than skip it. The Go backend has no
// interface artefact, so an interface role cannot be filled.
func TestGenerateRejectsEmptyRequiredRole(t *testing.T) {
	target := Target{
		ID:      "go",
		Emitter: "go",
		Files:   []DescriptorFile{{Path: "app.h", Role: "interface"}},
	}
	if _, err := Generate(newContext(t), target); err == nil {
		t.Fatal("expected an error for a required role with no content")
	}
}

func TestGenerateSkipsOptionalEmptyRole(t *testing.T) {
	target := Target{
		ID:      "go",
		Emitter: "go",
		Files: []DescriptorFile{
			{Path: "app.h", Role: "interface", Optional: true},
		},
	}
	ctx := newContext(t)
	if _, err := Generate(ctx, target); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ctx.OutputDir, "app.h")); err == nil {
		t.Fatal("expected the optional empty file to be skipped")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Backend Registry                                                    */
/* -------------------------------------------------------------------------- */

// The point of the descriptor is that a new backend needs no generator change.
func TestRegisterBackend(t *testing.T) {
	const emitter = "test-emitter"
	RegisterBackend(emitter, func(ctx Context) (Artifacts, error) {
		return Artifacts{Implementation: "// custom\n", Build: "custom\n"}, nil
	})
	t.Cleanup(func() { delete(backends, emitter) })

	if !containsEmitted(RegisteredEmitters(), emitter) {
		t.Fatal("expected the backend to be registered")
	}

	target := Target{
		ID:      "custom",
		Emitter: emitter,
		Files:   []DescriptorFile{{Path: "custom.go", Role: "implementation"}},
	}
	ctx := newContext(t)
	if _, err := Generate(ctx, target); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(ctx.OutputDir, "custom.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "// custom\n" {
		t.Fatalf("unexpected content %q", string(data))
	}
}

func TestArtifactsRoleContent(t *testing.T) {
	artifacts := Artifacts{
		Implementation: "impl",
		Interface:      "iface",
		Runtime:        "rt",
		Entrypoint:     "entry",
		Server:         "srv",
		Build:          "bld",
	}
	for role, want := range map[string]string{
		"implementation": "impl",
		"interface":      "iface",
		"runtime":        "rt",
		"entrypoint":     "entry",
		"server":         "srv",
		"build":          "bld",
	} {
		got, ok := artifacts.RoleContent(role)
		if !ok {
			t.Fatalf("role %q should be known", role)
		}
		if got != want {
			t.Fatalf("role %q: got %q, want %q", role, got, want)
		}
	}
	if _, ok := artifacts.RoleContent("nope"); ok {
		t.Fatal("expected an unknown role to be reported")
	}
}

func containsEmitted(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
