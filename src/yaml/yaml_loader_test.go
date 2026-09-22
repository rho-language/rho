//
// @file yaml_loader_test.go
// @brief Rho YAML Loader dedicated tests.
//
// Tests the deterministic YAML loader (yaml_loader.go) separately from
// the value model. Ensures correct parsing of key-value pairs, nested
// maps, lists, and error handling.
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
/*  Tests: Simple Key-Value                                                   */
/* -------------------------------------------------------------------------- */

func TestLoaderSimpleKeyValue(t *testing.T) {
	src := `
name: rho
version: 0.1
`

	v, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v.Map["name"].Str != "rho" {
		t.Fatalf("expected name=rho, got %q", v.Map["name"].Str)
	}
	if v.Map["version"].Str != "0.1" {
		t.Fatalf("expected version=0.1, got %q", v.Map["version"].Str)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Nested Map                                                         */
/* -------------------------------------------------------------------------- */

func TestLoaderNestedMap(t *testing.T) {
	src := `
database:
  host: localhost
  port: 5432
`

	v, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	db := v.Map["database"]
	if db.Kind != KindMap {
		t.Fatalf("expected database to be a map")
	}

	if db.Map["host"].Str != "localhost" {
		t.Fatalf("expected host=localhost")
	}
	if db.Map["port"].Str != "5432" {
		t.Fatalf("expected port=5432")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: List                                                               */
/* -------------------------------------------------------------------------- */

func TestLoaderList(t *testing.T) {
	src := `
list:
- alpha
- beta
- gamma
`

	v, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lst := v.Map["list"].List
	if len(lst) != 3 {
		t.Fatalf("expected 3 list items")
	}

	if lst[0].Str != "alpha" || lst[1].Str != "beta" || lst[2].Str != "gamma" {
		t.Fatalf("unexpected list contents")
	}
}

func TestLoaderListOfMaps(t *testing.T) {
	v, err := Load("depends:\n- name: common\n  type: c\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	depends := v.Map["depends"].List
	if len(depends) != 1 || depends[0].Map["name"].Str != "common" || depends[0].Map["type"].Str != "c" {
		t.Fatalf("unexpected list map: %#v", depends)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Comments and Empty Lines                                           */
/* -------------------------------------------------------------------------- */

func TestLoaderCommentsAndEmptyLines(t *testing.T) {
	src := `
# this is a comment

name: rho

# another comment
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
/*  Tests: Invalid Input                                                      */
/* -------------------------------------------------------------------------- */

func TestLoaderInvalid(t *testing.T) {
	src := `
this is not valid yaml
`

	_, err := Load(src)
	if err == nil {
		t.Fatalf("expected error for invalid YAML")
	}
}
