package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A number of one kind stands where another is declared — an argument, a let,
// an assignment, a return — as C allows implicitly. Go converts nothing
// implicitly, so the generated Go has to write the conversions, or the program
// the checker accepted does not compile.
func TestANumberMeetsTheTypeItIsDeclaredAsOnGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  name: conv\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\nbuild:\n  language: go\n  targets:\n    - go\n"), 0o644)
	os.WriteFile(filepath.Join(root, "src", "main.rho"), []byte(`module main
fn half(x: u32) u32 { return x / 2 }
fn widen(x: u8) u64 { return x }
fn main() void {
    let a: i32 = 7
    let b: u8 = 9
    var c: u32 = 0
    c = a
    let d: u64 = b
    let e: f64 = a
    print(int_to_str(a) + int_to_str(half(b)) + int_to_str(c) + int_to_str(u32(d)) + int_to_str(u32(widen(b))) + int_to_str(u32(e)))
}
`), 0o644)
	dirs, err := GenerateAllTargets(filepath.Join(root, "src", "main.rho"), "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "conv")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dirs[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(binary).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != "747997" {
		t.Errorf("output = %q, want %q", out, "747997")
	}
}
