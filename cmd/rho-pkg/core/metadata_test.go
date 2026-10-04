//
// @file metadata_test.go
// @brief Tests for Rho Package Metadata Reader - Enterprise v3.
//
// Validates:
//   - metadata extraction from .rho-pkg archives
//   - error handling for missing module.yml
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

func createMetaArchive(path string) error {
	f, _ := os.Create(path)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	// module.yml
	content := `
module:
  name: hello-rho
  type: rho
  version: 0.1.0

main: src/main.rho
`
	tw.WriteHeader(&tar.Header{
		Name: "module.yml",
		Mode: 0644,
		Size: int64(len(content)),
	})
	tw.Write([]byte(content))

	tw.Close()
	gz.Close()
	f.Close()
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Test: Successful Metadata Read                                             */
/* -------------------------------------------------------------------------- */

func TestReadPackageMetadata(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "test.rho-pkg")

	createMetaArchive(archive)

	meta, err := ReadPackageMetadata(archive)
	if err != nil {
		t.Fatalf("ReadPackageMetadata failed: %v", err)
	}

	if meta.Module.Name != "hello-rho" {
		t.Fatalf("wrong module name")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Missing module.yml                                                   */
/* -------------------------------------------------------------------------- */

func TestReadPackageMetadataMissing(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "test.rho-pkg")

	// Create empty archive
	f, _ := os.Create(archive)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.Close()
	gz.Close()
	f.Close()

	if _, err := ReadPackageMetadata(archive); err == nil {
		t.Fatalf("expected error for missing module.yml")
	}
}
