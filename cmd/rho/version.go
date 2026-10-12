//
// @file version.go
// @brief The version the `rho` binary reports, from the VERSION file beside it.
//
// The number lives in one place — `cmd/rho/VERSION`, beside the command it
// describes — so the version a person reads in the tree, the version
// `rho --version` prints, and the version a released binary carries are one
// number rather than three that a release has to remember to agree. It is
// embedded, so a binary installed anywhere still knows the version it is.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

// toolchainVersion is the version this binary was built from.
func toolchainVersion() string {
	if trimmed := strings.TrimSpace(version); trimmed != "" {
		return trimmed
	}
	return "unknown"
}
