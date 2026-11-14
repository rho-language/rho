package cli

import (
	"os"
	"path/filepath"
	"testing"

	"rho/internal/paths"
	"rho/src/deps"
)

// An installed library module is enabled by its name, like a component: it
// resolves as a dependency only once it is, so `enable` has to find it.
func TestALibraryModuleCanBeEnabled(t *testing.T) {
	home := t.TempDir()
	paths.SetTestRhoHome(home)
	t.Cleanup(func() { paths.SetTestRhoHome("") })
	dir := filepath.Join(home, "modules", "installed", deps.KindModule, "acme-shapes@0.2.0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "module.yml"), []byte("module:\n  name: acme-shapes\n  type: module\n  version: 0.2.0\nmain: src/main.rho\n"), 0o644)
	if err := Enable([]string{"acme-shapes"}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	state, err := deps.LoadEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if !state.IsEnabled(deps.KindModule, "acme-shapes", "0.2.0") {
		t.Fatalf("the library module is not enabled")
	}
}
