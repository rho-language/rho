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
// SPDX-License-Identifier: MIT
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
    base := filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod")
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
/*  Remote Versions (Simulated)                                                */
/* -------------------------------------------------------------------------- */

func getRemoteVersions(name string) []string {
    out := []string{}

    for _, entry := range simulatedRegistry {
        if entry.Name == name {
            out = append(out, entry.Version)
        }
    }

    return out
}
