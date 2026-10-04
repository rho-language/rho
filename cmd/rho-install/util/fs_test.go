//
// @file fs_test.go
// @brief Tests for Rho Install - Filesystem utilities.
//
// @component rho-install
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package util

import (
	"os"
	"path/filepath"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  EnsureDir                                                                  */
/* -------------------------------------------------------------------------- */

func TestEnsureDir(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "testdir")

	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	if !DirExists(dir) {
		t.Fatalf("DirExists returned false for created directory")
	}
}

/* -------------------------------------------------------------------------- */
/*  CopyFile                                                                    */
/* -------------------------------------------------------------------------- */

func TestCopyFile(t *testing.T) {
	tmp := t.TempDir()

	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "dst.txt")

	os.WriteFile(src, []byte("hello"), 0o644)

	if err := CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "hello" {
		t.Fatalf("CopyFile content mismatch")
	}
}

/* -------------------------------------------------------------------------- */
/*  WriteFile                                                                   */
/* -------------------------------------------------------------------------- */

func TestWriteFile(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "file.txt")

	if err := WriteFile(file, []byte("test")); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if !FileExists(file) {
		t.Fatalf("FileExists returned false for written file")
	}
}

/* -------------------------------------------------------------------------- */
/*  Join                                                                        */
/* -------------------------------------------------------------------------- */

func TestJoin(t *testing.T) {
	p := Join("a", "b", "c")
	if p != filepath.Join("a", "b", "c") {
		t.Fatalf("Join returned incorrect path: %s", p)
	}
}
