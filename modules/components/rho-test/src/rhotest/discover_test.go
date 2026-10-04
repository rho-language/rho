//
// @file discover_test.go
// @brief Tests for test file discovery - Enterprise v1.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhotest

import (
	"os"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Discover: Single File                                                      */
/* -------------------------------------------------------------------------- */

func TestDiscoverSingleFile(t *testing.T) {
	tmp := "tmp_test.rho"
	os.WriteFile(tmp, []byte("test fn x() {}"), 0644)
	defer os.Remove(tmp)

	files, err := Discover(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

/* -------------------------------------------------------------------------- */
/*  Discover: Directory                                                        */
/* -------------------------------------------------------------------------- */

func TestDiscoverDirectory(t *testing.T) {
	os.Mkdir("tmpdir", 0755)
	defer os.RemoveAll("tmpdir")

	os.WriteFile("tmpdir/a_test.rho", []byte("test fn x() {}"), 0644)
	os.WriteFile("tmpdir/b.rho", []byte("fn y() {}"), 0644)

	files, err := Discover("tmpdir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 test file, got %d", len(files))
	}
}
