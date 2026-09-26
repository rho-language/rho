//
// @file loader_test.go
// @brief Tests for loader module - Enterprise v1.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
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

	tf, err := LoadTests(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tf.Tests) != 1 {
		t.Fatalf("expected 1 test case, got %d", len(tf.Tests))
	}

	if tf.Tests[0].Name != "add_basic" {
		t.Fatalf("unexpected test name: %s", tf.Tests[0].Name)
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

	tf, err := LoadTests(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tf.Tests) != 0 {
		t.Fatalf("expected 0 tests, got %d", len(tf.Tests))
	}
}
