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
	_, err := DownloadModuleDigest(spec)
	return err
}

// DownloadModuleDigest downloads a module into the cache and returns the digest
// its package hashed to, for a caller that records it in a lockfile. A module
// already cached is not fetched again; its digest is read from what was
// recorded beside it when it was.
func DownloadModuleDigest(spec string) (string, error) {
	name, version := parseSpec(spec)

	// Step 1: Resolve version
	resolved, err := resolveRemoteVersion(name, version)
	if err != nil {
		return "", fmt.Errorf("cannot resolve version: %w", err)
	}

	// Check if already in global cache
	cachePath, err := paths.GetCachePath()
	if err != nil {
		return "", err
	}
	cacheDir := filepath.Join(cachePath, fmt.Sprintf("%s@%s", name, resolved))
	if _, err := os.Stat(cacheDir); err == nil {
		// Already cached, skip download. The digest is read from the copy beside
		// it, or is empty on a cache written before digests were kept.
		return readCachedDigest(cacheDir), nil
	}

	// Step 2: Download archive, checked against the registry's digest
	archive, digest, err := downloadArchive(name, resolved)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	// A download that turns out not to be an archive is discarded here, while
	// the file still exists. Caching first would move it away, leaving nothing
	// to inspect.
	defer os.Remove(archive)

	// Step 3: Validate archive
	if err := validateArchive(archive); err != nil {
		return "", fmt.Errorf("invalid archive: %w", err)
	}

	// Step 4: Store in cache
	if err := storeInCache(name, resolved, archive, digest); err != nil {
		return "", fmt.Errorf("cache error: %w", err)
	}

	return digest, nil
}

/* -------------------------------------------------------------------------- */
/*  Parse Spec                                                                 */
/* -------------------------------------------------------------------------- */

// parseSpec splits `name[@version]`.
//
// Only the first `@` separates them, so a version keeps whatever follows it. It
// used to split on every `@` and keep two parts, so `math@1.2.0@extra` quietly
// became `math@1.2.0` — a different request from the one typed, answered as if it
// were the same. Now the version is everything after the first `@`, so the request
// that goes to the registry is the one that was typed, and a registry that has no
// version called `1.2.0@extra` answers that it has none.
func parseSpec(spec string) (string, string) {
	if name, version, found := strings.Cut(spec, "@"); found {
		return name, version
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

// downloadArchive fetches a module, checks it against the digest the registry
// recorded, and returns the file it wrote and that digest.
//
// The check is here, before the package is cached, so a package that does not
// agree with what was published never becomes the copy this machine installs
// from. A registry that records no digest is not refused — it answers "", and
// the package is taken as it was before digests were kept — but an unreachable
// registry is an error, so a failure cannot be turned into a skipped check.
func downloadArchive(name, version string) (string, string, error) {
	// The archive is fetched through the configured remote, so the registry a
	// user points at is the one that is used.
	data, err := remote.FetchModule(name, version)
	if err != nil {
		return "", "", err
	}
	record, err := remote.RecordedVersion(name, version)
	if err != nil {
		return "", "", err
	}
	digest, err := verifyPackage(name, version, data, record)
	if err != nil {
		return "", "", err
	}

	// A file this process created and nobody else could have put there. The name
	// used to be `<tmp>/<name>-<version>.rho-module` and was opened with
	// os.Create, which follows a symbolic link: on a shared /tmp anyone could plant
	// one at that predictable name and have this write the package through it. A
	// random name, opened exclusively and mode 0600, has no such name to plant.
	out, err := os.CreateTemp("", fmt.Sprintf("rho-%s-*.rho-module", sanitiseForFile(name)))
	if err != nil {
		return "", "", err
	}
	tmp := out.Name()

	if _, err := out.Write(data); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return "", "", err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", "", err
	}

	return tmp, digest, nil
}

// sanitiseForFile is a module name made safe to put in a file name. A name that
// reaches here has passed the registry client's checks, but a path separator in a
// temp-file pattern is an error rather than a character, so it is not left to chance.
func sanitiseForFile(name string) string {
	var out strings.Builder
	for _, character := range name {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9', character == '-', character == '_', character == '.':
			out.WriteRune(character)
		default:
			out.WriteByte('_')
		}
	}
	return out.String()
}

/* -------------------------------------------------------------------------- */
/*  Store in Cache                                                             */
/* -------------------------------------------------------------------------- */

func storeInCache(name, version, archive, digest string) error {
	cachePath, err := paths.GetCachePath()
	if err != nil {
		return err
	}
	cacheDir := filepath.Join(cachePath, fmt.Sprintf("%s@%s", name, version))

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}

	dst := filepath.Join(cacheDir, filepath.Base(archive))
	if err := os.Rename(archive, dst); err != nil {
		return err
	}
	// The digest is kept beside the package, so a cache hit can be written into
	// a lockfile without fetching the module again.
	if digest != "" {
		if err := os.WriteFile(filepath.Join(cacheDir, "package.sha256"), []byte(digest+"\n"), 0644); err != nil {
			return err
		}
	}
	return nil
}

// readCachedDigest returns the digest recorded beside a cached module, or "".
func readCachedDigest(cacheDir string) string {
	data, err := os.ReadFile(filepath.Join(cacheDir, "package.sha256"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
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
