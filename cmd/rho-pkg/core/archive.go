//
// @file archive.go
// @brief Reading a package from bytes rather than from a file.
//
// A package arrives two ways: from disk, when a user publishes a directory, and
// from the network, when a module is fetched from a registry. The second has no
// file to open, and writing one out to read it back would be a copy of every
// package on the way past.
//
// The reading here is the same tar.gz the packer writes, so a package read this
// way is exactly the package the writer produced. What is added is asking what a
// package *is* before it is written anywhere: the kind of module it holds is the
// directory it will be installed under, and a package whose kind cannot be read
// is one that should not be half installed.
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
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// UnpackBytes extracts a package held in memory into a destination directory.
//
// Paths are confined to the destination: an archive that names a path climbing
// out of it is refused rather than written, because a package arrives from a
// registry and a registry is not trusted to keep its own archive honest.
func UnpackBytes(archive []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("not a package: %w", err)
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("damaged package: %w", err)
		}
		out, err := confinedPath(dest, header.Name)
		if err != nil {
			return err
		}
		if header.FileInfo().IsDir() {
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		// The mode travels with the file, so a component's command arrives
		// executable rather than needing a chmod after every install.
		mode := header.FileInfo().Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		file, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		if _, err := io.Copy(file, reader); err != nil {
			_ = file.Close()
			return fmt.Errorf("cannot write %s: %w", header.Name, err)
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
}

// ReadArchiveManifest reads the manifest a package carries.
//
// It is how a caller finds out what it is holding before it writes anything: the
// kind of module a package holds decides where it is installed, and an archive
// without a manifest is not a package.
func ReadArchiveManifest(archive []byte) (ModuleYAML, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return ModuleYAML{}, fmt.Errorf("not a package: %w", err)
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ModuleYAML{}, fmt.Errorf("damaged package: %w", err)
		}
		if header.Name != "module.yml" {
			continue
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			return ModuleYAML{}, fmt.Errorf("cannot read the manifest: %w", err)
		}
		return ParseModuleYAML(string(content))
	}
	return ModuleYAML{}, fmt.Errorf("the package carries no module.yml")
}

// confinedPath joins an archive entry name onto a destination, refusing a name
// that would leave it.
func confinedPath(dest, name string) (string, error) {
	// A tar entry name is a slash-separated path, which is not what Windows
	// uses, so it is converted before it is joined.
	cleaned := filepath.Clean(filepath.FromSlash(name))
	if cleaned == "." || filepath.IsAbs(cleaned) ||
		cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("the package contains a path outside it: %s", name)
	}
	out := filepath.Join(dest, cleaned)
	// The join is checked as well, because a name that survived the check above
	// can still land elsewhere through a symbolic-link-like form the cleaning
	// did not remove.
	if !strings.HasPrefix(out, filepath.Clean(dest)+string(filepath.Separator)) {
		return "", fmt.Errorf("the package contains a path outside it: %s", name)
	}
	return out, nil
}
