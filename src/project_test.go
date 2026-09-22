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
	for _, name := range []string{"app.c", "app.h", "main.c", "CMakeLists.txt", "config.h"} {
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
