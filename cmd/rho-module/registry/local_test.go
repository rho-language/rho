package registry

import (
	"os"
	"path/filepath"
	"testing"

	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Test: ListLocalModules                                                     */
/* -------------------------------------------------------------------------- */

func TestListLocalModules(t *testing.T) {
	tmp := t.TempDir()
	paths.SetTestRhoHome(tmp)
	defer paths.SetTestRhoHome("")

	if err := paths.EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	base, _ := paths.GetRegistryPath()
	os.MkdirAll(filepath.Join(base, "math@1.0.0"), 0755)
	os.MkdirAll(filepath.Join(base, "math@1.1.0"), 0755)

	modules, err := ListLocalModules()
	if err != nil {
		t.Fatalf("ListLocalModules failed: %v", err)
	}

	if len(modules) < 2 {
		t.Fatalf("expected at least 2 modules")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: LocalVersions                                                        */
/* -------------------------------------------------------------------------- */

func TestLocalVersions(t *testing.T) {
	tmp := t.TempDir()
	paths.SetTestRhoHome(tmp)
	defer paths.SetTestRhoHome("")

	if err := paths.EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	base, _ := paths.GetRegistryPath()
	os.MkdirAll(filepath.Join(base, "io@1.0.0"), 0755)
	os.MkdirAll(filepath.Join(base, "io@1.1.0"), 0755)

	versions, err := LocalVersions("io")
	if err != nil {
		t.Fatalf("LocalVersions failed: %v", err)
	}

	if len(versions) != 2 {
		t.Fatalf("expected 2 versions")
	}
}
