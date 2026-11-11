package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A secret written by a Rho program is readable by its owner only, on either
// backend, and a file that was already there with a wider mode is narrowed.
func TestWriteSecretIsOwnerOnlyOnBothBackends(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not on the path")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "module.yml"), []byte(parityManifest), 0o644)
	os.WriteFile(entry, []byte(`module app

extern fn rho_write_secret(path: string, text: string) bool
extern fn rho_read_text(path: string) string

fn main() void {
    if rho_write_secret("secret.txt", "s3cret") {
        print(rho_read_text("secret.txt"))
    }
}
`), 0o644)
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets: %v", err)
	}
	cDir, goDir := dirs[0], dirs[1]
	cBinary := filepath.Join(t.TempDir(), "secret-c")
	if out, err := exec.Command("gcc", "-std=c11", "-o", cBinary,
		filepath.Join(cDir, "main.c"), filepath.Join(cDir, "app.c"), filepath.Join(cDir, "runtime.c"), "-I", cDir).CombinedOutput(); err != nil {
		t.Fatalf("C build: %v\n%s", err, out)
	}
	goBinary := filepath.Join(t.TempDir(), "secret-go")
	build := exec.Command("go", "build", "-o", goBinary, ".")
	build.Dir = goDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Go build: %v\n%s", err, out)
	}
	for name, binary := range map[string]string{"c": cBinary, "go": goBinary} {
		work := t.TempDir()
		// A file that is already there, world-readable, is the case that matters:
		// creating a file with a mode does not change the mode of one that exists.
		if err := os.WriteFile(filepath.Join(work, "secret.txt"), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		run := exec.Command(binary)
		run.Dir = work
		out, err := run.Output()
		if err != nil || string(out) != "s3cret" {
			t.Fatalf("%s: output %q, %v", name, out, err)
		}
		info, err := os.Stat(filepath.Join(work, "secret.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("%s: the secret's mode is %o, want 600", name, mode)
		}
	}
}
