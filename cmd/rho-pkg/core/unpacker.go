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
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// UnpackArchive extracts a package on disk into a destination directory.
//
// It reads the file and hands the stream to the same extraction a package from a
// registry goes through. This function used to have its own loop, with a bare
// join of the destination and the entry name — which is how an entry named
// `../../x` wrote outside the destination, from `rho pkg unpack` and from every
// `rho module publish` that installed what it had just packed. One extraction, one
// set of rules: confinement, only files and directories, and the size bounds.
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

	if err := extract(tar.NewReader(expanded(gz)), dest); err != nil {
		return err
	}

	// Read module.yml to find the binary name and make it executable, for a package
	// that was packed before the mode travelled with the file.
	modPath := filepath.Join(dest, "module.yml")
	if _, err := os.Stat(modPath); err == nil {
		manifest, err := LoadModuleYAML(modPath)
		if err == nil && manifest.Main != "" {
			binaryName := filepath.Base(manifest.Main)
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
