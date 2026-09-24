//
// @file search.go
// @brief Rho Module Registry Search - Enterprise v3.
//
// Searches the remote registry for modules matching a query.
// Returns:
//   - name
//   - latest version
//   - type
//   - description
//
// In Enterprise v3, this will be backed by HTTP registry queries.
// For now, a simulated registry is used.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package core

import (
    "strings"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

type RegistryEntry struct {
    Name        string
    Version     string
    Type        string
    Description string
}

/* -------------------------------------------------------------------------- */
/*  Simulated Registry                                                         */
/* -------------------------------------------------------------------------- */

var simulatedRegistry = []RegistryEntry{
	{"math", "1.2.0", "module", "Math utilities and numeric algorithms"},
	{"io", "1.1.0", "module", "Input/output utilities"},
	{"http", "0.9.0", "plugin", "HTTP server plugin"},
	{"json", "1.0.0", "module", "JSON parsing and serialization"},
	{"crypto", "2.0.0", "module", "Cryptographic primitives"},
}

// SimulatedRegistry returns the full simulated registry for remote access
func SimulatedRegistry() []RegistryEntry {
	return simulatedRegistry
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func SearchRegistry(query string) ([]RegistryEntry, error) {
    var out []RegistryEntry

    q := strings.ToLower(query)

    for _, entry := range simulatedRegistry {
        if strings.Contains(strings.ToLower(entry.Name), q) ||
            strings.Contains(strings.ToLower(entry.Description), q) {
            out = append(out, entry)
        }
    }

    return out, nil
}
