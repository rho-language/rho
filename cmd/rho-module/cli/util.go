//
// @file util.go
// @brief CLI utility functions - Enterprise v3.
//
// Shared utility functions for rho-module CLI commands.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Utility Functions                                                          */
/* -------------------------------------------------------------------------- */

// parseSpec parses a module spec in the format name[@version]
// Returns (name, version). If version is not provided, returns empty string.
func parseSpec(spec string) (string, string) {
	if strings.Contains(spec, "@") {
		parts := strings.Split(spec, "@")
		return parts[0], parts[1]
	}
	return spec, ""
}
