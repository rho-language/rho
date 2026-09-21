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
