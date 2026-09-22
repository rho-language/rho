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

func captureVerifyOutput(t *testing.T, f func()) string {
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

func TestDepsVerifyOK(t *testing.T) {
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

	out := captureVerifyOutput(t, func() {
		runDepsVerify()
	})

	if !strings.Contains(out, "OK: depA 1.0.0") {
		t.Fatalf("verify output missing OK: %s", out)
	}
}

func TestDepsVerifyMissing(t *testing.T) {
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

	// rho.lock missing depA
	lock := ``
	os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte(lock), 0644)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmp)

	out := captureVerifyOutput(t, func() {
		runDepsVerify()
	})

	if !strings.Contains(out, "ERR: missing lock entry for depA") {
		t.Fatalf("verify output missing error: %s", out)
	}
}
