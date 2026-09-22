package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func captureOutput(f func()) string {
	var buf bytes.Buffer
	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = stdout
	buf.ReadFrom(r)
	return buf.String()
}

/* -------------------------------------------------------------------------- */
/*  Tests                                                                      */
/* -------------------------------------------------------------------------- */

func TestDepsTreeCLI(t *testing.T) {
	tmp := t.TempDir()

	// depB
	createFakeDep(t, tmp, "depB", "2.0.0")

	// depA → depB
	createFakeDep(t, tmp, "depA", "1.0.0", "depB")

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

	out := captureOutput(func() {
		runDepsTree()
	})

	if !strings.Contains(out, "depA 1.0.0") {
		t.Fatalf("missing depA in tree: %s", out)
	}
	if !strings.Contains(out, "-> depB") {
		t.Fatalf("missing depB in tree: %s", out)
	}
}
