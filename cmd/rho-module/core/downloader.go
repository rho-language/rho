//
// @file downloader.go
// @brief Rho Module Downloader - Enterprise v3.
//
// Downloads modules from a remote registry and stores them in the
// global cache directory (`~/.rho/pkg/mod`).
//
// Steps:
//   1. Parse module spec (name[@version])
//   2. Resolve version via remote registry
//   3. Download archive
//   4. Store archive in global cache
//   5. Validate archive
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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"rho/cmd/rho-module/remote"
	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func DownloadModule(spec string) error {
	name, version := parseSpec(spec)

	// Step 1: Resolve version
	resolved, err := resolveRemoteVersion(name, version)
	if err != nil {
		return fmt.Errorf("cannot resolve version: %w", err)
	}

	// Check if already in global cache
	cachePath, err := paths.GetCachePath()
	if err != nil {
		return err
	}
	cacheDir := filepath.Join(cachePath, fmt.Sprintf("%s@%s", name, resolved))
	if _, err := os.Stat(cacheDir); err == nil {
		// Already cached, skip download
		return nil
	}

	// Step 2: Download archive
	archive, err := downloadArchive(name, resolved)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	// A download that turns out not to be an archive is discarded here, while
	// the file still exists. Caching first would move it away, leaving nothing
	// to inspect.
	defer os.Remove(archive)

	// Step 3: Validate archive
	if err := validateArchive(archive); err != nil {
		return fmt.Errorf("invalid archive: %w", err)
	}

	// Step 4: Store in cache
	if err := storeInCache(name, resolved, archive); err != nil {
		return fmt.Errorf("cache error: %w", err)
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/*  Parse Spec                                                                 */
/* -------------------------------------------------------------------------- */

func parseSpec(spec string) (string, string) {
	if strings.Contains(spec, "@") {
		parts := strings.Split(spec, "@")
		return parts[0], parts[1]
	}
	return spec, "" // latest
}

/* -------------------------------------------------------------------------- */
/*  Resolve Version (Remote Registry)                                          */
/* -------------------------------------------------------------------------- */

// resolveRemoteVersion picks the version to download. An explicit version is
// used as given; otherwise the registry is asked which versions exist and the
// highest is chosen.
func resolveRemoteVersion(name, version string) (string, error) {
	if version != "" {
		return version, nil
	}

	versions, err := remote.AvailableVersions(name)
	if err != nil {
		return "", fmt.Errorf("cannot list versions of %s: %w", name, err)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("the registry has no published version of %s", name)
	}
	// The registry returns versions lowest first, so the last is the newest.
	return versions[len(versions)-1], nil
}

/* -------------------------------------------------------------------------- */
/*  Download Archive                                                           */
/* -------------------------------------------------------------------------- */

func downloadArchive(name, version string) (string, error) {
	// The archive is fetched through the configured remote, so the registry a
	// user points at is the one that is used.
	data, err := remote.FetchModule(name, version)
	if err != nil {
		return "", err
	}

	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s.rho-module", name, version))
	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}

	if _, err := out.Write(data); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}

	return tmp, nil
}

/* -------------------------------------------------------------------------- */
/*  Store in Cache                                                             */
/* -------------------------------------------------------------------------- */

func storeInCache(name, version, archive string) error {
	cachePath, err := paths.GetCachePath()
	if err != nil {
		return err
	}
	cacheDir := filepath.Join(cachePath, fmt.Sprintf("%s@%s", name, version))

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	dst := filepath.Join(cacheDir, filepath.Base(archive))
	return os.Rename(archive, dst)
}

/* -------------------------------------------------------------------------- */
/*  Validate Archive                                                           */
/* -------------------------------------------------------------------------- */

func validateArchive(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	foundModule := false
	foundBinary := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if hdr.Name == "module.yml" {
			foundModule = true
		}
		// Check for binary file (executable)
		if !strings.HasPrefix(hdr.Name, "src/") && !hdr.FileInfo().IsDir() {
			foundBinary = true
		}
	}

	if !foundModule {
		return errors.New("module.yml missing")
	}
	if !foundBinary {
		return errors.New("binary missing")
	}

	return nil
}
