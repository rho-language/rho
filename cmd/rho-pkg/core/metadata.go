//
// @file metadata.go
// @brief Rho Package Metadata Reader - Enterprise v3.
//
// Extracts `module.yml` from a `.rho-pkg` archive without unpacking
// the entire project.
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
	"io"
	"os"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func ReadPackageMetadata(src string) (ModuleYAML, error) {
	f, err := os.Open(src)
	if err != nil {
		return ModuleYAML{}, fmt.Errorf("cannot open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return ModuleYAML{}, fmt.Errorf("gzip error: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ModuleYAML{}, fmt.Errorf("tar error: %w", err)
		}

		if hdr.Name == "module.yml" {
			// Read file content
			data, err := io.ReadAll(tr)
			if err != nil {
				return ModuleYAML{}, fmt.Errorf("read error: %w", err)
			}

			// Parse YAML
			tmp := os.TempDir() + "/rho-module.yml"
			os.WriteFile(tmp, data, 0644)

			return LoadModuleYAML(tmp)
		}
	}

	return ModuleYAML{}, fmt.Errorf("module.yml not found in archive")
}
