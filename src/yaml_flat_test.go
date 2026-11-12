package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A program keeps a document it edits key by key as YAML: flat key=value lines
// are written as nested maps, with a header and every value quoted, and read
// back as the same lines; a key with both a value and keys under it survives.
func TestADocumentIsWrittenAndReadAsFlatLines(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  name: flat\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\nbuild:\n  language: go\n  targets:\n    - go\n"), 0o644)
	os.WriteFile(filepath.Join(root, "src", "main.rho"), []byte(`module main
extern fn rho_yaml_flat(path: string) string
extern fn rho_yaml_write_flat(path: string, flat: string, header: string) bool
fn main() void {
    let path: string = getenv("PLAN")
    if rho_yaml_write_flat(path, "node.10.name=b\nnode.2.name=a: #1\nnode.2=root\nrevision=3\n", "A plan.") {
        print(rho_yaml_flat(path))
    }
}
`), 0o644)
	dirs, err := GenerateAllTargets(filepath.Join(root, "src", "main.rho"), "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "flat")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dirs[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	plan := filepath.Join(t.TempDir(), "plan.yml")
	run := exec.Command(binary)
	run.Env = append(os.Environ(), "PLAN="+plan)
	out, err := run.Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "node.2=root\nnode.2.name=a: #1\nnode.10.name=b\nrevision=3\n"; string(out) != want {
		t.Errorf("read back %q, want %q", out, want)
	}
	written, _ := os.ReadFile(plan)
	for _, part := range []string{"# A plan.\n", "node:\n  2:\n    _: \"root\"\n    name: \"a: #1\"\n  10:\n", "revision: \"3\"\n"} {
		if !strings.Contains(string(written), part) {
			t.Errorf("the document lacks %q:\n%s", part, written)
		}
	}
}
