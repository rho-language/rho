//
// @file file.go
// @brief Reading and writing files, in one place.
//
// The generator reads headers and writes sources, and every path it touches is a
// path a person named in a manifest. Keeping that here means one answer to
// "where may this write" rather than one per call site.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cimport

import (
	"os"
	"path/filepath"
	"strings"
)

// ReadFile reads a file, or reports why it could not be read.
func ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// isDir reports whether a path is a directory, which is how a caller tells a
// declared header that is a directory of headers from one that is a file.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// WriteFile writes a file, refusing to leave the directory it was given.
//
// A generator writes into a project. A path that climbs out of that project is
// a path a manifest could name and a person would not, so it is refused here
// rather than followed.
func WriteFile(root, relative, content string) (string, error) {
	destination := filepath.Join(root, filepath.FromSlash(relative))
	if !within(root, destination) {
		return "", &PathError{Path: relative, Message: "is outside the project"}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(destination, []byte(content), 0o644); err != nil {
		return "", err
	}
	return destination, nil
}

// PathError is a path a generator refused.
type PathError struct {
	Path    string
	Message string
}

func (e *PathError) Error() string { return e.Path + ": " + e.Message }

// within reports whether a path is inside a directory.
func within(dir, path string) bool {
	relative, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
