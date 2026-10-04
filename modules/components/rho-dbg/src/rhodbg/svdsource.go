//
// @file svd.go — continued: where a register map comes from when nothing has one.
// @brief The cache, and the one fetch into it.
//
// A chip's register map is a vendor's file, and the same file whichever program is
// being debugged. So it is cached once per machine rather than fetched per
// project or carried per checkout: a project that vendors one still uses its own,
// because a file somebody committed to a repository is a fact, and the cache is
// only what happens when nothing has one.
//
// The fetch is one file over HTTP, not a clone. The CMSIS device data repository is
// every vendor's registers for every part — gigabytes, and mostly parts nobody is
// debugging — and a session that needed one file from it has no business taking the
// rest.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodbg

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// How long a fetch may take before it is given up on.
//
// Generous, because a register map is four megabytes and a first fetch is over
// whatever connection the machine has; and bounded, because a session that waits
// forever for a file it does not need to debug anything is worse than a session that
// says the map is missing.
const fetchTimeout = 2 * time.Minute

// How large a fetched file may be.
//
// A cap rather than a trust: a repository can be pointed at anything, and a session
// that asked for a register map should not end up writing a disk's worth of
// anything into a cache. A register map is a few megabytes, so this leaves room and
// still refuses what is not one.
const fetchLimit = 64 << 20

// SVDSource is a file in the repository a register map is fetched from: a path
// within it, as the repository itself writes it.
//
// `STMicro/STM32H7x3.svd` — a vendor directory and a file, because that is how the
// repository is laid out and the layout is what a person reading a target spec will
// recognise. It is the repository's naming and not ours, deliberately: a spec that
// names a file by a name of our own is a spec nobody can check against the thing it
// names.
type SVDSource struct {
	// Path is within the repository, as the repository writes it.
	Path string
	// Vendor is the first element of it, which is the cache's directory and what a
	// listing of the cache is sorted by.
	Vendor string
	// Device is the file name.
	Device string
}

// ParseSVDSource is a repository path as a target spec writes it.
func ParseSVDSource(declared string) (SVDSource, bool) {
	trimmed := strings.Trim(strings.TrimSpace(declared), "/")
	if trimmed == "" {
		return SVDSource{}, false
	}
	// An https URL is accepted as well as a repository path, because a person
	// copying a link from a browser has a URL and a person reading a spec has a
	// path, and both mean the same file.
	trimmed = strings.TrimPrefix(trimmed, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return SVDSource{}, false
	}
	// `data/<vendor>/<file>` is where the repository keeps them, and a spec that
	// names the file the way the repository does not have to know where in it they
	// are. The prefix is added once, by the fetcher, rather than being required of
	// every spec that names a file.
	device := parts[len(parts)-1]
	vendor := parts[len(parts)-2]
	// The suffix is optional, and a person copying a name out of a repository's
	// listing leaves it off. Restored rather than refused, because a spec that names
	// a device the repository has is a spec that has said which file it means, and
	// the only file of that device is the one with the suffix on it.
	if !strings.HasSuffix(strings.ToLower(device), ".svd") {
		device += ".svd"
	}
	return SVDSource{
		Path:   strings.Join(append(parts[len(parts)-2:len(parts)-1], device), "/"),
		Vendor: vendor,
		Device: device,
	}, true
}

// repositoryPath is the file within the repository: the vendor tree, under the
// directory the repository keeps it in.
func (s SVDSource) repositoryPath() string {
	return "data/" + s.Path
}

// rawURL is the file's address, built from the repository the machine is
// configured with.
//
// A repository is named as a person names it — the GitHub page — and the file's
// address within it is derived from that, so a machine that points at its own
// mirror gets its own mirror. A `.git` suffix is trimmed for the same reason.
func (s SVDSource) rawURL(repository string, ref string) string {
	base := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(repository), "/"), ".git")
	base = strings.TrimSuffix(strings.TrimSuffix(base, "/"), "/")
	if ref == "" {
		ref = DefaultRef
	}
	return fmt.Sprintf("%s/raw/%s/%s", base, ref, s.repositoryPath())
}

// CachePath is where this file is kept on this machine.
//
// Under the cache directory, in the repository's own layout, so that
// `cache/svd/STMicro/STM32H7x3.svd` says which file it is without anything else
// having to be read to find out.
func (s SVDSource) CachePath(settings Settings) string {
	directory := settings.CacheDirectory()
	if directory == "" {
		return ""
	}
	return filepath.Join(directory, filepath.FromSlash(s.Path))
}

/* -------------------------------------------------------------------------- */
/*  Fetching                                                                   */
/* -------------------------------------------------------------------------- */

// EnsureRegisterMap is the register map for a source: the cached file, or the file
// fetched once and cached.
//
// The check is a stat and the fetch is one file. Nothing is fetched that is already
// here, so a machine that has debugged this chip once has it for every project
// afterwards, and a machine with no network pays the cost once and never again.
func EnsureRegisterMap(source SVDSource, settings Settings) (string, error) {
	cached := source.CachePath(settings)
	if cached == "" {
		return "", fmt.Errorf("this component has nowhere to keep a register map: name svd_cache in %s, or let the project carry its own", settingsFile())
	}
	if info, err := os.Stat(cached); err == nil && info.Size() > 0 {
		return cached, nil
	}

	// Not fetching is a decision, and a session that needs the file has to hear it
	// rather than find out by waiting.
	if !settings.Fetchable() {
		return "", fmt.Errorf(
			"the register map %s is not in the cache and this component will not fetch one: %s",
			source.Path, fetchDisabledReason(settings))
	}

	if err := os.MkdirAll(filepath.Dir(cached), 0o755); err != nil {
		return "", fmt.Errorf("make the cache directory: %w", err)
	}
	address := source.rawURL(settings.Repository, settings.Ref)
	if err := download(address, cached); err != nil {
		return "", err
	}
	return cached, nil
}

// CachedRegisterMap is the register map for a source if this machine already has
// it, and nothing at all if it does not.
//
// The one that never reaches for the network, for a caller that is asking what is
// available rather than making it available: `rho dbg check` wants to know whether
// a session will have one without making it fetch four megabytes to find out.
func CachedRegisterMap(source SVDSource, settings Settings) (string, bool) {
	cached := source.CachePath(settings)
	if cached == "" {
		return "", false
	}
	info, err := os.Stat(cached)
	if err != nil || info.Size() == 0 {
		return "", false
	}
	return cached, true
}

// fetchDisabledReason is why this component will not fetch, in a sentence that
// says which of the three settings is responsible.
func fetchDisabledReason(settings Settings) string {
	switch {
	case !settings.Fetch:
		return "svd_fetch is false"
	case strings.TrimSpace(settings.Repository) == "":
		return "svd_repository is empty"
	default:
		return fmt.Sprintf("svd_cache is %q and there is no Rho home to put it under", settings.Cache)
	}
}

// settingsFile is where this component's values are, for a failure that tells a
// person which file to change. The manifest says which file that is; this is the
// name it says when it could not be read.
func settingsFile() string {
	return filepath.Join(ModuleRoot(), "config", "dbg.yml")
}

// download is one file fetched into the cache.
//
// Written to a temporary name beside the cache and renamed into place, so that two
// sessions on this machine at once cannot leave a half-written register map where a
// whole one belongs — and so that a fetch that fails leaves nothing behind that the
// next one would believe.
func download(address string, into string) error {
	client := &http.Client{Timeout: fetchTimeout}
	response, err := client.Get(address)
	if err != nil {
		return fmt.Errorf("fetch the register map from %s: %w", address, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch the register map from %s: the repository answered %s", address, response.Status)
	}

	temporary, err := os.CreateTemp(filepath.Dir(into), filepath.Base(into)+".*.partial")
	if err != nil {
		return fmt.Errorf("write into the register map cache: %w", err)
	}
	written, copyErr := io.Copy(temporary, io.LimitReader(response.Body, fetchLimit))
	closeErr := temporary.Close()
	if copyErr != nil {
		os.Remove(temporary.Name())
		return fmt.Errorf("fetch the register map from %s: %w", address, copyErr)
	}
	if closeErr != nil {
		os.Remove(temporary.Name())
		return fmt.Errorf("write into the register map cache: %w", closeErr)
	}
	if written == 0 {
		os.Remove(temporary.Name())
		return fmt.Errorf("fetch the register map from %s: the file was empty", address)
	}
	if err := os.Rename(temporary.Name(), into); err != nil {
		os.Remove(temporary.Name())
		return fmt.Errorf("put the register map in the cache: %w", err)
	}
	return nil
}

// ForgetRegisterMap takes a register map out of the cache, so the next one asked for
// is fetched again.
//
// For the case where the cached file is the wrong one — a device line whose
// registers changed, or a file that was fetched from a mirror that had the wrong
// revision — which is the only thing that makes a cached answer wrong and the only
// thing `svd --refresh` is for.
func ForgetRegisterMap(source SVDSource, settings Settings) error {
	cached := source.CachePath(settings)
	if cached == "" {
		return fmt.Errorf("this component has nowhere to keep a register map, so there is none to forget")
	}
	if _, err := os.Stat(cached); err != nil {
		return fmt.Errorf("there is no cached register map for %s", source.Path)
	}
	if err := os.Remove(cached); err != nil {
		return fmt.Errorf("take %s out of the cache: %w", cached, err)
	}
	return nil
}
