package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A test build reports each test fn it was told about: `[PASS] <name>` after a
// test that returned, once per call, and nothing after one that stopped the
// program or one nothing called. A test's value comes back unchanged.
func TestATestBuildReportsEachTestFnThatReturns(t *testing.T) {
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc is not on the path")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	os.MkdirAll(filepath.Dir(entry), 0o755)
	os.WriteFile(filepath.Join(root, "module.yml"), []byte(parityManifest), 0o644)
	os.WriteFile(entry, []byte(`module app

fn test_passes() void {
    if 1 + 1 != 2 {
        panic("arithmetic")
    }
}

fn test_returns(n: i32) i32 {
    if n > 1 {
        return n * 2
    }
    return 0
}

fn test_never() void {
}

fn test_aborts() void {
    panic("stopped")
}

fn main() void {
    test_passes()
    test_passes()
    print(int_to_str(test_returns(21)))
    print("\n")
    test_aborts()
    print("unreachable\n")
}
`), 0o644)
	cFile := filepath.Join(t.TempDir(), "main.c")
	if err := CompileTestFileTo(entry, cFile, nil, []string{"test_passes", "test_returns", "test_never", "test_aborts"}); err != nil {
		t.Fatalf("CompileTestFileTo: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "main")
	if out, err := exec.Command("cc", cFile, "-o", binary).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, out)
	}
	stdout, err := exec.Command(binary).Output()
	if err == nil {
		t.Fatal("a program whose test panicked exited 0")
	}
	want := "[PASS] test_passes\n[PASS] test_passes\n[PASS] test_returns\n42\n"
	if string(stdout) != want {
		t.Fatalf("stdout %q, want %q", stdout, want)
	}
	if strings.Contains(string(stdout), "test_aborts") || strings.Contains(string(stdout), "test_never") {
		t.Fatalf("a test that stopped or never ran reported: %q", stdout)
	}

	// An ordinary build of the same program reports nothing.
	plain := filepath.Join(t.TempDir(), "plain.c")
	if err := CompileFileToWithDependencies(entry, plain, nil); err != nil {
		t.Fatal(err)
	}
	if source, _ := os.ReadFile(plain); strings.Contains(string(source), "[PASS]") {
		t.Fatal("a build that was not a test build reports test fns")
	}

	// A name the program does not define is refused rather than left silent.
	if err := CompileTestFileTo(entry, cFile, nil, []string{"test_missing"}); err == nil || !strings.Contains(err.Error(), "test_missing") {
		t.Fatalf("an unknown test name: %v", err)
	}
}
