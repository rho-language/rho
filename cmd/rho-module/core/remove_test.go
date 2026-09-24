package core

import (
    "os"
    "path/filepath"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Remove Dependency                                                    */
/* -------------------------------------------------------------------------- */

func TestRemoveDependency(t *testing.T) {
    tmp := t.TempDir()

    // rho.lock
    os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte(`
lock:
  math: "1.0.0"
`), 0644)

    // .rho-deps/math@1.0.0
    dep := filepath.Join(tmp, ".rho-deps", "math@1.0.0")
    os.MkdirAll(dep, 0755)
    os.WriteFile(filepath.Join(dep, "module.yml"), []byte("module: math"), 0644)

    if err := RemoveDependency(tmp, "math"); err != nil {
        t.Fatalf("RemoveDependency failed: %v", err)
    }

    // Directory removed
    if _, err := os.Stat(dep); err == nil {
        t.Fatalf("dependency directory still exists")
    }

    // Lockfile updated
    data, _ := os.ReadFile(filepath.Join(tmp, "rho.lock"))
    if string(data) != "lock:\n" {
        t.Fatalf("rho.lock not updated")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Version Mismatch                                                     */
/* -------------------------------------------------------------------------- */

func TestRemoveDependencyVersionMismatch(t *testing.T) {
    tmp := t.TempDir()

    os.WriteFile(filepath.Join(tmp, "rho.lock"), []byte(`
lock:
  math: "1.0.0"
`), 0644)

    if err := RemoveDependency(tmp, "math@2.0.0"); err == nil {
        t.Fatalf("expected version mismatch error")
    }
}
