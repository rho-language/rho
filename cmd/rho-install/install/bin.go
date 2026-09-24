//
// @file bin.go
// @brief Rho Install - Binary deployment subsystem.
//
// Responsibilities:
//   - Copy Rho executables into ~/.rho/bin
//   - Validate source binaries
//   - Ensure correct permissions
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package install

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// InstallBinary copies a single binary into ~/.rho/bin.
func InstallBinary(src string) error {
    binPath, err := GetBinPath()
    if err != nil {
        return err
    }

    if _, err := os.Stat(src); os.IsNotExist(err) {
        return fmt.Errorf("source binary does not exist: %s", src)
    }

    dst := filepath.Join(binPath, filepath.Base(src))

    // Avoid accidental overwrite
    if _, err := os.Stat(dst); err == nil {
        return fmt.Errorf("binary already exists: %s", dst)
    }

    in, err := os.Open(src)
    if err != nil {
        return fmt.Errorf("cannot open source binary: %w", err)
    }
    defer in.Close()

    out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o755)
    if err != nil {
        return fmt.Errorf("cannot create destination binary: %w", err)
    }
    defer out.Close()

    if _, err := io.Copy(out, in); err != nil {
        return fmt.Errorf("failed to copy binary: %w", err)
    }

    return nil
}

// InstallAllBinaries installs all Rho executables.
func InstallAllBinaries(srcDir string) error {
    binaries := []string{
        "rho",
        "rho-pkg",
        "rho-fmt",
        "rho-test",
        "rho-deps",
        "rho-doc",
    }

    for _, b := range binaries {
        src := filepath.Join(srcDir, b)
        if err := InstallBinary(src); err != nil {
            return fmt.Errorf("failed to install %s: %w", b, err)
        }
    }

    return nil
}
