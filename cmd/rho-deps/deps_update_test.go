package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func createFakeDepUpdate(t *testing.T, root, name, version string) {
	p := filepath.Join(root, name)
	if err := os.Mkdir(p, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := "module " + name + "\nversion " + version + "\n"
	if err := os.WriteFile(filepath.Join(p, "rho.mod"), []byte(content), 0644); err != nil {
		t.Fatalf("write rho.mod: %v", err)
	}
}

func captureUpdateOutput(t *testing.T, f func()) string {
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = stdout

	out, _ := io.ReadAll(r)
	return string(out)
}

/* -------------------------------------------------------------------------- */
/*  Tests                                                                      */
/* -------------------------------------------------------------------------- */

func TestDepsUpdate(t *testing.T) {
	tmp := t.TempDir()

	// depA
	createFakeDepUpdate(t, tmp, "depA", "1.0.0")

	// root rho.mod
	rootMod := `
module root
version 0.1.0

dep depA {
    type = "c"
    version = ">=1.0.0 <2.0.0"
    path = "depA"
}
`
	if err := os.WriteFile(filepath.Join(tmp, "rho.mod"), []byte(rootMod), 0644); err != nil {
		t.Fatalf("write root rho.mod: %v", err)
	}

	// override working directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmp)

	out := captureUpdateOutput(t, func() {
		runDepsUpdate()
	})

	if !strings.Contains(out, "depA 1.0.0") {
		t.Fatalf("update output missing depA: %s", out)
	}

	if _, err := os.Stat(filepath.Join(tmp, "rho.lock")); err != nil {
		t.Fatalf("rho.lock not created")
	}
}
