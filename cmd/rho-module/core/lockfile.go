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
	out := "lock:\n"
	for name, version := range deps {
		out += fmt.Sprintf("  %s: \"%s\"\n", name, version)
	}

	path := filepath.Join(dir, "rho.lock")
	return os.WriteFile(path, []byte(out), 0644)
}

/* -------------------------------------------------------------------------- */
/*  Internal: Parse Lockfile                                                   */
/* -------------------------------------------------------------------------- */

func parseLockfile(data string) map[string]string {
	out := map[string]string{}

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "lock:" {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		version := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		if name != "" && version != "" {
			out[name] = version
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
