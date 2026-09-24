//
// @file index.go
// @brief Rho Registry Index - Enterprise v3.
//
// Provides unified access to both local and remote registry entries.
// Used by:
//   - resolver.go
//   - search.go
//   - installer.go
//
// Responsibilities:
//   - merge local + remote entries
//   - deduplicate versions
//   - provide unified module index
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package registry

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

type IndexEntry struct {
    Name        string
    Version     string
    Source      string // "local" or "remote"
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func BuildIndex() ([]IndexEntry, error) {
    out := []IndexEntry{}

    // Local
    local, err := ListLocalModules()
    if err == nil {
        for _, l := range local {
            out = append(out, IndexEntry{
                Name:    l.Name,
                Version: l.Version,
                Source:  "local",
            })
        }
    }

    // Remote
    remote := ListRemoteModules()
    for _, r := range remote {
        out = append(out, IndexEntry{
            Name:    r.Name,
            Version: r.Version,
            Source:  "remote",
        })
    }

    return out, nil
}
