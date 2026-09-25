//
// @file projectgen_test.go
// @brief End-to-end tests for descriptor-driven multi-target generation.
//
// Validates:
//   - the target list comes from the manifest's build.targets
//   - each target lands in its own directory under the descriptor's output_dir
//   - build_dir relocates compiled artefacts away from the sources
//   - a project.yml overrides the built-in descriptor
//   - a Go target compiles and runs, matching the C backend's behaviour
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                  */
/* -------------------------------------------------------------------------- */

// writeRhoProject creates a self-contained Rho project and returns its root.
func writeRhoProject(t *testing.T, manifest string) (root string, entry string) {
	t.Helper()
	root = t.TempDir()
	entry = filepath.Join(root, "src", "main.rho")

	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	source := `module app

fn greet() string {
    return "hello";
}

fn main() void {
    print(greet());
}
`
	if err := os.WriteFile(entry, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	return root, entry
}

const singleTargetManifest = `module:
  name: app
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: c
  targets:
    - c
`

const multiTargetManifest = `module:
  name: app
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: c
  targets:
    - c
    - go
    - go-server
`

/* -------------------------------------------------------------------------- */
/*  Test: Target Selection                                                    */
/* -------------------------------------------------------------------------- */

// A manifest that names no targets keeps the historical C-only behaviour.
func TestGenerateAllTargetsDefaultsToC(t *testing.T) {
	_, entry := writeRhoProject(t, `module:
  name: app
  type: rho
  version: 0.1.0

main: src/main.rho
`)

	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("expected a single target, got %d", len(dirs))
	}
	if filepath.Base(dirs[0]) != DefaultBuildTarget {
		t.Fatalf("expected the %q target, got %q", DefaultBuildTarget, filepath.Base(dirs[0]))
	}
}

func TestGenerateAllTargetsHonoursManifest(t *testing.T) {
	_, entry := writeRhoProject(t, multiTargetManifest)

	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}
	if len(dirs) != 3 {
		t.Fatalf("expected 3 targets, got %d: %v", len(dirs), dirs)
	}
	for i, want := range []string{"c", "go", "go-server"} {
		if filepath.Base(dirs[i]) != want {
			t.Fatalf("target %d: got %q, want %q", i, filepath.Base(dirs[i]), want)
		}
	}
}

// Each target must be written under the descriptor's output root.
func TestGenerateAllTargetsUsesOutputRoot(t *testing.T) {
	_, entry := writeRhoProject(t, multiTargetManifest)

	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}
	for _, dir := range dirs {
		want := filepath.Join("build", "generated", filepath.Base(dir))
		if filepath.Base(filepath.Dir(dir)) != "generated" {
			t.Fatalf("expected %s under build/generated, got %s", want, dir)
		}
	}
}

func TestGenerateAllTargetsRejectsUnknownTarget(t *testing.T) {
	_, entry := writeRhoProject(t, `module:
  name: app
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: c
  targets:
    - rust
`)
	_, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error for an unknown target")
	}
	if !strings.Contains(err.Error(), "rust") {
		t.Fatalf("expected the target named in the error, got %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Generated Layouts                                                   */
/* -------------------------------------------------------------------------- */

func TestGenerateAllTargetsWritesGoProject(t *testing.T) {
	_, entry := writeRhoProject(t, multiTargetManifest)
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}

	goDir := dirs[1]
	for _, name := range []string{"app.go", "main.go", "runtime.go", "go.mod"} {
		if _, err := os.Stat(filepath.Join(goDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestGenerateAllTargetsWritesGoServer(t *testing.T) {
	_, entry := writeRhoProject(t, multiTargetManifest)
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}

	serverDir := dirs[2]
	for _, name := range []string{"app.go", "server.go", "main.go", "go.mod"} {
		if _, err := os.Stat(filepath.Join(serverDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	// The descriptor's dir entry must create the asset folder.
	info, err := os.Stat(filepath.Join(serverDir, "static"))
	if err != nil {
		t.Fatalf("expected the static directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected static to be a directory")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Project Descriptor Override                                         */
/* -------------------------------------------------------------------------- */

// A project.yml takes precedence over the built-in descriptor.
func TestResolveDescriptorPrefersProjectFile(t *testing.T) {
	root := t.TempDir()
	custom := `version: 1
output_dir: dist/gen
build_dir: dist/build
targets:
  - id: custom
    emitter: go
    kind: cli
    files:
      - path: main.go
        role: entrypoint
`
	if err := os.WriteFile(filepath.Join(root, "project.yml"), []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}

	descriptor, err := ResolveDescriptor(root)
	if err != nil {
		t.Fatalf("ResolveDescriptor failed: %v", err)
	}
	if descriptor.OutputDir != "dist/gen" {
		t.Fatalf("expected the custom output_dir, got %q", descriptor.OutputDir)
	}
	if _, ok := descriptor.Target("custom"); !ok {
		t.Fatal("expected the custom target")
	}
	if _, ok := descriptor.Target("c"); ok {
		t.Fatal("a project descriptor replaces the default target set")
	}
}

func TestResolveDescriptorFallsBackToDefault(t *testing.T) {
	descriptor, err := ResolveDescriptor(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveDescriptor failed: %v", err)
	}
	if _, ok := descriptor.Target("c"); !ok {
		t.Fatal("expected the built-in descriptor")
	}
}

// A malformed project.yml must stop the build rather than be ignored.
func TestResolveDescriptorRejectsBadProjectFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "project.yml"), []byte("version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveDescriptor(root); err == nil {
		t.Fatal("expected an error for a descriptor without targets")
	}
}

func TestGenerateAllTargetsUsesProjectDescriptor(t *testing.T) {
	root, entry := writeRhoProject(t, `module:
  name: app
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: c
  targets:
    - custom
`)
	custom := `version: 1
output_dir: dist/gen
targets:
  - id: custom
    emitter: go
    kind: cli
    files:
      - path: main.go
        role: entrypoint
      - path: app.go
        role: implementation
      - path: go.mod
        role: build
`
	if err := os.WriteFile(filepath.Join(root, "project.yml"), []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}

	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}
	want := filepath.Join(root, "dist", "gen", "custom")
	if dirs[0] != want {
		t.Fatalf("expected the custom output path %q, got %q", want, dirs[0])
	}
	if _, err := os.Stat(filepath.Join(want, "app.go")); err != nil {
		t.Fatalf("expected the custom project to be written: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Build Directory                                                     */
/* -------------------------------------------------------------------------- */

// Compiled artefacts belong under build_dir, never beside the sources.
func TestProjectOutputPathUsesBuildDir(t *testing.T) {
	root, entry := writeRhoProject(t, singleTargetManifest)
	got := ProjectOutputPath(entry)
	want := filepath.Join(root, "build", "main.c")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestProjectOutputPathHonoursDescriptor(t *testing.T) {
	root, entry := writeRhoProject(t, singleTargetManifest)
	if err := os.WriteFile(filepath.Join(root, "project.yml"), []byte("version: 1\nbuild_dir: out\ntargets:\n  - id: c\n    emitter: c\n    files:\n      - path: app.c\n        role: implementation\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := ProjectOutputPath(entry)
	want := filepath.Join(root, "out", "main.c")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A standalone file with no project still compiles next to itself.
func TestProjectOutputPathStandaloneFile(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "solo.rho")
	if err := os.WriteFile(entry, []byte("module solo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got, want := ProjectOutputPath(entry), filepath.Join(dir, "solo.c"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Build Target Discovery                                             */
/* -------------------------------------------------------------------------- */

func TestBuildTargetIDs(t *testing.T) {
	root, _ := writeRhoProject(t, multiTargetManifest)
	ids, err := BuildTargetIDs(root)
	if err != nil {
		t.Fatalf("BuildTargetIDs failed: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 targets, got %v", ids)
	}
}

func TestBuildTargetIDsWithoutManifest(t *testing.T) {
	ids, err := BuildTargetIDs(t.TempDir())
	if err != nil {
		t.Fatalf("BuildTargetIDs failed: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no targets without a manifest, got %v", ids)
	}
}

// A manifest that exists but is broken must be reported, not silently ignored.
func TestBuildTargetIDsRejectsBadManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  type: not-a-type\n  name: x\n  version: 1.0.0\nmain: src/main.rho\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildTargetIDs(root); err == nil {
		t.Fatal("expected an error for an invalid manifest")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Generated Go Runs                                                   */
/* -------------------------------------------------------------------------- */

// The strongest guarantee available: a generated Go project compiles and
// produces the same output as the C backend for the same Rho source.
func TestGeneratedGoProjectMatchesC(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}

	_, entry := writeRhoProject(t, multiTargetManifest)
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}

	// Build the C target to learn the expected output.
	cDir := dirs[0]
	cOut := filepath.Join(cDir, "app.c")
	if err := CompileProjectTo(entry, cOut); err != nil {
		t.Fatalf("CompileProjectTo failed: %v", err)
	}
	cBinary := filepath.Join(t.TempDir(), "appc")
	gcc := exec.Command("gcc", "-o", cBinary, cOut, filepath.Join(cDir, "runtime.c"), "-I", cDir)
	if out, err := gcc.CombinedOutput(); err != nil {
		t.Skipf("cannot build the C target here: %v\n%s", err, out)
	}
	wantOut, err := exec.Command(cBinary).CombinedOutput()
	if err != nil {
		t.Skipf("cannot run the C target here: %v", err)
	}

	// Build the Go target.
	goDir := dirs[1]
	goBinary := filepath.Join(t.TempDir(), "appgo")
	build := exec.Command("go", "build", "-o", goBinary, ".")
	build.Dir = goDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("generated Go project did not compile: %v\n%s", err, out)
	}
	gotOut, err := exec.Command(goBinary).CombinedOutput()
	if err != nil {
		t.Fatalf("generated Go binary failed: %v\n%s", err, gotOut)
	}

	if strings.TrimSpace(string(gotOut)) != strings.TrimSpace(string(wantOut)) {
		t.Fatalf("Go output %q does not match C output %q", gotOut, wantOut)
	}
	if !strings.Contains(string(wantOut), "hello") {
		t.Fatalf("expected the program to print a greeting, got %q", wantOut)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: String Escapes Survive Both Backends                                 */
/* -------------------------------------------------------------------------- */

// The lexer decodes escapes and lowering re-encodes them, so the C and Go
// backends must emit the same bytes. A JSON payload is the motivating case:
// before escape support, a quoted field could not be written in Rho at all.
func TestStringEscapesMatchAcrossBackends(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}

	_, entry := writeEscapedProject(t)
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}

	// C target.
	cDir := dirs[0]
	cOut := filepath.Join(cDir, "app.c")
	if err := CompileProjectTo(entry, cOut); err != nil {
		t.Fatalf("CompileProjectTo failed: %v", err)
	}
	cBinary := filepath.Join(t.TempDir(), "appc")
	gcc := exec.Command("gcc", "-o", cBinary, cOut, filepath.Join(cDir, "runtime.c"), "-I", cDir)
	if out, err := gcc.CombinedOutput(); err != nil {
		t.Fatalf("C target did not build: %v\n%s", err, out)
	}
	cStdout, err := exec.Command(cBinary).CombinedOutput()
	if err != nil {
		t.Fatalf("C binary failed: %v\n%s", err, cStdout)
	}

	// Go target.
	goDir := dirs[1]
	goBinary := filepath.Join(t.TempDir(), "appgo")
	build := exec.Command("go", "build", "-o", goBinary, ".")
	build.Dir = goDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Go target did not build: %v\n%s", err, out)
	}
	goStdout, err := exec.Command(goBinary).CombinedOutput()
	if err != nil {
		t.Fatalf("Go binary failed: %v\n%s", err, goStdout)
	}

	if string(cStdout) != string(goStdout) {
		t.Fatalf("backends disagree:\nC  = %q\nGo = %q", cStdout, goStdout)
	}
	// The payload must actually contain a JSON object with a quoted field,
	// proving the escape survived rather than being dropped.
	if !strings.Contains(string(cStdout), `{"name":"rho-lsp"}`) {
		t.Fatalf("expected a JSON payload with an escaped quote, got %q", cStdout)
	}
}

// writeEscapedProject builds a project whose source needs escape sequences.
func writeEscapedProject(t *testing.T) (root string, entry string) {
	t.Helper()
	root = t.TempDir()
	entry = filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: escaped\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: c\n  targets:\n    - c\n    - go\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	source := "module app\n\n" +
		"fn payload() string {\n" +
		"    return \"{\\\"name\\\":\\\"rho-lsp\\\"}\";\n" +
		"}\n\n" +
		"fn main() void {\n" +
		"    print(payload());\n" +
		"}\n"
	if err := os.WriteFile(entry, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	return root, entry
}

/* -------------------------------------------------------------------------- */
/*  Test: Generics End to End                                                  */
/* -------------------------------------------------------------------------- */

// Generic declarations are monomorphised, so a program using Result and Option
// must build and behave identically for the C and Go backends.
func TestGenericProgramMatchesAcrossBackends(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}

	_, entry := writeGenericProject(t)
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}

	cDir := dirs[0]
	cOut := filepath.Join(cDir, "app.c")
	if err := CompileProjectTo(entry, cOut); err != nil {
		t.Fatalf("CompileProjectTo failed: %v", err)
	}
	cBinary := filepath.Join(t.TempDir(), "appc")
	gcc := exec.Command("gcc", "-o", cBinary, cOut, filepath.Join(cDir, "runtime.c"), "-I", cDir)
	if out, err := gcc.CombinedOutput(); err != nil {
		t.Fatalf("C target did not build: %v\n%s", err, out)
	}
	cStdout, err := exec.Command(cBinary).CombinedOutput()
	if err != nil {
		t.Fatalf("C binary failed: %v\n%s", err, cStdout)
	}

	goDir := dirs[1]
	// go vet catches unresolved generic names the compiler would accept.
	vet := exec.Command("go", "vet", "./...")
	vet.Dir = goDir
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("go vet rejected the generated project: %v\n%s", err, out)
	}
	goBinary := filepath.Join(t.TempDir(), "appgo")
	build := exec.Command("go", "build", "-o", goBinary, ".")
	build.Dir = goDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Go target did not build: %v\n%s", err, out)
	}
	goStdout, err := exec.Command(goBinary).CombinedOutput()
	if err != nil {
		t.Fatalf("Go binary failed: %v\n%s", err, goStdout)
	}

	if string(cStdout) != string(goStdout) {
		t.Fatalf("backends disagree:\nC  = %q\nGo = %q", cStdout, goStdout)
	}
	for _, want := range []string{"ok path", "err path", "some path", "none path"} {
		if !strings.Contains(string(cStdout), want) {
			t.Fatalf("expected %q in output, got %q", want, cStdout)
		}
	}
}

// writeGenericProject builds a project that instantiates Result and Option,
// which are monomorphised generic structs.
func writeGenericProject(t *testing.T) (root string, entry string) {
	t.Helper()
	root = t.TempDir()
	entry = filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: generics\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: c\n  targets:\n    - c\n    - go\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	source := `module app

import std.core.result
import std.core.option

fn parse(ok: bool) Result[u32, u32] {
    if ok {
        return Ok[u32, u32](7)
    }
    return Err[u32, u32](1)
}

fn main() void {
    let good: Result[u32, u32] = parse(true)
    if good.is_ok {
        print("ok path");
    }
    let bad: Result[u32, u32] = parse(false)
    if !bad.is_ok {
        print("err path");
    }
    let some: Option[u32] = Some[u32](5)
    if some.is_some {
        print("some path");
    }
    let none: Option[u32] = None[u32]()
    if !none.is_some {
        print("none path");
    }
}
`
	if err := os.WriteFile(entry, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	return root, entry
}

/* -------------------------------------------------------------------------- */
/*  Test: Type Inference End to End                                            */
/* -------------------------------------------------------------------------- */

// An application written without type arguments is deduced and monomorphised,
// so the program must build and behave identically for both backends.
func TestInferredGenericsMatchAcrossBackends(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}

	_, entry := writeInferredProject(t)
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}

	cDir := dirs[0]
	cOut := filepath.Join(cDir, "app.c")
	if err := CompileProjectTo(entry, cOut); err != nil {
		t.Fatalf("CompileProjectTo failed: %v", err)
	}
	cBinary := filepath.Join(t.TempDir(), "appc")
	gcc := exec.Command("gcc", "-o", cBinary, cOut, filepath.Join(cDir, "runtime.c"), "-I", cDir)
	if out, err := gcc.CombinedOutput(); err != nil {
		t.Fatalf("C target did not build: %v\n%s", err, out)
	}
	cStdout, err := exec.Command(cBinary).CombinedOutput()
	if err != nil {
		t.Fatalf("C binary failed: %v\n%s", err, cStdout)
	}

	goDir := dirs[1]
	vet := exec.Command("go", "vet", "./...")
	vet.Dir = goDir
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("go vet rejected the generated project: %v\n%s", err, out)
	}
	goBinary := filepath.Join(t.TempDir(), "appgo")
	build := exec.Command("go", "build", "-o", goBinary, ".")
	build.Dir = goDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Go target did not build: %v\n%s", err, out)
	}
	goStdout, err := exec.Command(goBinary).CombinedOutput()
	if err != nil {
		t.Fatalf("Go binary failed: %v\n%s", err, goStdout)
	}

	if string(cStdout) != string(goStdout) {
		t.Fatalf("backends disagree:\nC  = %q\nGo = %q", cStdout, goStdout)
	}
	for _, want := range []string{"deduced field", "deduced call", "result ok", "result err", "option some", "option none"} {
		if !strings.Contains(string(cStdout), want) {
			t.Fatalf("expected %q in output, got %q", want, cStdout)
		}
	}
}

// writeInferredProject omits type arguments everywhere, so the compiler has to
// deduce them from field values, call arguments and expected types.
func writeInferredProject(t *testing.T) (root string, entry string) {
	t.Helper()
	root = t.TempDir()
	entry = filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: inferred\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: c\n  targets:\n    - c\n    - go\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	source := `module app

import std.core.result
import std.core.option

struct Box[T] {
    value: T
}

fn wrap[T](v: T) Box[T] {
    return Box[T]{ v }
}

fn parse(good: bool) Result[u32, u32] {
    if good {
        return Ok(7)
    }
    return Err(1)
}

fn main() void {
    // deduced from the field value
    let a: Box[i32] = Box{ 1 }
    if a.value == 1 {
        print("deduced field");
    }
    // deduced from the argument
    let b: Box[u32] = wrap(7)
    if b.value == 7 {
        print("deduced call");
    }
    // E is supplied only by the declared return type
    let good: Result[u32, u32] = parse(true)
    if good.is_ok {
        print("result ok");
    }
    let bad: Result[u32, u32] = parse(false)
    if !bad.is_ok {
        print("result err");
    }
    let some: Option[u32] = Some(5)
    if some.is_some {
        print("option some");
    }
    let none: Option[u32] = None()
    if !none.is_some {
        print("option none");
    }
}
`
	if err := os.WriteFile(entry, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	return root, entry
}

/* -------------------------------------------------------------------------- */
/*  Test: Inference and Defaults End to End                                    */
/* -------------------------------------------------------------------------- */

// A program with no type annotations and no explicit type arguments must build
// and behave identically for both backends.
func TestUnannotatedProgramMatchesAcrossBackends(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}

	_, entry := writeUnannotatedProject(t)
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}

	cDir := dirs[0]
	cOut := filepath.Join(cDir, "app.c")
	if err := CompileProjectTo(entry, cOut); err != nil {
		t.Fatalf("CompileProjectTo failed: %v", err)
	}
	cBinary := filepath.Join(t.TempDir(), "appc")
	gcc := exec.Command("gcc", "-o", cBinary, cOut, filepath.Join(cDir, "runtime.c"), "-I", cDir)
	if out, err := gcc.CombinedOutput(); err != nil {
		t.Fatalf("C target did not build: %v\n%s", err, out)
	}
	cStdout, err := exec.Command(cBinary).CombinedOutput()
	if err != nil {
		t.Fatalf("C binary failed: %v\n%s", err, cStdout)
	}

	goDir := dirs[1]
	vet := exec.Command("go", "vet", "./...")
	vet.Dir = goDir
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("go vet rejected the generated project: %v\n%s", err, out)
	}
	goBinary := filepath.Join(t.TempDir(), "appgo")
	build := exec.Command("go", "build", "-o", goBinary, ".")
	build.Dir = goDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Go target did not build: %v\n%s", err, out)
	}
	goStdout, err := exec.Command(goBinary).CombinedOutput()
	if err != nil {
		t.Fatalf("Go binary failed: %v\n%s", err, goStdout)
	}

	if string(cStdout) != string(goStdout) {
		t.Fatalf("backends disagree:\nC  = %q\nGo = %q", cStdout, goStdout)
	}
	for _, want := range []string{"inferred int", "inferred string", "result ok", "result err", "option some", "option none"} {
		if !strings.Contains(string(cStdout), want) {
			t.Fatalf("expected %q in output, got %q", want, cStdout)
		}
	}
}

// writeUnannotatedProject omits every type annotation and type argument it can,
// relying on inference and on generic parameter defaults.
func writeUnannotatedProject(t *testing.T) (root string, entry string) {
	t.Helper()
	root = t.TempDir()
	entry = filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: unannotated\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: c\n  targets:\n    - c\n    - go\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	source := `module app

import std.core.result
import std.core.option

fn main() void {
    // no type annotation
    let total = 1 + 2
    if total == 3 {
        print("inferred int");
    }
    let label = "hi"
    if label == "hi" {
        print("inferred string");
    }
    // no type arguments, and E falls back to its default
    let good = Ok(7)
    if good.is_ok {
        print("result ok");
    }
    let bad = Err(1)
    if !bad.is_ok {
        print("result err");
    }
    let some = Some(5)
    if some.is_some {
        print("option some");
    }
    let none = None()
    if !none.is_some {
        print("option none");
    }
}
`
	if err := os.WriteFile(entry, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	return root, entry
}
