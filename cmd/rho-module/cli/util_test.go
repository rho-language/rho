//
// @file util_test.go
// @brief Tests for CLI utilities - Enterprise v3.
//
// Validates:
//   - parseSpec function
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: parseSpec                                                            */
/* -------------------------------------------------------------------------- */

func TestParseSpecWithVersion(t *testing.T) {
	name, version := parseSpec("math@1.2.0")
	if name != "math" {
		t.Fatalf("expected name 'math', got '%s'", name)
	}
	if version != "1.2.0" {
		t.Fatalf("expected version '1.2.0', got '%s'", version)
	}
}

func TestParseSpecWithoutVersion(t *testing.T) {
	name, version := parseSpec("math")
	if name != "math" {
		t.Fatalf("expected name 'math', got '%s'", name)
	}
	if version != "" {
		t.Fatalf("expected empty version, got '%s'", version)
	}
}

func TestParseSpecMultipleAt(t *testing.T) {
	// Split splits on ALL @ characters, so only first part is name
	name, version := parseSpec("math@1.2.0@extra")
	if name != "math" {
		t.Fatalf("expected name 'math', got '%s'", name)
	}
	// strings.Split returns all parts, so version will be just "1.2.0"
	if version != "1.2.0" {
		t.Fatalf("expected version '1.2.0', got '%s'", version)
	}
}
