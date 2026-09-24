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

func createTestModuleArchive(path string) {
    f, _ := os.Create(path)
    gz := gzip.NewWriter(f)
    tw := tar.NewWriter(gz)

    tw.WriteHeader(&tar.Header{
        Name: "module.yml",
        Mode: 0644,
        Size: int64(len("module: test")),
    })
    tw.Write([]byte("module: test"))

    tw.WriteHeader(&tar.Header{
        Name: "src/lib.rho",
        Mode: 0644,
        Size: int64(len("module test")),
    })
    tw.Write([]byte("module test"))

    tw.Close()
    gz.Close()
    f.Close()
}

/* -------------------------------------------------------------------------- */
/*  Test: Validate Archive                                                     */
/* -------------------------------------------------------------------------- */

func TestValidateArchive(t *testing.T) {
    tmp := t.TempDir()
    archive := filepath.Join(tmp, "test.rho-module")

    createTestModuleArchive(archive)

    if err := validateArchive(archive); err != nil {
        t.Fatalf("validateArchive failed: %v", err)
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Missing module.yml                                                   */
/* -------------------------------------------------------------------------- */

func TestValidateArchiveMissingModule(t *testing.T) {
    tmp := t.TempDir()
    archive := filepath.Join(tmp, "bad.rho-module")

    f, _ := os.Create(archive)
    gz := gzip.NewWriter(f)
    tw := tar.NewWriter(gz)

    tw.WriteHeader(&tar.Header{
        Name: "src/lib.rho",
        Mode: 0644,
        Size: int64(len("module test")),
    })
    tw.Write([]byte("module test"))

    tw.Close()
    gz.Close()
    f.Close()

    if err := validateArchive(archive); err == nil {
        t.Fatalf("expected error for missing module.yml")
    }
}
