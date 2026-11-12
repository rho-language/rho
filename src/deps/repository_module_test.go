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

// A grouped dependency is found in the repository its group names when that
// repository sits in the workspace, which is how a module that left the
// repository it was developed in still builds beside it.
func TestAGroupNamesTheRepositoryItsMembersComeFrom(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "acme-mesh")
	forms := filepath.Join(workspace, "acme", "modules", "forms")
	other := filepath.Join(workspace, "other", "modules", "widgets")
	for _, dir := range []string{filepath.Join(app, ".git"), filepath.Join(workspace, "acme", ".git"), forms, other} {
		os.MkdirAll(dir, 0o755)
	}
	os.WriteFile(filepath.Join(forms, "module.yml"), []byte("module: {}\n"), 0o644)
	os.WriteFile(filepath.Join(other, "module.yml"), []byte("module: {}\n"), 0o644)
	t.Setenv("RHO_WORKSPACE", workspace)
	t.Setenv("ACME_WORKSPACE", "")

	if got := WorkspaceGroupModuleRoot(app, "acme", "forms"); got != forms {
		t.Errorf("forms = %q, want %q", got, forms)
	}
	if got := WorkspaceGroupModuleRoot(app, "acme", "widgets"); got != "" {
		t.Errorf("a module of another repository answered for the group: %q", got)
	}
	if got := WorkspaceGroupModuleRoot(app, "", "forms"); got != "" {
		t.Errorf("a dependency with no group was looked for in the workspace: %q", got)
	}
	// The group's own variable moves its repository elsewhere.
	moved := t.TempDir()
	t.Setenv("ACME_WORKSPACE", moved)
	if got := WorkspaceGroupModuleRoot(app, "acme", "forms"); got != "" {
		t.Errorf("ACME_WORKSPACE was not honoured: %q", got)
	}
}
