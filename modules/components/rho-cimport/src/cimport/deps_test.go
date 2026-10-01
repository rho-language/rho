//
// @file deps_test.go
// @brief What resolving a preset's C dependencies produces.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cimport

import (
	"os"
	"path/filepath"
	"testing"
)

// writeModule lays down a dependency with the manifest and directories a test
// needs, and returns its root.
func writeModule(t *testing.T, manifest string, dirs ...string) string {
	t.Helper()
	root := t.TempDir()
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}
	return root
}

// A dependency's manifest says what a consumer needs on its include path, so a
// program that depends on it does not restate those directories. A restated path
// is one that only means something on the machine that wrote it, which is what
// depending on a module is supposed to avoid.
func TestResolveReadsIncludeDirsFromDependencyManifest(t *testing.T) {
	root := writeModule(t, `
module:
  name: platform
build:
  include_dirs:
    - include/
    - include/robohub/hal/
`, "include/robohub/hal", "src")

	resolver := &Resolver{ProjectRoot: root, Cache: filepath.Join(root, ".cache")}
	resolution, err := resolver.Resolve(Preset{
		Name:         "board",
		Dependencies: []CDep{{Name: "platform", Path: root}},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	for _, want := range []string{"include", "include/robohub/hal"} {
		found := false
		for _, dir := range resolution.IncludeDirs {
			if dir == filepath.Join(root, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q on the include path, got %v", want, resolution.IncludeDirs)
		}
	}
}

// A path carrying a build-tool variable is left out rather than passed on
// unexpanded: the compiler would name a path nobody can find, and a missing
// include directory should be visible as a missing header rather than as a
// diagnostic about a directory that never existed.
func TestResolveSkipsUnexpandableIncludeDirs(t *testing.T) {
	root := writeModule(t, `
module:
  name: platform
build:
  include_dirs:
    - include/
    - ${CMAKE_BINARY_DIR}/generated/
`, "include")

	resolver := &Resolver{ProjectRoot: root, Cache: filepath.Join(root, ".cache")}
	resolution, err := resolver.Resolve(Preset{
		Name:         "board",
		Dependencies: []CDep{{Name: "platform", Path: root}},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	for _, dir := range resolution.IncludeDirs {
		if dir != filepath.Join(root, "include") {
			t.Errorf("expected only the resolvable directory, got %q", dir)
		}
	}
}

// The preset's own declarations and the dependency manifest's both apply, and a
// directory named by both is one directory: two copies would put the same header
// on the include path twice and make an include order decide which wins.
func TestResolveMergesManifestAndPresetIncludeDirs(t *testing.T) {
	root := writeModule(t, `
module:
  name: platform
build:
  include_dirs:
    - include/
`, "include")

	resolver := &Resolver{ProjectRoot: root, Cache: filepath.Join(root, ".cache")}
	resolution, err := resolver.Resolve(Preset{
		Name: "board",
		Dependencies: []CDep{{
			Name:    "platform",
			Path:    root,
			Include: []string{"include"},
		}},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	seen := 0
	for _, dir := range resolution.IncludeDirs {
		if dir == filepath.Join(root, "include") {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("expected the shared directory once, found it %d times in %v", seen, resolution.IncludeDirs)
	}
}

// A dependency with no manifest says nothing about its include path, and that is
// not an error: a vendor SDK is a directory of headers and the preset already
// says which of them the build wants.
func TestResolveToleratesDependencyWithoutManifest(t *testing.T) {
	root := writeModule(t, "")

	resolver := &Resolver{ProjectRoot: root, Cache: filepath.Join(root, ".cache")}
	resolution, err := resolver.Resolve(Preset{
		Name:         "board",
		Dependencies: []CDep{{Name: "bare", Path: root}},
	})
	if err != nil {
		t.Fatalf("a dependency with no manifest should resolve: %v", err)
	}
	if len(resolution.IncludeDirs) != 0 {
		t.Errorf("expected no include directories, got %v", resolution.IncludeDirs)
	}
}
