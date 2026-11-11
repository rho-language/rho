package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const expandManifest = `module:
  name: app
  version: 0.1.0
  type: module
  description: x

main: src/main.c

depends:
  - name: lib
    type: c
    path: ${RHO_WORKSPACE}/lib
    git: https://example.invalid/lib
    branch: main

c_connection:
  library: lib
  include_dirs:
    - ${RHO_WORKSPACE}/lib/include
`

func TestManifestPathsExpandAgainstTheWorkspace(t *testing.T) {
	ws := t.TempDir()
	t.Setenv("RHO_WORKSPACE", ws)
	dir := t.TempDir()
	file := filepath.Join(dir, "module.yml")
	if err := os.WriteFile(file, []byte(expandManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Deps[0].Path; got != filepath.Join(ws, "lib") {
		t.Errorf("depends path = %q", got)
	}
	if got := m.Connection.IncludeDirs[0]; got != ws+"/lib/include" {
		t.Errorf("include dir = %q", got)
	}
}

func TestManifestWithAnUnsetVariableNamesIt(t *testing.T) {
	t.Setenv("NOPE", "")
	dir := t.TempDir()
	file := filepath.Join(dir, "module.yml")
	os.WriteFile(file, []byte(strings.ReplaceAll(expandManifest, "RHO_WORKSPACE", "NOPE")), 0o644)
	if _, err := LoadManifest(file); err == nil || !strings.Contains(err.Error(), "NOPE") {
		t.Fatalf("want an error naming NOPE, got %v", err)
	}
}

func TestLocatorDoesNotCloneIntoForeignAbsolutePaths(t *testing.T) {
	if within("/home/other/x", t.TempDir()) {
		t.Fatal("a foreign path is not within the project")
	}
	root := t.TempDir()
	if !within(filepath.Join(root, "a", "b"), root) || within(root+"x", root) {
		t.Fatal("within is wrong")
	}
}
