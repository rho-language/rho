//
// @file loader_test.go
// @brief Tests for reading the tests out of a Rho source file.
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
/*  LoadTests: Basic Detection                                                 */
/* -------------------------------------------------------------------------- */

func TestLoadTestsBasic(t *testing.T) {
	tmp := "tmp_load_test.rho"
	code := `
test fn add_basic() {
    return true
}

fn normal() {}
`
	os.WriteFile(tmp, []byte(code), 0644)
	defer os.Remove(tmp)

	cases, err := LoadTests(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cases) != 1 {
		t.Fatalf("expected 1 test case, got %d", len(cases))
	}

	if cases[0].Name != "add_basic" {
		t.Fatalf("unexpected test name: %s", cases[0].Name)
	}
	if cases[0].File != tmp {
		t.Errorf("a case should point at the file it came from, got %q", cases[0].File)
	}
	if cases[0].Line == 0 {
		t.Error("a case should point at the line it is declared on")
	}
}

/* -------------------------------------------------------------------------- */
/*  LoadTests: No Tests                                                        */
/* -------------------------------------------------------------------------- */

func TestLoadTestsNone(t *testing.T) {
	tmp := "tmp_no_tests.rho"
	code := `
fn x() {}
fn y() {}
`
	os.WriteFile(tmp, []byte(code), 0644)
	defer os.Remove(tmp)

	cases, err := LoadTests(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cases) != 0 {
		t.Fatalf("expected 0 tests, got %d", len(cases))
	}
}
