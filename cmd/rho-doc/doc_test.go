//
// @file doc_test.go
// @brief Rho Documentation Generator tests.
//
// Tests deterministic Markdown generation for Rho modules.
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
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Basic Documentation                                                 */
/* -------------------------------------------------------------------------- */

func TestGenerateDoc(t *testing.T) {
	src := `
module testdoc

struct A {
    x: u8
    y: u16
}

fn add(a: u8, b: u8) u8 {
}
`

	expected := "# Module `testdoc`\n\n" +
		"## Structs\n\n" +
		"### `A`\n\n" +
		"| Field | Type |\n" +
		"|-------|------|\n" +
		"| x | u8 |\n" +
		"| y | u16 |\n\n" +
		"## Functions\n\n" +
		"### `add()`\n\n" +
		"**Signature:**\n\n```\n" +
		"fn add(a: u8, b: u8) u8\n```\n\n"

	path := "doc_test.rho"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	got, err := GenerateDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != expected {
		t.Fatalf("unexpected documentation:\n%s", got)
	}
}
