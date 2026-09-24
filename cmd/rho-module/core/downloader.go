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
// SPDX-License-Identifier: MIT
//

package core

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

	// Step 3: Store in cache
	if err := storeInCache(name, resolved, archive); err != nil {
		return fmt.Errorf("cache error: %w", err)
	}

	// Step 4: Validate archive
	if err := validateArchive(archive); err != nil {
		return fmt.Errorf("invalid archive: %w", err)
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

func resolveRemoteVersion(name, version string) (string, error) {
    // Placeholder: always return version or "1.0.0"
    if version != "" {
        return version, nil
    }
    return "1.0.0", nil
}

/* -------------------------------------------------------------------------- */
/*  Download Archive                                                           */
/* -------------------------------------------------------------------------- */

func downloadArchive(name, version string) (string, error) {
    url := fmt.Sprintf("https://registry.rho-lang.org/%s/%s.rho-module", name, version)

    resp, err := http.Get(url)
    if err != nil {
        return "", fmt.Errorf("http error: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        return "", fmt.Errorf("registry returned %d", resp.StatusCode)
    }

    tmp := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s.rho-module", name, version))
    out, err := os.Create(tmp)
    if err != nil {
        return "", err
    }

    io.Copy(out, resp.Body)
    out.Close()

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
