//
// @file unpacker.go
// @brief Rho Package Unpacker - Enterprise v3.
//
// Extracts `.rho-pkg` archives into a destination directory.
//
// Archive format: tar.gz
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
    "fmt"
    "io"
    "os"
    "path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func UnpackArchive(src, dest string) error {
    f, err := os.Open(src)
    if err != nil {
        return fmt.Errorf("cannot open archive: %w", err)
    }
    defer f.Close()

    gz, err := gzip.NewReader(f)
    if err != nil {
        return fmt.Errorf("gzip error: %w", err)
    }
    defer gz.Close()

    tr := tar.NewReader(gz)

    var binaryName string

    for {
        hdr, err := tr.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return fmt.Errorf("tar error: %w", err)
        }

        outPath := filepath.Join(dest, hdr.Name)

        if hdr.FileInfo().IsDir() {
            if err := os.MkdirAll(outPath, 0755); err != nil {
                return fmt.Errorf("mkdir error: %w", err)
            }
            continue
        }

        // Ensure directory exists
        if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
            return fmt.Errorf("mkdir error: %w", err)
        }

        // Write file
        outFile, err := os.Create(outPath)
        if err != nil {
            return fmt.Errorf("create error: %w", err)
        }

        if _, err := io.Copy(outFile, tr); err != nil {
            outFile.Close()
            return fmt.Errorf("copy error: %w", err)
        }

        outFile.Close()

        // Track binary name from module.yml
        if hdr.Name == "module.yml" {
            // We'll read it after extraction to find the binary name
        }
    }

    // Read module.yml to find binary name and make it executable
    modPath := filepath.Join(dest, "module.yml")
    if _, err := os.Stat(modPath); err == nil {
        manifest, err := LoadModuleYAML(modPath)
        if err == nil && manifest.Main != "" {
            binaryName = filepath.Base(manifest.Main)
            binaryPath := filepath.Join(dest, binaryName)
            if _, err := os.Stat(binaryPath); err == nil {
                // Make binary executable
                if err := os.Chmod(binaryPath, 0755); err != nil {
                    return fmt.Errorf("chmod binary: %w", err)
                }
            }
        }
    }

    return nil
}
