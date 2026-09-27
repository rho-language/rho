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
	} else if manifest.Layout().Binary {
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
	// The package lands in the build directory too, so a failed run leaves
	// nothing in the source tree. A library package has no binary, so nothing
	// has created the directory yet.
	artifacts := ArtifactDir(dir)
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		return "", fmt.Errorf("cannot create the build directory: %w", err)
	}
	pkgPath := filepath.Join(artifacts, pkgName)

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

// PackageLayout describes what a published archive contains.
//
// It is data rather than a hardcoded list, so a component, a plugin, an
// application and a library can each be described differently, and a new kind
// is a layout rather than a new branch in the packager.
type PackageLayout struct {
	// Kind is the module kind this layout describes.
	Kind string
	// Binary says whether the archive carries a compiled command.
	Binary bool
	// Include lists the paths, relative to the project, that go in. A path may
	// be a file or a directory, and a directory is taken whole.
	Include []string
	// Require lists the paths that must exist, failing the pack otherwise.
	Require []string
	// ArtifactDir is where the build writes intermediate files.
	ArtifactDir string
	// Interface lists the header files a library package carries. When set, the
	// package is the manifest, the documentation and these files alone.
	Interface []string
	// Exclude lists file extensions left out of the package. The command is
	// shipped prebuilt and a caller reads only the header, so the host sources
	// are not needed in an installed component, and leaving them out means
	// there is nothing there for someone to edit.
	Exclude []string
}

// layouts are the defaults, per module kind. A manifest may override any of them
// with a `package:` section.
var layouts = map[string]PackageLayout{
	"component": {
		Kind:    "component",
		Binary:  true,
		Include: []string{"module.yml", "README.md", "src", "lib", "config"},
		Require: []string{"module.yml", "README.md", "src"},
	},
	"plugin": {
		Kind:    "plugin",
		Binary:  true,
		Include: []string{"module.yml", "README.md", "src", "lib", "plugin", "config"},
		Require: []string{"module.yml", "README.md", "src"},
	},
	"application": {
		// An application is a Rho program, so it carries no command binary;
		// the rho toolchain builds it.
		Kind:    "application",
		Include: []string{"module.yml", "README.md", "src", "config", "project.yml"},
		Require: []string{"module.yml", "src"},
	},
	"library": {
		Kind:    "library",
		Include: []string{"module.yml", "README.md", "src", "lib"},
		Require: []string{"module.yml", "src"},
	},
}

// LayoutFor returns the layout for a module type, falling back to a component
// so an unknown kind still packs something sensible.
func LayoutFor(moduleType string) PackageLayout {
	if layout, ok := layouts[moduleType]; ok {
		return layout
	}
	return layouts["component"]
}

// WithOverrides applies a manifest's `package:` section to a layout, so a
// project can add or replace entries without the packager changing.
func (l PackageLayout) WithOverrides(kind string, binary bool, hasBinary bool, artifactDir string, include, require []string) PackageLayout {
	if kind != "" {
		l.Kind = kind
	}
	if hasBinary {
		l.Binary = binary
	}
	if artifactDir != "" {
		l.ArtifactDir = artifactDir
	}
	if len(include) > 0 {
		l.Include = include
	}
	if len(require) > 0 {
		l.Require = require
	}
	return l
}

// ArtifactDir is where a build writes its intermediate artefacts: the
// compiled binary and the packed archives.
//
// They are deliberately not written beside the sources, so a run that fails
// part way does not leave a package sitting in the source tree, and the
// directory is ignored by git.
func ArtifactDirFor(dir string, manifest ModuleYAML) string {
	if location := manifest.Layout().ArtifactDir; location != "" {
		return filepath.Join(dir, filepath.FromSlash(location))
	}
	return filepath.Join(dir, "build")
}

func ArtifactDir(dir string) string { return filepath.Join(dir, "build") }

func BuildBinary(dir string, manifest ModuleYAML) (string, error) {
	// Determine binary output name
	binaryName := manifest.Module.Name
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	// The artifact path is made absolute because it is named twice, from two
	// places: `go build` reads it from the directory the build runs in, and the
	// caller reads it from its own. A relative directory would resolve them
	// differently and the caller would look for the binary where it is not.
	artifacts, err := filepath.Abs(ArtifactDir(dir))
	if err != nil {
		return "", fmt.Errorf("cannot resolve the build directory: %w", err)
	}
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		return "", fmt.Errorf("cannot create the build directory: %w", err)
	}
	binaryPath := filepath.Join(artifacts, binaryName)

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

// collectFiles gathers what a binary package holds: the layout's entries, plus
// the command when the layout says this kind of module has one.
func collectFiles(dir string, binaryPath string, manifest ModuleYAML) ([]string, error) {
	files, err := CollectLayoutFiles(dir, manifest.Layout())
	if err != nil {
		return nil, err
	}
	if binaryPath == "" {
		return files, nil
	}
	if _, err := os.Stat(binaryPath); err != nil {
		return nil, fmt.Errorf("missing binary: %s", binaryPath)
	}
	return append(files, binaryPath), nil
}

// CollectLayoutFiles resolves a layout into the files it names, taking a
// directory whole. Both packers use it, so a module's shape is described once.
func CollectLayoutFiles(dir string, layout PackageLayout) ([]string, error) {
	var out []string
	for _, required := range layout.Require {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(required))); err != nil {
			return nil, fmt.Errorf("missing required file: %s", required)
		}
	}
	// A package carries the manifest, the documentation, the interface and the
	// prebuilt command. The host sources are build input, not output, so they
	// are excluded by default.
	exclude := layout.Exclude
	if len(exclude) == 0 {
		exclude = []string{".go"}
	}
	_ = exclude

	seen := map[string]bool{}
	for _, entry := range layout.Include {
		path := filepath.Join(dir, filepath.FromSlash(entry))
		info, err := os.Stat(path)
		if err != nil {
			// An optional entry that is absent is not an error.
			continue
		}
		if !info.IsDir() {
			if !seen[path] {
				out = append(out, path)
				seen[path] = true
			}
			continue
		}
		filepath.Walk(path, func(walked string, walkedInfo os.FileInfo, walkErr error) error {
			if walkErr != nil || walkedInfo.IsDir() || seen[walked] {
				return nil
			}
			if excludedExtension(walked, exclude) {
				return nil
			}
			out = append(out, walked)
			seen[walked] = true
			return nil
		})
	}
	return out, nil
}

// excludedExtension reports whether a path carries an extension the layout
// leaves out.
func excludedExtension(path string, exclude []string) bool {
	extension := filepath.Ext(path)
	for _, unwanted := range exclude {
		if extension == unwanted {
			return true
		}
	}
	return false
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

	// A build artefact is written flat in the archive, whatever folder it was
	// produced in, so the installed module keeps the shape callers expect.
	//
	// The comparison is made on resolved paths. A build artefact is named by
	// the build and its files are named by the layout, and the two are not
	// always spelled the same way: one is often absolute and the other
	// relative to the process. A prefix that does not match would write the
	// artefact under a path escaping the module, and unpacking it would fail.
	artifacts, err := filepath.Abs(ArtifactDir(baseDir))
	if err != nil {
		return fmt.Errorf("cannot resolve the build directory: %w", err)
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("cannot resolve %s: %w", baseDir, err)
	}

	for _, file := range files {
		resolved, resolveErr := filepath.Abs(file)
		if resolveErr != nil {
			return fmt.Errorf("cannot resolve %s: %w", file, resolveErr)
		}
		rel, relErr := filepath.Rel(base, resolved)
		if relErr != nil {
			return fmt.Errorf("cannot place %s in the package: %w", file, relErr)
		}
		if strings.HasPrefix(resolved, artifacts+string(filepath.Separator)) {
			rel = filepath.Base(resolved)
		}
		file = resolved
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
