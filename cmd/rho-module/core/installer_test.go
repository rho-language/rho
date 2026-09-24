package core

import (
    "os"
    "path/filepath"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Install Dependencies                                                 */
/* -------------------------------------------------------------------------- */

func TestInstallDependencies(t *testing.T) {
    tmp := t.TempDir()

    // module.yml
    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: app
  type: rho
  version: 0.1.0

main: src/main.rho

deps:
  math: "1.0.0"
`), 0644)

    // src/main.rho
    os.Mkdir(filepath.Join(tmp, "src"), 0755)
    os.WriteFile(filepath.Join(tmp, "src", "main.rho"), []byte("module app"), 0644)

    // Fake global cache module
    global := filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod", "math@1.0.0")
    os.MkdirAll(global, 0755)
    os.WriteFile(filepath.Join(global, "module.yml"), []byte("module: math"), 0644)

    if err := InstallDependencies(tmp); err != nil {
        t.Fatalf("InstallDependencies failed: %v", err)
    }

    // Check local cache
    if _, err := os.Stat(filepath.Join(tmp, ".rho-deps", "math@1.0.0")); err != nil {
        t.Fatalf("math@1.0.0 not installed locally")
    }

    // Check lockfile
    if _, err := os.Stat(filepath.Join(tmp, "rho.lock")); err != nil {
        t.Fatalf("rho.lock missing")
    }
}
