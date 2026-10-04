//
// @file fs.go
// @brief Rho Install - Filesystem utilities.
//
// Responsibilities:
//   - Safe file and directory operations
//   - Deterministic helpers for install subsystem
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

/* -------------------------------------------------------------------------- */
/*  Directory Helpers                                                          */
/* -------------------------------------------------------------------------- */

// EnsureDir creates a directory if it does not exist.
func EnsureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}
	return nil
}

// DirExists checks if a directory exists.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

/* -------------------------------------------------------------------------- */
/*  File Helpers                                                               */
/* -------------------------------------------------------------------------- */

// FileExists checks if a file exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// CopyFile copies a file from src to dst.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("cannot open source file: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot create destination file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

// WriteFile writes content to a file.
func WriteFile(path string, content []byte) error {
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Path Helpers                                                               */
/* -------------------------------------------------------------------------- */

// Join joins path components safely.
func Join(parts ...string) string {
	return filepath.Join(parts...)
}
