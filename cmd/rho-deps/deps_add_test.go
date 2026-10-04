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

func captureAddOutput(t *testing.T, f func()) string {
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

func TestDepsAdd(t *testing.T) {
	tmp := t.TempDir()

	// minimal rho.mod
	rootMod := `
module root
version 0.1.0
`
	if err := os.WriteFile(filepath.Join(tmp, "rho.mod"), []byte(rootMod), 0644); err != nil {
		t.Fatalf("write rho.mod: %v", err)
	}

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmp)

	out := captureAddOutput(t, func() {
		runDepsAdd([]string{
			"sensor-lib",
			"--git", "https://github.com/aurel/sensor-lib.git",
			"--version", ">=1.0.0 <2.0.0",
			"--type", "c",
		})
	})

	if !strings.Contains(out, "sensor-lib") {
		t.Fatalf("missing dep name in output: %s", out)
	}

	// Check manifest updated
	data, _ := os.ReadFile(filepath.Join(tmp, "rho.mod"))
	if !strings.Contains(string(data), "dep sensor-lib") {
		t.Fatalf("dependency block not added to manifest")
	}
}
