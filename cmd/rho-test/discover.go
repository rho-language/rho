//
// @file discover.go
// @brief Test file discovery for Rho Test Framework - Enterprise v1.
//
// Responsibilities:
//   - Accepts a path (file or directory)
//   - Returns all *_test.rho files
//   - Supports recursive directory traversal
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"os"
	"path/filepath"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Discover                                                                   */
/* -------------------------------------------------------------------------- */

func Discover(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// Single file mode
	if !info.IsDir() {
		if strings.HasSuffix(path, "_test.rho") {
			return []string{path}, nil
		}
		return []string{}, nil
	}

	// Directory mode
	var out []string

	err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() && strings.HasSuffix(p, "_test.rho") {
			out = append(out, p)
		}
		return nil
	})

	return out, err
}
