//
// @file yaml_test.go
// @brief Rho YAML Loader tests.
//
// Tests the minimal deterministic YAML loader.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package yaml

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Basic Key-Value                                                    */
/* -------------------------------------------------------------------------- */

func TestLoadKeyValue(t *testing.T) {
	src := `
name: rho
version: 0.1
`

	v, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v.Map["name"].Str != "rho" {
		t.Fatalf("expected name=rho")
	}
	if v.Map["version"].Str != "0.1" {
		t.Fatalf("expected version=0.1")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: List                                                               */
/* -------------------------------------------------------------------------- */

func TestLoadList(t *testing.T) {
	src := `
list:
- a
- b
- c
`

	v, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lst := v.Map["list"].List
	if len(lst) != 3 {
		t.Fatalf("expected 3 list items")
	}
	if lst[0].Str != "a" || lst[1].Str != "b" || lst[2].Str != "c" {
		t.Fatalf("unexpected list contents")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Invalid                                                            */
/* -------------------------------------------------------------------------- */

func TestLoadInvalid(t *testing.T) {
	src := `
invalid line without colon
`

	_, err := Load(src)
	if err == nil {
		t.Fatalf("expected error for invalid YAML")
	}
}

func TestLoadKeepsQuotesThatDoNotEncloseTheValue(t *testing.T) {
	// A value that starts and ends with a quote is not a quoted scalar when a
	// quote of its own sits between them: the quotes belong to the value, the
	// way they do in a command that runs a program with arguments.
	src := `run: "prog" --flag "arg"`
	root, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	got := root.Map["run"].Str
	if got != `"prog" --flag "arg"` {
		t.Fatalf("expected the value kept as written, got %s", got)
	}
}

func TestLoadUnquotesAScalar(t *testing.T) {
	src := `title: "A quoted title"`
	root, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := root.Map["title"].Str; got != "A quoted title" {
		t.Fatalf("expected the quotes removed, got %s", got)
	}
}
