package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain gives the package's tests a Rho home of their own, with no remote
// registry: a test that publishes, enables or installs a module writes there
// and never into the home of the person running it, and never uploads.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "rho-module-core-home-")
	if err != nil {
		panic(err)
	}
	os.MkdirAll(filepath.Join(home, "config"), 0o755)
	os.WriteFile(filepath.Join(home, "config", "registry.yaml"), []byte("remote:\n  url: \"\"\n"), 0o644)
	os.Setenv("RHO_HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
