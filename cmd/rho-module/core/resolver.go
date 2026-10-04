//
// @file resolver.go
// @brief Rho Module Dependency Resolver - Enterprise v3.
//
// Resolves dependency versions using:
//   - semantic version constraints
//   - remote registry
//   - local registry (cache)
//   - simulated registry index
//
// Responsibilities:
//   - parse dependency constraints
//   - find all available versions
//   - select latest compatible version
//   - detect conflicts
//   - return resolved dependency map
//
// Used by:
//   - installer.go
//   - update.go
//   - publisher.go
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

	"rho/internal/paths"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func ResolveVersion(name string, constraint string) (string, error) {
	// Step 1: Get available versions
	versions, err := getAvailableVersions(name)
	if err != nil {
		return "", err
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no versions available for module: %s", name)
	}

	// Step 2: Filter by constraint
	compatible := []string{}
	for _, v := range versions {
		ok, err := SemverSatisfies(v, constraint)
		if err != nil {
			return "", err
		}
		if ok {
			compatible = append(compatible, v)
		}
	}

	if len(compatible) == 0 {
		return "", fmt.Errorf("no compatible versions for %s with constraint %s", name, constraint)
	}

	// Step 3: Sort compatible versions
	sort.Slice(compatible, func(i, j int) bool {
		vi, _ := ParseSemver(compatible[i])
		vj, _ := ParseSemver(compatible[j])
		return CompareSemver(vi, vj) > 0 // descending
	})

	// Step 4: Return latest compatible version
	return compatible[0], nil
}

/* -------------------------------------------------------------------------- */
/*  Available Versions                                                          */
/* -------------------------------------------------------------------------- */

func getAvailableVersions(name string) ([]string, error) {
	out := []string{}

	// 1. Check local cache
	local := getLocalVersions(name)
	out = append(out, local...)

	// 2. Check remote registry (simulated)
	remote := getRemoteVersions(name)
	out = append(out, remote...)

	// Deduplicate
	uniq := map[string]bool{}
	final := []string{}
	for _, v := range out {
		if !uniq[v] {
			uniq[v] = true
			final = append(final, v)
		}
	}

	return final, nil
}

/* -------------------------------------------------------------------------- */
/*  Local Versions                                                              */
/* -------------------------------------------------------------------------- */

func getLocalVersions(name string) []string {
	cachePath, err := paths.GetCachePath()
	if err != nil {
		return []string{}
	}
	base := cachePath
	entries, _ := os.ReadDir(base)

	out := []string{}
	prefix := name + "@"

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			parts := strings.Split(e.Name(), "@")
			if len(parts) == 2 {
				out = append(out, parts[1])
			}
		}
	}

	return out
}

/* -------------------------------------------------------------------------- */
/*  Remote Versions (Local Registry)                                           */
/* -------------------------------------------------------------------------- */

func getRemoteVersions(name string) []string {
	registryPath, err := paths.GetRegistryPath()
	if err != nil {
		return []string{}
	}
	registryFile := filepath.Join(registryPath, "registry.yaml")

	registry, err := readRegistry(registryFile)
	if err != nil {
		return []string{}
	}

	out := []string{}
	for _, c := range registry.Components {
		if c.Name == name {
			out = append(out, c.Version)
		}
	}
	for _, p := range registry.Plugins {
		if p.Name == name {
			out = append(out, p.Version)
		}
	}

	return out
}
