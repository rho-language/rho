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

/* -------------------------------------------------------------------------- */
/*  Components in the repository a project lives in                           */
/* -------------------------------------------------------------------------- */

// writeRepository marks a directory as the repository root, which is where the
// search for a component in the working tree stops.
func writeRepository(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", root, err)
	}
	// A linked checkout carries a .git file rather than a directory, so a
	// repository marker is written as a file.
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: .git\n"), 0644); err != nil {
		t.Fatalf("write repository marker in %s: %v", root, err)
	}
}

// writeComponent writes a component directory: the manifest makes it a
// component, and the manifest is one the schema accepts, so the fixture is
// something `rho module install` could have produced.
func writeComponent(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", root, err)
	}
	manifest := "module:\n" +
		"  name: rho-log\n" +
		"  type: component\n" +
		"  version: 0.1.0\n" +
		"  command: log\n" +
		"  import_path: rho.log\n" +
		"\n" +
		"main: log\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatalf("write manifest in %s: %v", root, err)
	}
}

func TestLocatePrefersInstalledComponentOverRepository(t *testing.T) {
	rhoHome := t.TempDir()
	t.Setenv("RHO_HOME", rhoHome)

	repo := t.TempDir()
	writeRepository(t, repo)
	project := filepath.Join(repo, "applications", "registry")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	writeComponent(t, filepath.Join(repo, "modules", "components", "rho-log"))
	installed := filepath.Join(rhoHome, "modules", "installed", "component", "rho-log@0.1.0")
	writeComponent(t, installed)

	loc := &DefaultLocator{ProjectRoot: project}
	got, err := loc.Locate(DepSpec{Name: "rho-log"})
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}
	if got != installed {
		t.Fatalf("expected the installed component %s, got %s", installed, got)
	}
}

func TestLocateFallsBackToRepositoryComponent(t *testing.T) {
	// Nothing is installed, which is what a fresh CI machine looks like.
	t.Setenv("RHO_HOME", t.TempDir())

	repo := t.TempDir()
	writeRepository(t, repo)
	project := filepath.Join(repo, "applications", "registry")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	vendored := filepath.Join(repo, "modules", "components", "rho-log")
	writeComponent(t, vendored)

	loc := &DefaultLocator{ProjectRoot: project}
	got, err := loc.Locate(DepSpec{Name: "rho-log"})
	if err != nil {
		t.Fatalf("Locate error: %v", err)
	}
	if got != vendored {
		t.Fatalf("expected the repository component %s, got %s", vendored, got)
	}
}

func TestLocateIgnoresRepositoryComponentWhenASourceIsDeclared(t *testing.T) {
	// A dependency that names a source is resolved from that source. Answering
	// it with a same-named component in the working tree would record a lock
	// entry that claims one source and points at another.
	t.Setenv("RHO_HOME", t.TempDir())

	repo := t.TempDir()
	writeRepository(t, repo)
	project := filepath.Join(repo, "applications", "registry")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	vendored := filepath.Join(repo, "modules", "components", "rho-log")
	writeComponent(t, vendored)
	// A directory that is not a repository, so a clone of it fails without
	// reaching the network.
	notARepo := filepath.Join(repo, "not-a-repository")
	if err := os.MkdirAll(notARepo, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", notARepo, err)
	}
	t.Setenv("TEST_DEP_SOURCE", filepath.Join(repo, "from-the-environment"))

	sourced := []DepSpec{
		{Name: "rho-log", Path: filepath.Join(repo, "from-a-path")},
		{Name: "rho-log", EnvVar: "TEST_DEP_SOURCE"},
		{Name: "rho-log", GitURL: notARepo, GitBranch: "main"},
	}
	for _, dep := range sourced {
		loc := &DefaultLocator{ProjectRoot: project}
		got, _ := loc.Locate(dep)
		if got == vendored {
			t.Fatalf("dependency %+v was answered with the working tree component %s", dep, got)
		}
	}
}

func TestInstalledComponentRootPrefersNewestVersion(t *testing.T) {
	// Two installed versions of one component: the import has to resolve the
	// version the build links, not whichever the directory listing names first.
	rhoHome := t.TempDir()
	t.Setenv("RHO_HOME", rhoHome)
	installed := filepath.Join(rhoHome, "modules", "installed", "component")
	writeComponent(t, filepath.Join(installed, "rho-log@0.1.0"))
	writeComponent(t, filepath.Join(installed, "rho-log@0.2.0"))

	want := filepath.Join(installed, "rho-log@0.2.0")
	if got := InstalledComponentRoot("rho-log"); got != want {
		t.Fatalf("expected the newest installed component %s, got %s", want, got)
	}
}

func TestRhoHomeFollowsTheEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RHO_HOME", home)
	if got := RhoHome(); got != home {
		t.Fatalf("expected the Rho home %s, got %s", home, got)
	}
}

func TestVendoredComponentRootNeedsAManifest(t *testing.T) {
	repo := t.TempDir()
	writeRepository(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, "modules", "components", "rho-log"), 0755); err != nil {
		t.Fatalf("mkdir component: %v", err)
	}

	if root := VendoredComponentRoot(filepath.Join(repo, "applications", "registry"), "rho-log"); root != "" {
		t.Fatalf("expected no component without a manifest, got %s", root)
	}
}

func TestVendoredComponentRootStopsAtTheRepository(t *testing.T) {
	// A components directory above the repository belongs to some other
	// project, so it must not decide what this project compiles.
	outer := t.TempDir()
	writeComponent(t, filepath.Join(outer, "modules", "components", "rho-log"))
	repo := filepath.Join(outer, "repo")
	writeRepository(t, repo)
	project := filepath.Join(repo, "applications", "registry")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	if root := VendoredComponentRoot(project, "rho-log"); root != "" {
		t.Fatalf("expected the search to stop at the repository, got %s", root)
	}
}

func TestVendoredComponentRootRejectsALinkedDirectory(t *testing.T) {
	repo := t.TempDir()
	writeRepository(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, "modules", "components"), 0755); err != nil {
		t.Fatalf("mkdir component tree: %v", err)
	}
	elsewhere := t.TempDir()
	writeComponent(t, filepath.Join(elsewhere, "rho-log"))
	if err := os.Symlink(filepath.Join(elsewhere, "rho-log"), filepath.Join(repo, "modules", "components", "rho-log")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	project := filepath.Join(repo, "applications", "registry")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	if root := VendoredComponentRoot(project, "rho-log"); root != "" {
		t.Fatalf("expected a linked component directory to be refused, got %s", root)
	}
}

func TestVendoredComponentRootRejectsATraversingName(t *testing.T) {
	repo := t.TempDir()
	writeRepository(t, repo)
	writeComponent(t, filepath.Join(repo, "modules", "components", "rho-log"))

	if root := VendoredComponentRoot(repo, "../rho-log"); root != "" {
		t.Fatalf("expected a traversing name to be refused, got %s", root)
	}
}

func TestVendoredComponentRootOutsideARepository(t *testing.T) {
	if root := VendoredComponentRoot(t.TempDir(), "rho-log"); root != "" {
		t.Fatalf("expected no component outside a repository, got %s", root)
	}
}
