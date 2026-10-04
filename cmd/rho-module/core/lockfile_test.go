package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Load Lockfile                                                        */
/* -------------------------------------------------------------------------- */

func TestLoadLockfile(t *testing.T) {
	tmp := t.TempDir()

	content := `
lock:
  math: "1.2.0"
  io: "1.1.0"
`
	os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte(content), 0644)

	lock, err := LoadLockfile(tmp)
	if err != nil {
		t.Fatalf("LoadLockfile failed: %v", err)
	}

	if lock["math"] != "1.2.0" {
		t.Fatalf("wrong version for math")
	}
	if lock["io"] != "1.1.0" {
		t.Fatalf("wrong version for io")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Write Lockfile                                                       */
/* -------------------------------------------------------------------------- */

func TestWriteLockfile(t *testing.T) {
	tmp := t.TempDir()

	deps := map[string]string{
		"math": "1.2.0",
		"io":   "1.1.0",
	}

	if err := WriteLockfile(tmp, deps); err != nil {
		t.Fatalf("WriteLockfile failed: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmp, "rho.lock"))
	s := string(data)

	if !strings.Contains(s, "math: \"1.2.0\"") {
		t.Fatalf("math missing in lockfile")
	}
	if !strings.Contains(s, "io: \"1.1.0\"") {
		t.Fatalf("io missing in lockfile")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Validate Lockfile                                                    */
/* -------------------------------------------------------------------------- */

func TestValidateLockfile(t *testing.T) {
	tmp := t.TempDir()

	os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte(`
lock:
  math: "1.2.0"
`), 0644)

	if err := ValidateLockfile(tmp); err != nil {
		t.Fatalf("ValidateLockfile failed: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid Version                                                      */
/* -------------------------------------------------------------------------- */

func TestValidateLockfileInvalid(t *testing.T) {
	tmp := t.TempDir()

	os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte(`
lock:
  math: "1.2"
`), 0644)

	if err := ValidateLockfile(tmp); err == nil {
		t.Fatalf("expected invalid semver error")
	}
}
