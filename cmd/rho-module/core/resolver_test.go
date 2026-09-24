package core

import (
    "os"
    "path/filepath"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: ResolveVersion                                                        */
/* -------------------------------------------------------------------------- */

func TestResolveVersion(t *testing.T) {
	_ = t.TempDir()

	// Fake local cache
	local := filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod", "math@1.0.0")
	os.MkdirAll(local, 0755)

	// Simulated registry has math@1.2.0

	v, err := ResolveVersion("math", ">=1.0.0 <2.0.0")
	if err != nil {
		t.Fatalf("ResolveVersion failed: %v", err)
	}

	if v != "1.2.0" {
		t.Fatalf("expected latest compatible version 1.2.0, got %s", v)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: No Compatible Version                                                */
/* -------------------------------------------------------------------------- */

func TestResolveVersionNoCompatible(t *testing.T) {
	_, err := ResolveVersion("math", "<1.0.0")
	if err == nil {
		t.Fatalf("expected error for no compatible version")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Local Versions                                                        */
/* -------------------------------------------------------------------------- */

func TestGetLocalVersions(t *testing.T) {
	_ = t.TempDir()
	base := filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod")

	os.MkdirAll(filepath.Join(base, "io@1.0.0"), 0755)
	os.MkdirAll(filepath.Join(base, "io@1.1.0"), 0755)

	versions := getLocalVersions("io")
	if len(versions) != 2 {
		t.Fatalf("expected 2 local versions")
	}
}
