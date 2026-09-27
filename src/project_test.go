package rho

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileProjectResolvesImportAndInclude(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src", "lib"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rho.mod"), []byte("module test-project\nversion 0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "lib", "message.rho"), []byte(`module message

fn imported() string {
    return "imported"
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "included.rho"), []byte(`module included

fn included() string {
    return " included"
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.WriteFile(entry, []byte(`module main
include "included.rho"
import lib.message

fn main() void {
    print(imported() + included());
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "build", "main.c")
	if err := CompileProjectTo(entry, out); err != nil {
		t.Fatalf("CompileProjectTo failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "imported(") || !strings.Contains(string(data), "included(") {
		t.Fatalf("generated C does not contain imported and included functions")
	}
}

func TestGenerateProjectLayout(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rho.mod"), []byte("module app\nversion 0.1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(`module app

fn init() void {
}

fn main() void {
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, "build", "generated")
	if err := GenerateProject(entry, outDir, []string{"/opt/include"}, []string{"/opt/src/support.c"}); err != nil {
		t.Fatalf("GenerateProject failed: %v", err)
	}
	for _, name := range []string{"app.c", "app.h", "main.c", "runtime.c", "CMakeLists.txt", "config.h"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("missing generated file %s: %v", name, err)
		}
	}
	mainC, err := os.ReadFile(filepath.Join(outDir, "main.c"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainC), "init();") {
		t.Fatal("generated app_init does not call Rho init")
	}
	cmake, err := os.ReadFile(filepath.Join(outDir, "CMakeLists.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cmake), "/opt/src/support.c") || !strings.Contains(string(cmake), "/opt/include") {
		t.Fatal("generated CMakeLists.txt is missing dependency paths")
	}
}

func TestProjectBuildOutputPath(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rho.mod"), []byte("module app\nversion 1.0.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := ProjectOutputPath(entry); got != filepath.Join(root, "build", "main.c") {
		t.Fatalf("unexpected project output: %s", got)
	}
}

func TestStandardMathModulesParse(t *testing.T) {
	modules := []string{
		"../std/math/basic.rho",
		"../std/math/float.rho",
		"../std/math/control.rho",
		"../std/math/linear.rho",
		"../std/math/fixed.rho",
		"../std/math/fast.rho",
		"../std/math/embedded.rho",
	}
	for _, modulePath := range modules {
		source, err := os.ReadFile(modulePath)
		if err != nil {
			t.Fatalf("read %s: %v", modulePath, err)
		}
		if err := CompileSource(string(source), filepath.Join(t.TempDir(), "module.c")); err != nil {
			t.Fatalf("compile %s: %v", modulePath, err)
		}
	}
}

func TestFeatureDeclarationsReachGeneratedC(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rho.mod"), []byte("module feature-app\nversion 0.1.0\nfeature = \"module.manifest\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "main.rho")
	if err := os.WriteFile(entry, []byte("module feature\nfeature \"module.local\"\n\nfn main() void {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "build", "app.c")
	if err := GenerateProject(entry, filepath.Dir(out), nil, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, define := range []string{"RHO_FEATURE_MATH 1", "RHO_FEATURE_MODULE_MANIFEST 1", "RHO_FEATURE_MODULE_LOCAL 1"} {
		if !strings.Contains(text, define) {
			t.Fatalf("missing feature define %q", define)
		}
	}
}

func TestStandardLibraryImportResolves(t *testing.T) {
	out := filepath.Join(t.TempDir(), "app.c")
	entry := "../examples/robohub-common-app/src/main.rho"
	if err := CompileProjectTo(entry, out); err != nil {
		t.Fatalf("compile standard-library import: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "typedef struct") || !strings.Contains(string(data), "PID") {
		t.Fatal("standard control module was not included in generated C")
	}
}

func TestDependencyModuleImportResolves(t *testing.T) {
	root := t.TempDir()
	dependency := filepath.Join(root, "dependency")
	if err := os.MkdirAll(filepath.Join(dependency, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependency, "rho.mod"), []byte("module dependency\nversion 1.0.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependency, "src", "message.rho"), []byte(`module message

fn dependency_message() string {
    return "from dependency"
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rho.mod"), []byte("module app\nversion 1.0.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "main.rho")
	if err := os.WriteFile(entry, []byte(`module app
import message

fn main() void {
    print(dependency_message());
}
`), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "build", "app.c")
	if err := CompileFileToWithDependencies(entry, out, []string{dependency}); err != nil {
		t.Fatalf("dependency import failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "dependency_message") {
		t.Fatal("generated C does not contain dependency module function")
	}
}

func TestManifestFeatureLoadsStandardModule(t *testing.T) {
	out := filepath.Join(t.TempDir(), "app.c")
	if err := CompileProjectTo("../examples/robohub-common-app/src/main.rho", out); err != nil {
		t.Fatalf("compile feature-enabled project: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "PID") || !strings.Contains(string(data), "RHO_FEATURE_MATH_CONTROL 1") {
		t.Fatal("manifest feature did not load math.control")
	}
}

func TestStandardLibraryImportGraphsCompile(t *testing.T) {
	err := filepath.Walk("../std", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || filepath.Ext(path) != ".rho" {
			return nil
		}
		if compileErr := CompileProjectTo(path, filepath.Join(t.TempDir(), "module.c")); compileErr != nil {
			t.Errorf("%s: %v", path, compileErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestApplicationCanImportEmbeddedModuleGraph(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rho.mod"), []byte("module app\nversion 1.0.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "main.rho")
	if err := os.WriteFile(entry, []byte("module app\nimport std.embedded.modbus\n\nfn main() void {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CompileProjectTo(entry, filepath.Join(root, "build", "app.c")); err != nil {
		t.Fatal(err)
	}
}

func TestCompilerDiagnosticIncludesSourceLocation(t *testing.T) {
	err := CompileSource("module broken\n\nfn main() void {\n  let value: bool = 1\n}\n", filepath.Join(t.TempDir(), "broken.c"))
	if err == nil || !strings.Contains(err.Error(), "typechecking failed: function main") {
		t.Fatalf("expected source-aware compiler error, got %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Component imports                                                          */
/* -------------------------------------------------------------------------- */

// writeRepository marks a directory as the repository root, which is where the
// search for a component in the working tree stops.
func writeRepository(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	// A linked checkout carries a .git file rather than a directory, so a
	// repository marker is written as a file.
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: .git\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeComponent writes a component that exports a Rho library, which is the
// shape a project imports: a manifest naming the import path, and the library
// sources next to it. The marker ends up in the generated C, so a test can see
// which copy of the component answered for the import.
func writeComponent(t *testing.T, root, marker string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n" +
		"  name: rho-log\n" +
		"  type: component\n" +
		"  version: 0.1.0\n" +
		"  command: log\n" +
		"  import_path: rho.log\n" +
		"\n" +
		"main: log\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	library := "module rho.log\n\nfn subject() string {\n    return \"" + marker + "\";\n}\n"
	if err := os.WriteFile(filepath.Join(root, "src", "log.rho"), []byte(library), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeComponentProject writes an application that imports the component. The
// dependency is declared unless it is left out, because a component is a
// dependency by being named in the manifest.
func writeComponentProject(t *testing.T, root string, declareDependency bool) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n" +
		"  name: app\n" +
		"  type: rho\n" +
		"  version: 0.1.0\n" +
		"\n" +
		"main: src/main.rho\n"
	if declareDependency {
		manifest += "\ndeps:\n  rho-log: \">=0.1.0\"\n"
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	entry := "module app\nimport rho.log\n\nfn main() void {\n    print(subject());\n}\n"
	if err := os.WriteFile(filepath.Join(root, "src", "main.rho"), []byte(entry), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestComponentImportResolvesFromRepositoryTree(t *testing.T) {
	// Nothing is installed, which is what a fresh machine and a CI runner look
	// like: the component the repository carries is the only copy there is.
	t.Setenv("RHO_HOME", t.TempDir())
	repo := t.TempDir()
	writeRepository(t, repo)
	writeComponent(t, filepath.Join(repo, "modules", "components", "rho-log"), "from-repository")
	project := writeComponentProject(t, filepath.Join(repo, "applications", "app"), true)

	out := filepath.Join(t.TempDir(), "app.c")
	if err := CompileProjectTo(filepath.Join(project, "src", "main.rho"), out); err != nil {
		t.Fatalf("compile component import: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "from-repository") {
		t.Fatal("the component in the repository tree did not answer the import")
	}
}

func TestInstalledComponentWinsOverRepositoryTree(t *testing.T) {
	rhoHome := t.TempDir()
	t.Setenv("RHO_HOME", rhoHome)
	repo := t.TempDir()
	writeRepository(t, repo)
	writeComponent(t, filepath.Join(repo, "modules", "components", "rho-log"), "from-repository")
	installed := filepath.Join(rhoHome, "modules", "installed", "component", "rho-log@0.1.0")
	writeComponent(t, installed, "from-installation")
	project := writeComponentProject(t, filepath.Join(repo, "applications", "app"), true)

	out := filepath.Join(t.TempDir(), "app.c")
	if err := CompileProjectTo(filepath.Join(project, "src", "main.rho"), out); err != nil {
		t.Fatalf("compile component import: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "from-installation") {
		t.Fatal("the installed component did not answer the import")
	}
}

func TestComponentImportResolvesNewestInstalledVersion(t *testing.T) {
	// The version the build links is the version the import has to compile.
	rhoHome := t.TempDir()
	t.Setenv("RHO_HOME", rhoHome)
	repo := t.TempDir()
	writeRepository(t, repo)
	installed := filepath.Join(rhoHome, "modules", "installed", "component")
	writeComponent(t, filepath.Join(installed, "rho-log@0.1.0"), "from-0-1-0")
	writeComponent(t, filepath.Join(installed, "rho-log@0.2.0"), "from-0-2-0")
	project := writeComponentProject(t, filepath.Join(repo, "applications", "app"), true)

	out := filepath.Join(t.TempDir(), "app.c")
	if err := CompileProjectTo(filepath.Join(project, "src", "main.rho"), out); err != nil {
		t.Fatalf("compile component import: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "from-0-2-0") {
		t.Fatal("the import did not resolve the newest installed component")
	}
}

func TestSourcedDependencyDoesNotAnswerAComponentImport(t *testing.T) {
	// A dependency that names its own source is not read from a component
	// tree: the sources the compiler reads have to be the ones the build links.
	t.Setenv("RHO_HOME", t.TempDir())
	repo := t.TempDir()
	writeRepository(t, repo)
	writeComponent(t, filepath.Join(repo, "modules", "components", "rho-log"), "from-repository")
	project := filepath.Join(repo, "applications", "app")
	writeComponentProject(t, project, false)

	// A native dependency, declared the way an application that links a C
	// library declares it.
	manifest := "module:\n" +
		"  name: app\n" +
		"  type: rho\n" +
		"  version: 0.1.0\n" +
		"\n" +
		"main: src/main.rho\n" +
		"\n" +
		"depends:\n" +
		"  - name: rho-log\n" +
		"    type: c\n" +
		"    version: 1.0.0\n" +
		"    path: /nowhere/rho-log\n"
	if err := os.WriteFile(filepath.Join(project, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	err := CompileProjectTo(filepath.Join(project, "src", "main.rho"), filepath.Join(t.TempDir(), "app.c"))
	if err == nil || !strings.Contains(err.Error(), "rho.log") {
		t.Fatalf("expected a sourced dependency not to answer a component import, got %v", err)
	}
}

func TestUndeclaredComponentImportDoesNotResolve(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	repo := t.TempDir()
	writeRepository(t, repo)
	writeComponent(t, filepath.Join(repo, "modules", "components", "rho-log"), "from-repository")
	project := writeComponentProject(t, filepath.Join(repo, "applications", "app"), false)

	err := CompileProjectTo(filepath.Join(project, "src", "main.rho"), filepath.Join(t.TempDir(), "app.c"))
	if err == nil || !strings.Contains(err.Error(), "rho.log") {
		t.Fatalf("expected an undeclared component import to fail, got %v", err)
	}
}

func TestComponentImportStopsAtTheRepository(t *testing.T) {
	// A component directory above the repository belongs to some other
	// project, so it must not decide what this project compiles.
	t.Setenv("RHO_HOME", t.TempDir())
	outer := t.TempDir()
	writeComponent(t, filepath.Join(outer, "modules", "components", "rho-log"), "from-outside")
	repo := filepath.Join(outer, "repo")
	writeRepository(t, repo)
	project := writeComponentProject(t, filepath.Join(repo, "applications", "app"), true)

	err := CompileProjectTo(filepath.Join(project, "src", "main.rho"), filepath.Join(t.TempDir(), "app.c"))
	if err == nil || !strings.Contains(err.Error(), "rho.log") {
		t.Fatalf("expected the component search to stop at the repository, got %v", err)
	}
}
