package deps

import (
	"os"
	"path/filepath"
	"testing"
)

// Helper: create a fake dependency with a rho.mod inside
func createFakeDep(t *testing.T, root string, name string, version string, deps []string) string {
	depPath := filepath.Join(root, name)
	if err := os.Mkdir(depPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	content := "module " + name + "\nversion " + version + "\n\n"
	for _, d := range deps {
		content += "dep " + d + " {}\n"
	}

	if err := os.WriteFile(filepath.Join(depPath, "rho.mod"), []byte(content), 0644); err != nil {
		t.Fatalf("write rho.mod: %v", err)
	}

	return depPath
}

func TestResolverSimple(t *testing.T) {
	tmp := t.TempDir()

	// Create fake dependency
	createFakeDep(t, tmp, "depA", "1.2.3", nil)

	manifest := &Manifest{
		ModuleName: "root",
		Version:    "0.1.0",
		Deps: []DepSpec{
			{
				Name:    "depA",
				Type:    DepTypeC,
				GitURL:  "https://example.com/depA.git",
				Version: VersionRange{Raw: ">=1.0.0 <2.0.0"},
				Path:    "depA",
			},
		},
	}

	loc := &DefaultLocator{
		ProjectRoot: tmp,
		GlobalCache: filepath.Join(tmp, "global"),
		LocalCache:  filepath.Join(tmp, "local"),
	}

	r := &Resolver{Locator: loc}

	graph, lock, err := r.Resolve(manifest, tmp)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	if len(graph.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(graph.Nodes))
	}

	if len(lock.Packages) != 1 {
		t.Fatalf("expected 1 locked package, got %d", len(lock.Packages))
	}

	p := lock.Packages[0]
	if p.Name != "depA" {
		t.Fatalf("expected depA, got %s", p.Name)
	}
	if p.Version != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %s", p.Version)
	}
}

func TestResolverVersionMismatch(t *testing.T) {
	tmp := t.TempDir()

	// Create fake dependency with version 3.0.0
	createFakeDep(t, tmp, "depX", "3.0.0", nil)

	manifest := &Manifest{
		ModuleName: "root",
		Version:    "0.1.0",
		Deps: []DepSpec{
			{
				Name:    "depX",
				Type:    DepTypeC,
				GitURL:  "https://example.com/depX.git",
				Version: VersionRange{Raw: ">=1.0.0 <2.0.0"},
				Path:    "depX",
			},
		},
	}

	loc := &DefaultLocator{
		ProjectRoot: tmp,
		GlobalCache: filepath.Join(tmp, "global"),
		LocalCache:  filepath.Join(tmp, "local"),
	}

	r := &Resolver{Locator: loc}

	_, _, err := r.Resolve(manifest, tmp)
	if err == nil {
		t.Fatalf("expected version mismatch error")
	}
}

func TestResolverCRepositoryWithoutRhoManifest(t *testing.T) {
	tmp := t.TempDir()
	cPath := filepath.Join(tmp, "clib")
	if err := os.Mkdir(cPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cPath, "include.h"), []byte("#define VALUE 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Resolver{Locator: &DefaultLocator{ProjectRoot: tmp}}
	graph, lock, err := r.Resolve(&Manifest{Deps: []DepSpec{{
		Name: "clib", Type: DepTypeC, Path: "clib", Version: VersionRange{Raw: ">=1.0.0"},
	}}}, tmp)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if graph.Nodes["clib"].Type != DepTypeC || len(lock.Packages) != 1 {
		t.Fatalf("expected one C dependency, got graph=%v lock=%v", graph.Nodes, lock.Packages)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Source Collection Is Deduplicated                                   */
/* -------------------------------------------------------------------------- */

// A manifest may list the same translation unit through both build.sources
// and a build.source_dirs scan. Passing it to the linker twice fails the link
// step with a multiple-definition error, so collection has to be deduplicated.
func TestResolverDeduplicatesSources(t *testing.T) {
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "clib")
	if err := os.MkdirAll(filepath.Join(lib, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "src", "error.c"), []byte("int error_fn(void) { return 0; }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := `module:
  name: clib
  type: interfaces
  version: 1.0.0
build:
  language: c
  source_dirs:
    - src
  sources:
    - src/error.c
`
	if err := os.WriteFile(filepath.Join(lib, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Resolver{Locator: &DefaultLocator{ProjectRoot: tmp}}
	graph, _, err := r.Resolve(&Manifest{Deps: []DepSpec{{
		Name: "clib", Type: DepTypeC, Path: "clib", Version: VersionRange{Raw: ">=1.0.0"},
	}}}, tmp)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	sources := graph.Nodes["clib"].SourceFiles
	seen := map[string]bool{}
	for _, source := range sources {
		if seen[source] {
			t.Fatalf("duplicate source collected: %q in %v", source, sources)
		}
		seen[source] = true
	}
	if len(sources) != 1 {
		t.Fatalf("expected one source, got %d: %v", len(sources), sources)
	}
}

// Include directories collected from a manifest are deduplicated too.
func TestResolverDeduplicatesIncludeDirs(t *testing.T) {
	tmp := t.TempDir()
	lib := filepath.Join(tmp, "clib")
	if err := os.MkdirAll(filepath.Join(lib, "include"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `module:
  name: clib
  type: interfaces
  version: 1.0.0
build:
  language: c
  include_dirs:
    - include
    - include
`
	if err := os.WriteFile(filepath.Join(lib, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Resolver{Locator: &DefaultLocator{ProjectRoot: tmp}}
	graph, _, err := r.Resolve(&Manifest{Deps: []DepSpec{{
		Name: "clib", Type: DepTypeC, Path: "clib", Version: VersionRange{Raw: ">=1.0.0"},
	}}}, tmp)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if got := len(graph.Nodes["clib"].IncludeDirs); got != 1 {
		t.Fatalf("expected one include dir, got %d: %v", got, graph.Nodes["clib"].IncludeDirs)
	}
}
