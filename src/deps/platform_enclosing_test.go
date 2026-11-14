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
