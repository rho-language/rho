package deps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGroupedDependenciesAreDependencies(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "module.yml")
	os.WriteFile(file, []byte("module:\n  name: app\n  version: 0.1.0\n  type: rho\n\nmain: src/main.rho\n\ndeps:\n  rho-log: \">=0.1.0\"\n  acme:\n    forms: \">=0.2.0\"\n"), 0o644)
	m, err := LoadManifest(file)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]DepSpec{}
	for _, d := range m.Deps {
		found[d.Name] = d
	}
	if found["forms"].Group != "acme" || found["forms"].Version.Raw != ">=0.2.0" {
		t.Errorf("forms = %+v", found["forms"])
	}
	if found["rho-log"].Group != "" {
		t.Errorf("a direct dependency has a group: %+v", found["rho-log"])
	}
}

func TestRepositoryModuleRootStopsAtTheRepository(t *testing.T) {
	outer := t.TempDir()
	repo := filepath.Join(outer, "repo")
	app := filepath.Join(repo, "applications", "app")
	for _, dir := range []string{filepath.Join(repo, ".git"), filepath.Join(repo, "modules", "forms"), filepath.Join(repo, "contracts", "gateway"), app, filepath.Join(outer, "modules", "elsewhere")} {
		os.MkdirAll(dir, 0o755)
	}
	os.WriteFile(filepath.Join(repo, "modules", "forms", "module.yml"), []byte("module: {}\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "contracts", "gateway", "module.yml"), []byte("module: {}\n"), 0o644)
	os.WriteFile(filepath.Join(outer, "modules", "elsewhere", "module.yml"), []byte("module: {}\n"), 0o644)
	if got := RepositoryModuleRoot(app, "forms"); got != filepath.Join(repo, "modules", "forms") {
		t.Errorf("forms = %q", got)
	}
	if got := RepositoryModuleRoot(app, "gateway"); got != filepath.Join(repo, "contracts", "gateway") {
		t.Errorf("gateway = %q", got)
	}
	if got := RepositoryModuleRoot(app, "elsewhere"); got != "" {
		t.Errorf("a module outside the repository was found: %q", got)
	}
	if got := RepositoryModuleRoot(app, "../x"); got != "" {
		t.Errorf("a path was accepted as a name: %q", got)
	}
}
