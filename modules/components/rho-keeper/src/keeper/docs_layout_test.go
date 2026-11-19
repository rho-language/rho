package keeper

import (
	"os"
	"path/filepath"
	"testing"
)

// An installed module that keeps its pages in docs/doc/ has documentation: the
// badge and the page find docs/doc/index.md as they find docs/index.md.
func TestAModuleWithDocsDocHasDocs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RHO_HOME", home)
	dir := filepath.Join(home, "modules", "installed", "component", "rho-sample@0.1.0")
	if err := os.MkdirAll(filepath.Join(dir, "docs", "doc"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "module.yml"), []byte("module:\n  name: rho-sample\n  type: component\n  version: 0.1.0\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "docs", "doc", "index.md"), []byte("# Sample\n"), 0o644)
	got := ModuleDocFile("rho-sample")
	if got != filepath.Join(dir, "docs", "doc", "index.md") {
		t.Fatalf("expected docs/doc/index.md, got %q", got)
	}
}
