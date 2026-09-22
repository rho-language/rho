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
	outDir := filepath.Join(root, "build")
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
