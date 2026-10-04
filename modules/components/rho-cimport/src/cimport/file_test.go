//
// @file file_test.go
// @brief Tests for the file helpers.
//
// A generator reads headers and writes sources, and every path it touches is a
// path a person named in a manifest. The one rule worth a test of its own is that
// a write stays inside the project it was given: everything else is a read that
// reports why it failed.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cimport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadingAFileThatIsNotThereSaysWhy(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadFile(filepath.Join(dir, "absent.h")); err == nil {
		t.Fatal("expected a missing file to be reported")
	}
}

func TestReadingAFileThatIsThere(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "there.h")
	if err := os.WriteFile(path, []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if text != "content\n" {
		t.Fatalf("expected the contents, got %q", text)
	}
}

func TestWritingCreatesTheDirectoriesItNeeds(t *testing.T) {
	root := t.TempDir()
	path, err := WriteFile(root, "src/nested/ffi.rho", "module ffi\n")
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "module ffi\n" {
		t.Fatalf("expected the content, got %q", data)
	}
}

func TestWritingOutsideTheProjectIsRefused(t *testing.T) {
	// A path that climbs out of the project is one a manifest could name and a
	// person would not, so it is refused rather than followed. The check is on the
	// cleaned path, so a path that climbs out through a subdirectory is refused too.
	root := t.TempDir()
	for _, name := range []string{"../escape.rho", "src/../../escape.rho"} {
		if _, err := WriteFile(root, name, "x"); err == nil {
			t.Errorf("expected %q to be refused", name)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.rho")); err == nil {
		t.Fatal("a file was written outside the project")
	}
}

func TestExistsTellsAFileFromADirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.h")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !exists(file) {
		t.Error("expected a file to exist")
	}
	if exists(root) {
		t.Error("expected a directory not to count as a file")
	}
	if !isDir(root) {
		t.Error("expected a directory to be a directory")
	}
	if isDir(file) {
		t.Error("expected a file not to be a directory")
	}
}
