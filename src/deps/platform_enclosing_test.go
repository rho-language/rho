package deps

import (
	"os"
	"path/filepath"
	"testing"
)

// A board inside its platform module, depending on nothing, belongs to the
// platform that encloses it; a folder with no enclosing platform has none.
func TestABoardBelongsToTheEnclosingPlatform(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "acme-board", "boards", "one"), 0o755)
	os.WriteFile(filepath.Join(root, "acme-board", "module.yml"), []byte("module:\n  name: acme-board\n  type: platform\n  version: 0.1.0\nplatform: acme\n"), 0o644)
	os.WriteFile(filepath.Join(root, "acme-board", "boards", "one", "module.yml"), []byte("module:\n  name: one\n  type: platform-board\n  version: 0.1.0\n"), 0o644)
	if got := Platform(filepath.Join(root, "acme-board", "boards", "one")); got != "acme" {
		t.Fatalf("the board's platform is %q", got)
	}
	os.MkdirAll(filepath.Join(root, "lonely"), 0o755)
	os.WriteFile(filepath.Join(root, "lonely", "module.yml"), []byte("module:\n  name: lonely\n  type: module\n  version: 0.1.0\n"), 0o644)
	if got := Platform(filepath.Join(root, "lonely")); got != "" {
		t.Fatalf("a module with no platform anywhere got %q", got)
	}
}

// A project named relatively — `.` from inside it, `../<name>` from beside it —
// still finds a platform through a sibling it depends on (`../acme-common`),
// whose `..` would otherwise be refused as a path that climbs.
func TestAPlatformIsFoundFromARelativelyNamedProject(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "acme-platform"), 0o755)
	os.MkdirAll(filepath.Join(root, "acme-common"), 0o755)
	os.MkdirAll(filepath.Join(root, "acme-things"), 0o755)
	os.WriteFile(filepath.Join(root, "acme-platform", "module.yml"), []byte("module:\n  name: acme-platform\n  type: platform\n  version: 0.1.0\nplatform: acme\n"), 0o644)
	os.WriteFile(filepath.Join(root, "acme-common", "module.yml"), []byte("module:\n  name: acme-common\n  type: interfaces\n  version: 0.1.0\nplatforms:\n  - name: acme-platform\n    path: ../acme-platform\n"), 0o644)
	os.WriteFile(filepath.Join(root, "acme-things", "module.yml"), []byte("module:\n  name: acme-things\n  type: collection\n  version: 0.1.0\ndepends:\n  - name: acme-common\n    type: c\n    path: ../acme-common\n"), 0o644)
	previous, _ := os.Getwd()
	defer os.Chdir(previous)
	if err := os.Chdir(filepath.Join(root, "acme-things")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".", "./", "../acme-things", filepath.Join(root, "acme-things")} {
		if got := Platform(name); got != "acme" {
			t.Errorf("Platform(%q) is %q, want acme", name, got)
		}
	}
}
