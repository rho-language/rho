//
// @file packager.go
// @brief Rho Package Packager - Enterprise v3.
//
// Creates `.rho-pkg` archives from a Rho project directory.
// Includes:
//   - module.yml
//   - src/
//   - config/ (optional)
//   - README.md
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

func PackProject(dir string) (string, error) {
    // Load manifest
    manifestPath := filepath.Join(dir, "module.yml")
    manifest, err := LoadModuleYAML(manifestPath)
    if err != nil {
        return "", err
    }

    // Collect files
    files, err := collectFiles(dir)
    if err != nil {
        return "", err
    }

    // Output file name
    pkgName := fmt.Sprintf("%s-%s.rho-pkg",
        manifest.Module.Name,
        manifest.Module.Version,
    )
    pkgPath := filepath.Join(dir, pkgName)

    // Create archive
    if err := writeArchive(pkgPath, files, dir); err != nil {
        return "", err
    }

    return pkgPath, nil
}

/* -------------------------------------------------------------------------- */
/*  Collect Files                                                              */
/* -------------------------------------------------------------------------- */

func collectFiles(dir string) ([]string, error) {
    var out []string

    // Required files
    required := []string{
        "module.yml",
        "README.md",
    }

    for _, f := range required {
        path := filepath.Join(dir, f)
        if _, err := os.Stat(path); err != nil {
            return nil, fmt.Errorf("missing required file: %s", f)
        }
        out = append(out, path)
    }

    // src/
    srcDir := filepath.Join(dir, "src")
    if _, err := os.Stat(srcDir); err != nil {
        return nil, fmt.Errorf("missing src/ directory")
    }

    filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
        if err == nil && !info.IsDir() {
            out = append(out, path)
        }
        return nil
    })

    // config/ (optional)
    configDir := filepath.Join(dir, "config")
    if _, err := os.Stat(configDir); err == nil {
        filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
            if err == nil && !info.IsDir() {
                out = append(out, path)
            }
            return nil
        })
    }

    return out, nil
}

/* -------------------------------------------------------------------------- */
/*  Write Archive                                                              */
/* -------------------------------------------------------------------------- */

func writeArchive(pkgPath string, files []string, baseDir string) error {
    out, err := os.Create(pkgPath)
    if err != nil {
        return fmt.Errorf("cannot create archive: %w", err)
    }
    defer out.Close()

    gz := gzip.NewWriter(out)
    defer gz.Close()

    tarw := tar.NewWriter(gz)
    defer tarw.Close()

    for _, file := range files {
        rel, _ := filepath.Rel(baseDir, file)

        info, err := os.Stat(file)
        if err != nil {
            return err
        }

        hdr, err := tar.FileInfoHeader(info, "")
        if err != nil {
            return err
        }

        hdr.Name = rel

        if err := tarw.WriteHeader(hdr); err != nil {
            return err
        }

        f, err := os.Open(file)
        if err != nil {
            return err
        }

        if _, err := io.Copy(tarw, f); err != nil {
            f.Close()
            return err
        }

        f.Close()
    }

    return nil
}
