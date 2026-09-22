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

func captureCleanOutput(t *testing.T, f func()) string {
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

func TestDepsClean(t *testing.T) {
	tmp := t.TempDir()

	// create .rho-deps
	os.Mkdir(filepath.Join(tmp, ".rho-deps"), 0755)

	// create rho.lock
	os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte("test"), 0644)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmp)

	out := captureCleanOutput(t, func() {
		runDepsClean()
	})

	if !strings.Contains(out, "removed .rho-deps") {
		t.Fatalf("missing .rho-deps removal output: %s", out)
	}
	if !strings.Contains(out, "removed rho.lock") {
		t.Fatalf("missing rho.lock removal output: %s", out)
	}

	// verify deletion
	if _, err := os.Stat(filepath.Join(tmp, ".rho-deps")); !os.IsNotExist(err) {
		t.Fatalf(".rho-deps not removed")
	}
	if _, err := os.Stat(filepath.Join(tmp, "rho.lock")); !os.IsNotExist(err) {
		t.Fatalf("rho.lock not removed")
	}
}
