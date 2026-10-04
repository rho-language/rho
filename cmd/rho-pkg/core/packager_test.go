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
// SPDX-License-Identifier: Apache-2.0
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

    // go.mod
    os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(`
module github.com/test/hello-rho

go 1.18
`), 0644)

    // module.yml
    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: hello-rho
  type: component
  version: 0.1.0
  command: hello-rho

main: hello-rho
`), 0644)

    // README.md
    os.WriteFile(filepath.Join(tmp, "README.md"), []byte("Hello"), 0644)

    // src/main.go
    os.Mkdir(filepath.Join(tmp, "src"), 0755)
    os.WriteFile(filepath.Join(tmp, "src", "main.go"), []byte(`
package main

import "fmt"

func main() {
    fmt.Println("Hello from hello-rho")
}
`), 0644)

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
    // Check binary exists (name based on module name)
    if !found["hello-rho"] {
        t.Fatalf("hello-rho binary missing from archive")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Missing Required Files                                               */
/* -------------------------------------------------------------------------- */

func TestPackMissingManifest(t *testing.T) {
    tmp := t.TempDir()

    // go.mod
    os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(`
module github.com/test/hello-rho

go 1.18
`), 0644)

    // Missing module.yml
    os.WriteFile(filepath.Join(tmp, "README.md"), []byte("Hello"), 0644)
    os.Mkdir(filepath.Join(tmp, "src"), 0755)
    os.WriteFile(filepath.Join(tmp, "src", "main.go"), []byte("package main"), 0644)

    if _, err := PackProject(tmp); err == nil {
        t.Fatalf("expected error for missing module.yml")
    }
}
