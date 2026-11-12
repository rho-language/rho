package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A program takes a named lock around two steps another goroutine of a service
// could come between — read a counter, write the next value — and on Go the
// lock is a mutex by that name.
func TestANamedLockIsAMutexOnGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  name: locks\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\nbuild:\n  language: go\n  targets:\n    - go\n"), 0o644)
	os.WriteFile(filepath.Join(root, "src", "main.rho"), []byte(`module main
extern fn rho_lock(name: string)
extern fn rho_unlock(name: string)
fn main() void {
    rho_lock("counter")
    print("held")
    rho_unlock("counter")
    rho_lock("counter")
    print("|again")
    rho_unlock("counter")
}
`), 0o644)
	dirs, err := GenerateAllTargets(filepath.Join(root, "src", "main.rho"), "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	runtime, _ := os.ReadFile(filepath.Join(dirs[0], "runtime.go"))
	if !strings.Contains(string(runtime), "sync.Mutex") {
		t.Errorf("the lock is not a mutex:\n%s", runtime)
	}
	binary := filepath.Join(t.TempDir(), "locks")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dirs[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(binary).Output()
	if err != nil || string(out) != "held|again" {
		t.Errorf("run: %q, %v", out, err)
	}
}
