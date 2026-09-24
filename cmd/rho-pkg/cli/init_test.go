//
// @file init_test.go
// @brief Tests for `rho pkg init` - Enterprise v3.
//
// Validates:
//   - project creation
//   - module.yml generation
//   - error on existing manifest
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cli

import (
    "os"
    "path/filepath"
    "testing"
)

func TestInitCreatesProject(t *testing.T) {
    tmp := t.TempDir()

    if err := runInit(tmp); err != nil {
        t.Fatalf("runInit failed: %v", err)
    }

    if _, err := os.Stat(filepath.Join(tmp, "module.yml")); err != nil {
        t.Fatalf("module.yml missing")
    }

    if _, err := os.Stat(filepath.Join(tmp, "src", "main.rho")); err != nil {
        t.Fatalf("src/main.rho missing")
    }

    if _, err := os.Stat(filepath.Join(tmp, "README.md")); err != nil {
        t.Fatalf("README.md missing")
    }
}

func TestInitFailsIfManifestExists(t *testing.T) {
    tmp := t.TempDir()

    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte("module: test"), 0644)

    if err := runInit(tmp); err == nil {
        t.Fatalf("expected error when module.yml exists")
    }
}
