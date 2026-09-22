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

func captureStatusOutput(t *testing.T, f func()) string {
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

func TestDepsStatusOK(t *testing.T) {
	tmp := t.TempDir()

	// depA directory
	depA := filepath.Join(tmp, "depA")
	os.Mkdir(depA, 0755)
	os.WriteFile(filepath.Join(depA, "rho.mod"), []byte("module depA\nversion 1.0.0\n"), 0644)

	// rho.mod
	rootMod := `
module root
version 0.1.0

dep depA {
    type = "c"
    version = ">=1.0.0 <2.0.0"
    path = "depA"
}
`
	os.WriteFile(filepath.Join(tmp, "rho.mod"), []byte(rootMod), 0644)

	// rho.lock
	lock := `
depA|1.0.0|https://example.com/depA.git|1.0.0|depA|sha256:xxx
`
	os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte(lock), 0644)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmp)

	out := captureStatusOutput(t, func() {
		runDepsStatus()
	})

	if !strings.Contains(out, "OK           depA") {
		t.Fatalf("status output missing OK: %s", out)
	}
}

func TestDepsStatusMissing(t *testing.T) {
	tmp := t.TempDir()

	// rho.mod
	rootMod := `
module root
version 0.1.0

dep depA {
    type = "c"
    version = ">=1.0.0 <2.0.0"
    path = "depA"
}
`
	os.WriteFile(filepath.Join(tmp, "rho.mod"), []byte(rootMod), 0644)

	// rho.lock empty
	os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte(""), 0644)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmp)

	out := captureStatusOutput(t, func() {
		runDepsStatus()
	})

	if !strings.Contains(out, "MISSING      depA") {
		t.Fatalf("status output missing MISSING: %s", out)
	}
}
