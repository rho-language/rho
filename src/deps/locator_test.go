package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocatorEnvOverride(t *testing.T) {
	os.Setenv("TEST_DEP_PATH", "/tmp/testdep")
	defer os.Unsetenv("TEST_DEP_PATH")

	dep := DepSpec{
		Name:   "testdep",
		EnvVar: "TEST_DEP_PATH",
	}

	loc := &DefaultLocator{
		ProjectRoot: "/project",
		GlobalCache: "/global",
		LocalCache:  "/local",
	}

	p, err := loc.Locate(dep)
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}

	if p != "/tmp/testdep" {
		t.Fatalf("expected env override path, got %s", p)
	}
}

func TestLocatorPathOverride(t *testing.T) {
	tmp := t.TempDir()
	localDep := filepath.Join(tmp, "externaldep")
	if err := os.Mkdir(localDep, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	dep := DepSpec{
		Name: "externaldep",
		Path: "externaldep",
	}

	loc := &DefaultLocator{
		ProjectRoot: tmp,
		GlobalCache: "/global",
		LocalCache:  "/local",
	}

	p, err := loc.Locate(dep)
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}

	if p != localDep {
		t.Fatalf("expected path override, got %s", p)
	}
}

func TestLocatorGlobalCache(t *testing.T) {
	tmp := t.TempDir()
	global := filepath.Join(tmp, "globaldep")
	if err := os.Mkdir(global, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	dep := DepSpec{Name: "globaldep"}

	loc := &DefaultLocator{
		ProjectRoot: "/project",
		GlobalCache: tmp,
		LocalCache:  "/local",
	}

	p, err := loc.Locate(dep)
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}

	if p != global {
		t.Fatalf("expected global cache path, got %s", p)
	}
}

func TestLocatorLocalCache(t *testing.T) {
	tmp := t.TempDir()
	local := filepath.Join(tmp, "localdep")
	if err := os.Mkdir(local, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	dep := DepSpec{Name: "localdep"}

	loc := &DefaultLocator{
		ProjectRoot: "/project",
		GlobalCache: "/global",
		LocalCache:  tmp,
	}

	p, err := loc.Locate(dep)
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}

	if p != local {
		t.Fatalf("expected local cache path, got %s", p)
	}
}

func TestLocatorMissingSource(t *testing.T) {
	dep := DepSpec{Name: "nosource"}

	loc := &DefaultLocator{
		ProjectRoot: "/project",
		GlobalCache: "/global",
		LocalCache:  "/local",
	}

	_, err := loc.Locate(dep)
	if err == nil {
		t.Fatalf("expected error for missing git source")
	}
}

func TestGitCloneArgsPreferTag(t *testing.T) {
	args := gitCloneArgs("https://example.com/repo.git", "/tmp/repo", "v1.0.0", "main")
	got := strings.Join(args, " ")
	if got != "clone --branch v1.0.0 --single-branch https://example.com/repo.git /tmp/repo" {
		t.Fatalf("unexpected git args: %s", got)
	}
}
