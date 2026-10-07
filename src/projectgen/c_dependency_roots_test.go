//
// @file c_dependency_roots_test.go
// @brief Rho project generator - C dependency root collection tests.
//
// A dependency reached through two parents is one root to add. The two parents
// are a common shape once a platform and a driver both depend on the same
// module, and adding its directory twice is two CMake targets of one name.
//
// SPDX-License-Identifier: Apache-2.0
//

package projectgen

import (
	"os"
	"path/filepath"
	"testing"
)

// writeModule writes a manifest with the given dependencies under dir and a
// CMakeLists.txt beside it, which is what marks the directory as a C root.
func writeModule(t *testing.T, dir string, name string, depends []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write CMakeLists.txt: %v", err)
	}

	body := "module:\n  name: " + name + "\n  type: platform\n  version: 0.1.0\n"
	if len(depends) > 0 {
		body += "depends:\n"
		for _, path := range depends {
			body += "  - name: " + filepath.Base(path) + "\n    type: c\n    path: " + path + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// A diamond — the application depends on two modules that both depend on a
// third — must add the shared root once. Two `add_subdirectory` calls for one
// source tree are two targets of one name, and the configure fails.
func TestCDependencyRootsDeduplicatesADiamond(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "app", []string{"a", "b"})
	writeModule(t, filepath.Join(root, "a"), "a", []string{"../common"})
	writeModule(t, filepath.Join(root, "b"), "b", []string{"../common"})
	writeModule(t, filepath.Join(root, "common"), "common", nil)

	roots := cDependencyRoots(root)

	common := filepath.Join(root, "common")
	count := 0
	for _, r := range roots {
		if r == common {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("the shared dependency appears %d times, want 1; roots=%v", count, roots)
	}
	if len(roots) != 3 {
		t.Fatalf("got %d roots, want 3 (a, common, b); roots=%v", len(roots), roots)
	}
}
