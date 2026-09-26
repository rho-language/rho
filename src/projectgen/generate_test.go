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
