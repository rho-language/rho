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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func PackProject(dir string) (string, error) {
	return PackProjectWithBinary(dir, "")
}

/* -------------------------------------------------------------------------- */
/*  Pack Project with Pre-built Binary                                         */
/* -------------------------------------------------------------------------- */

func PackProjectWithBinary(dir string, prebuiltBinaryPath string) (string, error) {
	// Load manifest
	manifestPath := filepath.Join(dir, "module.yml")
	manifest, err := LoadModuleYAML(manifestPath)
	if err != nil {
		return "", err
	}

	var binaryPath string

	// Use pre-built binary if provided, otherwise build
	if prebuiltBinaryPath != "" {
		binaryPath = prebuiltBinaryPath
	} else if manifest.Module.Type == "component" || manifest.Module.Type == "plugin" {
		binaryPath, err = BuildBinary(dir, manifest)
		if err != nil {
			return "", fmt.Errorf("build binary: %w", err)
		}
	}

	// Collect files: module.yml + binary (if component/plugin)
	files, err := collectFiles(dir, binaryPath, manifest)
	if err != nil {
		return "", err
	}

	// Output file name with platform suffix
	var pkgName string
	if binaryPath != "" {
		pkgName = fmt.Sprintf("%s-%s-%s-%s.rho-pkg",
			manifest.Module.Name,
			manifest.Module.Version,
			runtime.GOOS,
			runtime.GOARCH,
		)
	} else {
		pkgName = fmt.Sprintf("%s-%s.rho-pkg",
			manifest.Module.Name,
			manifest.Module.Version,
		)
	}
	pkgPath := filepath.Join(dir, pkgName)

	// Create archive
	if err := writeArchive(pkgPath, files, dir); err != nil {
		return "", err
	}

	// Note: Don't clean up binary here - caller is responsible

	return pkgPath, nil
}

/* -------------------------------------------------------------------------- */
/*  Build Binary (exported for reuse)                                          */
/* -------------------------------------------------------------------------- */

func BuildBinary(dir string, manifest ModuleYAML) (string, error) {
	// Determine binary output name
	binaryName := manifest.Module.Name
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(dir, binaryName)

	// Determine the Go package to build
	// First, read the module path from go.mod
	modPath := filepath.Join(dir, "go.mod")
	var modulePath string
	if _, err := os.Stat(modPath); err == nil {
		data, err := os.ReadFile(modPath)
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module"))
					break
				}
			}
		}
	}

	// Find main package - could be a src/ directory or just a Go module here.
	var mainPkg string
	if hasGoSources(filepath.Join(dir, "src")) {
		// Main is in src/ - use module path + /src
		if modulePath != "" {
			mainPkg = modulePath + "/src"
		} else {
			mainPkg = "./src"
		}
	} else if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		// Go module at root
		if modulePath != "" {
			mainPkg = modulePath
		} else {
			mainPkg = "."
		}
	} else {
		return "", fmt.Errorf("no main.go or go.mod found")
	}

	// Build with go build
	cmd := exec.Command("go", "build", "-o", binaryPath, mainPkg)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build failed: %w", err)
	}

	return binaryPath, nil
}

// hasGoSources reports whether a directory holds any Go file. The entry point
// is not always called main.go, so a single fixed name would miss a component
// whose main lives in a differently named file.
func hasGoSources(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			return true
		}
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  Collect Files                                                              */
/* -------------------------------------------------------------------------- */

func collectFiles(dir string, binaryPath string, manifest ModuleYAML) ([]string, error) {
	var out []string

	// Required: module.yml
	modPath := filepath.Join(dir, "module.yml")
	if _, err := os.Stat(modPath); err != nil {
		return nil, fmt.Errorf("missing required file: module.yml")
	}
	out = append(out, modPath)

	// Required: binary (only for component/plugin)
	if binaryPath != "" {
		if _, err := os.Stat(binaryPath); err != nil {
			return nil, fmt.Errorf("missing binary: %s", binaryPath)
		}
		out = append(out, binaryPath)
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
