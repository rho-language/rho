//
// @file remote.go
// @brief Rho Remote Registry - Enterprise v3.
//
// Provides access to the remote registry.
// In Enterprise v3, this will be backed by HTTP endpoints.
//
// Responsibilities:
//   - list remote modules
//   - get available versions
//   - fetch metadata
//   - integrate with resolver + search
//
// For now, uses simulated registry entries.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package registry

import "rho/cmd/rho-module/core"

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

type RemoteEntry = core.RegistryEntry

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

func ListRemoteModules() []RemoteEntry {
    return core.SimulatedRegistry()
}

func RemoteVersions(name string) []string {
    out := []string{}

    for _, e := range core.SimulatedRegistry() {
        if e.Name == name {
            out = append(out, e.Version)
        }
    }

    return out
}
