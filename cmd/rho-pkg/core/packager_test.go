//
// @file packager_test.go
// @brief Tests for Rho Package Packager - Enterprise v3.
//
// Validates:
//   - pack creation
//   - required files
//   - src/ inclusion
//   - config/ optional inclusion
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package core

import (
    "archive/tar"
    "compress/gzip"
    "os"
    "path/filepath"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Successful Packaging                                                 */
/* -------------------------------------------------------------------------- */

func TestPackProject(t *testing.T) {
    tmp := t.TempDir()

    // module.yml
    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: hello-rho
  type: rho
  version: 0.1.0

main: src/main.rho
`), 0644)

    // README.md
    os.WriteFile(filepath.Join(tmp, "README.md"), []byte("Hello"), 0644)

    // src/main.rho
    os.Mkdir(filepath.Join(tmp, "src"), 0755)
    os.WriteFile(filepath.Join(tmp, "src", "main.rho"), []byte("module main"), 0644)

    // config/app.yml
    os.Mkdir(filepath.Join(tmp, "config"), 0755)
    os.WriteFile(filepath.Join(tmp, "config", "app.yml"), []byte("app: test"), 0644)

    pkgPath, err := PackProject(tmp)
    if err != nil {
        t.Fatalf("PackProject failed: %v", err)
    }

    // Open archive
    f, err := os.Open(pkgPath)
    if err != nil {
        t.Fatalf("cannot open archive: %v", err)
    }
    defer f.Close()

    gz, err := gzip.NewReader(f)
    if err != nil {
        t.Fatalf("gzip error: %v", err)
    }
    defer gz.Close()

    tr := tar.NewReader(gz)

    found := map[string]bool{}

    for {
        hdr, err := tr.Next()
        if err != nil {
            break
        }
        found[hdr.Name] = true
    }

    // Check required files
    if !found["module.yml"] {
        t.Fatalf("module.yml missing from archive")
    }
    if !found["README.md"] {
        t.Fatalf("README.md missing from archive")
    }
    if !found["src/main.rho"] {
        t.Fatalf("src/main.rho missing from archive")
    }
    if !found["config/app.yml"] {
        t.Fatalf("config/app.yml missing from archive")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Missing Required Files                                               */
/* -------------------------------------------------------------------------- */

func TestPackMissingManifest(t *testing.T) {
    tmp := t.TempDir()

    // Missing module.yml
    os.WriteFile(filepath.Join(tmp, "README.md"), []byte("Hello"), 0644)
    os.Mkdir(filepath.Join(tmp, "src"), 0755)
    os.WriteFile(filepath.Join(tmp, "src", "main.rho"), []byte("module main"), 0644)

    if _, err := PackProject(tmp); err == nil {
        t.Fatalf("expected error for missing module.yml")
    }
}
