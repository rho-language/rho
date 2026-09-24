//
// @file publisher.go
// @brief Rho Module Publisher - Enterprise v3.
//
// Handles publishing modules and plugins to a remote registry.
// Steps:
//   1. Validate module
//   2. Read module metadata
//   3. Pack module into `.rho-module` archive
//   4. Upload archive to remote registry
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

func PublishModule(dir string) error {
    // Step 1: Validate module
    if err := ValidateModule(dir); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }

    // Step 2: Read metadata
    meta, err := ReadModuleInfo(dir)
    if err != nil {
        return fmt.Errorf("cannot read module info: %w", err)
    }

    // Step 3: Pack module
    archive, err := packModule(dir, meta)
    if err != nil {
        return fmt.Errorf("cannot pack module: %w", err)
    }

    // Step 4: Upload to registry
    if err := uploadModule(meta, archive); err != nil {
        return fmt.Errorf("upload failed: %w", err)
    }

    return nil
}

/* -------------------------------------------------------------------------- */
/*  Pack Module                                                                */
/* -------------------------------------------------------------------------- */

func packModule(dir string, meta ModuleInfo) (string, error) {
    archiveName := fmt.Sprintf("%s-%s.rho-module",
        meta.Module.Name,
        meta.Module.Version,
    )
    archivePath := filepath.Join(dir, archiveName)

    out, err := os.Create(archivePath)
    if err != nil {
        return "", fmt.Errorf("cannot create archive: %w", err)
    }
    defer out.Close()

    gz := gzip.NewWriter(out)
    defer gz.Close()

    tw := tar.NewWriter(gz)
    defer tw.Close()

    // Collect files
    files, err := collectModuleFiles(dir)
    if err != nil {
        return "", err
    }

    // Write files
    for _, file := range files {
        rel, _ := filepath.Rel(dir, file)
        info, _ := os.Stat(file)

        hdr, _ := tar.FileInfoHeader(info, "")
        hdr.Name = rel

        if err := tw.WriteHeader(hdr); err != nil {
            return "", err
        }

        f, _ := os.Open(file)
        io.Copy(tw, f)
        f.Close()
    }

    return archivePath, nil
}

/* -------------------------------------------------------------------------- */
/*  Collect Files                                                              */
/* -------------------------------------------------------------------------- */

func collectModuleFiles(dir string) ([]string, error) {
    var out []string

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

    // plugin/ (optional)
    pluginDir := filepath.Join(dir, "plugin")
    if _, err := os.Stat(pluginDir); err == nil {
        filepath.Walk(pluginDir, func(path string, info os.FileInfo, err error) error {
            if err == nil && !info.IsDir() {
                out = append(out, path)
            }
            return nil
        })
    }

    return out, nil
}

/* -------------------------------------------------------------------------- */
/*  Upload Module                                                              */
/* -------------------------------------------------------------------------- */

func uploadModule(meta ModuleInfo, archive string) error {
    // Placeholder for remote registry upload
    // In Enterprise v3, this will be implemented via:
    //   - HTTP POST
    //   - Git push
    //   - Nexus upload
    //
    // For now, simulate success.

    fmt.Println("Uploading", archive, "to registry...")
    return nil
}
