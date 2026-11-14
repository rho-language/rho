package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A program's own extern, implemented in C the project carries, is declared in
// the generated C: built as C99 and GCC 14 build it, a call to an undeclared
// function is an error rather than an implicit `int` declaration.
func TestAProgramsOwnExternIsDeclaredForTheCCompiler(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(parityManifest, "    - go\n", "", 1)
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	source := "module app\n\nextern fn board_twice(x: u64) u64\n\nfn main() void {\n    print(i64_to_str(i64(board_twice(21))) + \"\\n\")\n}\n"
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cDir := dirs[0]
	impl := filepath.Join(t.TempDir(), "board.c")
	if err := os.WriteFile(impl, []byte("#include <stdint.h>\nuint64_t board_twice(uint64_t x) { return x * 2u; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "app")
	build := exec.Command("gcc", "-std=c11", "-Werror=implicit-function-declaration", "-o", binary,
		filepath.Join(cDir, "main.c"), filepath.Join(cDir, "app.c"), filepath.Join(cDir, "runtime.c"), impl, "-I", cDir, "-lm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("C build failed: %v\n%s", err, out)
	}
	result := runBinary(binary)
	if result.failed != "" || result.stdout != "42\n" {
		t.Fatalf("got %q, failed=%q", result.stdout, result.failed)
	}
}
