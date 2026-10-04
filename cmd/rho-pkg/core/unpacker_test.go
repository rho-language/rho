//
// @file unpacker_test.go
// @brief Tests for Rho Package Unpacker - Enterprise v3.
//
// Validates:
//   - archive unpacking
//   - file extraction
//   - directory creation
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
/*  Helper: Create Test Archive                                                */
/* -------------------------------------------------------------------------- */

func createTestArchive(path string) error {
    f, _ := os.Create(path)
    gz := gzip.NewWriter(f)
    tw := tar.NewWriter(gz)

    // module.yml
    tw.WriteHeader(&tar.Header{
        Name: "module.yml",
        Mode: 0644,
        Size: int64(len("module: test")),
    })
    tw.Write([]byte("module: test"))

    // src/main.rho
    tw.WriteHeader(&tar.Header{
        Name: "src/main.rho",
        Mode: 0644,
        Size: int64(len("module main")),
    })
    tw.Write([]byte("module main"))

    tw.Close()
    gz.Close()
    f.Close()
    return nil
}

/* -------------------------------------------------------------------------- */
/*  Test: Successful Unpack                                                    */
/* -------------------------------------------------------------------------- */

func TestUnpackArchive(t *testing.T) {
    tmp := t.TempDir()
    archive := filepath.Join(tmp, "test.rho-pkg")

    createTestArchive(archive)

    dest := filepath.Join(tmp, "out")

    if err := UnpackArchive(archive, dest); err != nil {
        t.Fatalf("UnpackArchive failed: %v", err)
    }

    if _, err := os.Stat(filepath.Join(dest, "module.yml")); err != nil {
        t.Fatalf("module.yml missing")
    }

    if _, err := os.Stat(filepath.Join(dest, "src", "main.rho")); err != nil {
        t.Fatalf("src/main.rho missing")
    }
}
