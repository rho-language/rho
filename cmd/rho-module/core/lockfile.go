//
// @file lockfile.go
// @brief Rho Lockfile Manager - Enterprise v3.
//
// Handles reading, writing, and validating the `rho.lock` file.
// The lockfile stores resolved dependency versions:
//
//   lock:
//     math: "1.2.0"
//     io: "1.1.0"
//
// Responsibilities:
//   - load lockfile into map[string]string
//   - write lockfile from map[string]string
//   - validate lockfile structure
//   - used by installer, updater, remover
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func LoadLockfile(dir string) (map[string]string, error) {
	path := filepath.Join(dir, "rho.lock")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read rho.lock: %w", err)
	}

	return parseLockfile(string(data)), nil
}

func WriteLockfile(dir string, deps map[string]string) error {
	return WriteLockfileWithHashes(dir, deps, nil)
}

// WriteLockfileWithHashes writes the resolved versions, and the digest each
// package hashed to when it was fetched, in a `sha256:` section.
//
// The digest is what makes a lock reproducible: the versions say what to
// install, and the digest says which bytes those versions were, so a module
// altered where it is kept no longer matches the lock that recorded it. The
// section is separate so a lockfile written before digests were kept stays
// exactly what it was.
func WriteLockfileWithHashes(dir string, deps map[string]string, hashes map[string]string) error {
	out := "lock:\n"
	names := make([]string, 0, len(deps))
	for name := range deps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out += fmt.Sprintf("  %s: \"%s\"\n", name, deps[name])
	}

	digested := make([]string, 0, len(hashes))
	for name, digest := range hashes {
		if digest != "" {
			digested = append(digested, name)
		}
	}
	if len(digested) > 0 {
		sort.Strings(digested)
		out += "sha256:\n"
		for _, name := range digested {
			out += fmt.Sprintf("  %s: \"%s\"\n", name, hashes[name])
		}
	}

	path := filepath.Join(dir, "rho.lock")
	return os.WriteFile(path, []byte(out), 0644)
}

// LoadLockfileHashes returns the package digests a lockfile recorded, which is
// empty for a lock written before digests were kept.
func LoadLockfileHashes(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, "rho.lock"))
	if err != nil {
		return nil
	}
	return parseLockfileSection(string(data), "sha256:")
}

/* -------------------------------------------------------------------------- */
/*  Internal: Parse Lockfile                                                   */
/* -------------------------------------------------------------------------- */

func parseLockfile(data string) map[string]string {
	return parseLockfileSection(data, "lock:")
}

// parseLockfileSection reads the entries under one heading. The file has more
// than one section — versions under `lock:`, digests under `sha256:` — and a
// reader that read the whole file as one would take a digest for a version.
func parseLockfileSection(data, heading string) map[string]string {
	out := map[string]string{}

	lines := strings.Split(data, "\n")
	inSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// A heading is a word and a colon and nothing after it.
		if strings.HasSuffix(line, ":") && !strings.Contains(line, ": ") {
			inSection = line == heading
			continue
		}
		if !inSection {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		if name != "" && value != "" {
			out[name] = value
		}
	}

	return out
}

/* -------------------------------------------------------------------------- */
/*  Validate Lockfile                                                          */
/* -------------------------------------------------------------------------- */

func ValidateLockfile(dir string) error {
	lock, err := LoadLockfile(dir)
	if err != nil {
		return err
	}

	for name, version := range lock {
		if !IsValidSemver(version) {
			return fmt.Errorf("invalid version in lockfile: %s=%s", name, version)
		}
	}

	return nil
}
