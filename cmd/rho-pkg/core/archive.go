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
// SPDX-License-Identifier: Apache-2.0
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

// What a package may be, as a bound.
//
// A package arrives from a registry, and a registry is not trusted to keep its own
// archive honest. A gzip stream can claim a few hundred bytes and expand to
// gigabytes, so what is written is counted rather than believed: the number of
// entries, the size of any one file, and the total are all capped. The caps are
// generous — a component's binary is tens of megabytes — and they are variables
// rather than constants only so a test can lower them instead of writing a
// gigabyte to prove they hold.
var (
	// maxPackageEntries is how many entries a package may carry.
	maxPackageEntries = 20000
	// maxPackageFileBytes is how large any one file in a package may be.
	maxPackageFileBytes int64 = 512 << 20
	// maxPackageBytes is how much a package may expand to in all.
	maxPackageBytes int64 = 1 << 30
	// maxManifestBytes is how large a package's module.yml may be.
	maxManifestBytes int64 = 1 << 20
)

// tarOverhead is room for the tar headers and block padding that sit around the
// content, so the stream limit is the content limit and not a smaller number.
const tarOverhead = 64 << 20

// expanded is a decompressed stream that stops being read once it is larger than a
// package may be, so that a bomb costs a bounded amount of time and memory rather
// than as much as its author chose.
func expanded(stream io.Reader) io.Reader {
	return io.LimitReader(stream, maxPackageBytes+tarOverhead)
}

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
	return extract(tar.NewReader(expanded(gz)), dest)
}

// extract writes a package's entries under a destination, and is the one place a
// package is turned into files.
//
// Both ways in — a package read from disk and one fetched from a registry — come
// here, because two copies of the rules are how one of them ends up without the
// confinement check. What it refuses, and why:
//
//   - a path that leaves the destination, or is absolute;
//   - anything that is not a regular file or a directory: a symbolic link or a
//     hard link is a way to make a later entry write somewhere the path check
//     never looked, and a device node has no business in a package;
//   - more entries, a larger file, or a larger total than a package may be.
func extract(reader *tar.Reader, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	var entries int
	var total int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("damaged package: %w", err)
		}
		entries++
		if entries > maxPackageEntries {
			return fmt.Errorf("the package has more than %d entries", maxPackageEntries)
		}

		switch header.Typeflag {
		case tar.TypeReg, tar.TypeDir:
			// The reader reports the old regular-file flag as TypeReg, so these
			// two are the whole of what is allowed.
		default:
			return fmt.Errorf("the package contains %s, which is not a file or a directory: %s",
				entryKind(header.Typeflag), header.Name)
		}

		// A directory entry for the root itself ("./") says nothing and is skipped.
		if header.Typeflag == tar.TypeDir && filepath.Clean(filepath.FromSlash(header.Name)) == "." {
			continue
		}
		out, err := confinedPath(dest, header.Name)
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
			continue
		}

		if header.Size > maxPackageFileBytes {
			return fmt.Errorf("%s is %d bytes, more than the %d a file in a package may be",
				header.Name, header.Size, maxPackageFileBytes)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		// The mode travels with the file, so a component's command arrives
		// executable rather than needing a chmod after every install. Only the
		// permission bits: a setuid bit in a package is not something to honour.
		mode := header.FileInfo().Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		file, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		// One byte past the cap, so that exceeding it is seen as exceeding it rather
		// than as a file that happens to be exactly that long.
		written, err := io.Copy(file, io.LimitReader(reader, maxPackageFileBytes+1))
		if err != nil {
			_ = file.Close()
			return fmt.Errorf("cannot write %s: %w", header.Name, err)
		}
		if err := file.Close(); err != nil {
			return err
		}
		if written > maxPackageFileBytes {
			return fmt.Errorf("%s is larger than the %d bytes a file in a package may be",
				header.Name, maxPackageFileBytes)
		}
		total += written
		if total > maxPackageBytes {
			return fmt.Errorf("the package expands to more than %d bytes", maxPackageBytes)
		}
	}
}

// entryKind names a tar entry type for a message.
func entryKind(flag byte) string {
	switch flag {
	case tar.TypeSymlink:
		return "a symbolic link"
	case tar.TypeLink:
		return "a hard link"
	case tar.TypeChar, tar.TypeBlock:
		return "a device"
	case tar.TypeFifo:
		return "a named pipe"
	default:
		return fmt.Sprintf("an entry of type %q", string(rune(flag)))
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

	reader := tar.NewReader(expanded(gz))
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
		// A manifest is a page of text. One larger than the cap is refused rather
		// than read, because this runs before anything else has looked at the package.
		content, err := io.ReadAll(io.LimitReader(reader, maxManifestBytes+1))
		if err != nil {
			return ModuleYAML{}, fmt.Errorf("cannot read the manifest: %w", err)
		}
		if int64(len(content)) > maxManifestBytes {
			return ModuleYAML{}, fmt.Errorf("the package's module.yml is larger than %d bytes", maxManifestBytes)
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
