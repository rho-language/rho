package registry

import (
	"os"
	"path/filepath"
	"testing"

	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Test: BuildIndex                                                           */
/* -------------------------------------------------------------------------- */

func TestBuildIndex(t *testing.T) {
	tmp := t.TempDir()
	paths.SetTestRhoHome(tmp)
	defer paths.SetTestRhoHome("")

	if err := paths.EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths failed: %v", err)
	}

	// Initialize local registry
	registryPath, _ := paths.GetRegistryPath()
	registryFile := filepath.Join(registryPath, "registry.yaml")
	if err := os.MkdirAll(registryPath, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	testRegistry := `components:
  - name: math
    version: 1.2.0
plugins:
  - name: http
    version: 0.9.0
active_components:
  - name: math
    version: 1.2.0
active_plugins:
  - name: http
    version: 0.9.0
`
	os.WriteFile(registryFile, []byte(testRegistry), 0644)

	index, err := BuildIndex()
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}

	if len(index) == 0 {
		t.Fatalf("expected non-empty index")
	}
}
