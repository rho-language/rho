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

func capturePurgeOutput(t *testing.T, f func()) string {
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

func TestDepsPurge(t *testing.T) {
	tmp := t.TempDir()

	// create .rho-deps
	os.Mkdir(filepath.Join(tmp, ".rho-deps"), 0755)

	// create rho.lock
	os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte("test"), 0644)

	// create global cache
	global := filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod")
	os.MkdirAll(global, 0755)
	os.WriteFile(filepath.Join(global, "dummy"), []byte("x"), 0644)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmp)

	out := capturePurgeOutput(t, func() {
		runDepsPurge()
	})

	if !strings.Contains(out, "Purged all dependency state") {
		t.Fatalf("missing purge output: %s", out)
	}

	// verify deletion
	if _, err := os.Stat(filepath.Join(tmp, ".rho-deps")); !os.IsNotExist(err) {
		t.Fatalf(".rho-deps not removed")
	}
	if _, err := os.Stat(filepath.Join(tmp, "rho.lock")); !os.IsNotExist(err) {
		t.Fatalf("rho.lock not removed")
	}
	if _, err := os.Stat(global); !os.IsNotExist(err) {
		t.Fatalf("global cache not removed")
	}
}
