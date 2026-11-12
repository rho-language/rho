package deps

import (
	"os"
	"os/exec"
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

// A module in a working tree is pinned to the version its manifest declares,
// not to the commit of the repository around it: a commit that does not change
// the module, or one that edits it without a new version, leaves the lock alone.
func TestAWorkingTreeModuleIsPinnedByVersion(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on the path")
	}
	workspace := t.TempDir()
	repo := filepath.Join(workspace, "acme")
	forms := filepath.Join(repo, "modules", "forms")
	os.MkdirAll(forms, 0o755)
	os.WriteFile(filepath.Join(forms, "module.yml"), []byte("module:\n  name: forms\n  type: module\n  version: 0.2.0\nmain: src/main.rho\n"), 0o644)
	git := func(args ...string) {
		command := exec.Command("git", append([]string{"-C", repo, "-c", "user.email=t@example.org", "-c", "user.name=t"}, args...)...)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-q", "-m", "one")
	app := filepath.Join(workspace, "app")
	os.MkdirAll(app, 0o755)
	t.Setenv("RHO_WORKSPACE", workspace)
	t.Setenv("ACME_WORKSPACE", "")

	resolve := func() LockedPackage {
		manifest := &Manifest{Deps: []DepSpec{{Name: "forms", Group: "acme", Version: VersionRange{Raw: ">=0.1.0"}}}}
		resolver := &Resolver{Locator: &DefaultLocator{ProjectRoot: app, GlobalCache: t.TempDir(), LocalCache: filepath.Join(app, ".rho-deps")}}
		_, lock, err := resolver.Resolve(manifest, app)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		for _, pkg := range lock.Packages {
			if pkg.Name == "forms" {
				return pkg
			}
		}
		t.Fatal("forms is not in the lock")
		return LockedPackage{}
	}
	first := resolve()
	if first.Rev != "0.2.0" || first.Version != "0.2.0" || first.Hash != "" {
		t.Fatalf("pin = rev %q version %q hash %q, want the version and no hash", first.Rev, first.Version, first.Hash)
	}
	// Another commit in the repository, and an edit to the module, are not a new version.
	os.WriteFile(filepath.Join(forms, "README.md"), []byte("docs"), 0o644)
	git("add", ".")
	git("commit", "-q", "-m", "two")
	if second := resolve(); second != first {
		t.Errorf("the lock moved without a new version: %+v then %+v", first, second)
	}
}
