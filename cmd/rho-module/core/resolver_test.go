package core

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: ResolveVersion                                                        */
/* -------------------------------------------------------------------------- */

func TestResolveVersion(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

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
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	_, err := ResolveVersion("math", "<1.0.0")
	if err == nil {
		t.Fatalf("expected error for no compatible version")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Local Versions                                                        */
/* -------------------------------------------------------------------------- */

func TestGetLocalVersions(t *testing.T) {
	_, cleanup := SetupTestRegistry(t)
	defer cleanup()

	versions := getLocalVersions("io")
	if len(versions) != 2 {
		t.Fatalf("expected 2 local versions")
	}
}
