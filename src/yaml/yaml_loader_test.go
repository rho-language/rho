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
/*  Tests: List Inference Is Content Driven                                    */
/* -------------------------------------------------------------------------- */

// A key holding list items becomes a list even when its name was never
// registered in any table. Schema validation depends on this.
func TestLoaderInfersListFromContent(t *testing.T) {
	v, err := Load("any_key_name:\n- alpha\n- beta\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Map["any_key_name"].Kind != KindList {
		t.Fatalf("expected inferred list, got kind %d", v.Map["any_key_name"].Kind)
	}
	if len(v.Map["any_key_name"].List) != 2 {
		t.Fatalf("expected 2 items, got %d", len(v.Map["any_key_name"].List))
	}
}

// A key holding key/value pairs becomes a map even when its name looks
// like a list.
func TestLoaderInfersMapFromContent(t *testing.T) {
	v, err := Load("features:\n  first: a\n  second: b\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Map["features"].Kind != KindMap {
		t.Fatalf("expected inferred map, got kind %d", v.Map["features"].Kind)
	}
	if v.Map["features"].Map["first"].Str != "a" {
		t.Fatalf("unexpected nested map: %#v", v.Map["features"].Map)
	}
}

// A key with no content at all stays null so schemas can report it.
func TestLoaderEmptyKeyIsNull(t *testing.T) {
	v, err := Load("components:\nplugins:\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Map["components"].Kind != KindNull {
		t.Fatalf("expected null for valueless key, got kind %d", v.Map["components"].Kind)
	}
}

// Nested lists inside maps must round-trip through the parent copies.
func TestLoaderNestedListInsideMap(t *testing.T) {
	src := "build:\n  language: c\n  include_dirs:\n    - include\n    - src/include\n  source_dirs:\n    - src\n"
	v, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	build := v.Map["build"]
	if build.Map["language"].Str != "c" {
		t.Fatalf("unexpected language: %q", build.Map["language"].Str)
	}
	includeDirs := build.Map["include_dirs"]
	if includeDirs.Kind != KindList || len(includeDirs.List) != 2 {
		t.Fatalf("unexpected include_dirs: %#v", includeDirs)
	}
	if includeDirs.List[0].Str != "include" || includeDirs.List[1].Str != "src/include" {
		t.Fatalf("unexpected include_dirs content: %#v", includeDirs.List)
	}
	if srcDirs := build.Map["source_dirs"]; srcDirs.Kind != KindList || len(srcDirs.List) != 1 {
		t.Fatalf("unexpected source_dirs: %#v", srcDirs)
	}
}

// Registry-style sections are lists of maps written with indentation.
func TestLoaderIndentedListOfMaps(t *testing.T) {
	src := "components:\n  - name: math\n    version: 1.2.0\nactive_components:\n  - name: math\n    version: 1.2.0\n"
	v, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	comps := v.Map["components"]
	if comps.Kind != KindList || len(comps.List) != 1 {
		t.Fatalf("unexpected components: %#v", comps)
	}
	if comps.List[0].Map["name"].Str != "math" || comps.List[0].Map["version"].Str != "1.2.0" {
		t.Fatalf("unexpected component entry: %#v", comps.List[0])
	}
	active := v.Map["active_components"]
	if active.Kind != KindList || len(active.List) != 1 || active.List[0].Map["version"].Str != "1.2.0" {
		t.Fatalf("unexpected active_components: %#v", active)
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

/* -------------------------------------------------------------------------- */
/*  Tests: Block Scalars                                                      */
/* -------------------------------------------------------------------------- */

// Real manifests carry multi-line descriptions with the folded ">" style.
func TestLoaderBlockScalarFolded(t *testing.T) {
	src := `module:
  name: robohub-common
  description: >
    Core platform-independent interfaces for the platform.
    Provides generic types and error codes.
  version: 1.0.0
`
	v, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mod := v.Map["module"]
	got := mod.Map["description"].Str
	want := "Core platform-independent interfaces for the platform. Provides generic types and error codes."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// The key after the block scalar must still be read normally.
	if mod.Map["version"].Str != "1.0.0" {
		t.Fatalf("expected the version after the block, got %q", mod.Map["version"].Str)
	}
}

// The literal "|" style keeps the line breaks.
func TestLoaderBlockScalarLiteral(t *testing.T) {
	src := `notes: |
  first line
  second line
other: kept
`
	v, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := v.Map["notes"].Str; got != "first line\nsecond line" {
		t.Fatalf("got %q", got)
	}
	if v.Map["other"].Str != "kept" {
		t.Fatalf("expected the key after the block, got %q", v.Map["other"].Str)
	}
}

func TestLoaderBlockScalarEmpty(t *testing.T) {
	v, err := Load("notes: >\nother: kept\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Map["notes"].Str != "" {
		t.Fatalf("expected an empty block scalar, got %q", v.Map["notes"].Str)
	}
	if v.Map["other"].Str != "kept" {
		t.Fatalf("expected the following key to survive, got %q", v.Map["other"].Str)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Inline Sequences                                                   */
/* -------------------------------------------------------------------------- */

// Manifests use flow-style empty lists for absent dependency groups.
func TestLoaderInlineEmptyList(t *testing.T) {
	src := `dependencies:
  required: []
  optional: []
`
	v, err := Load(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deps := v.Map["dependencies"]
	for _, key := range []string{"required", "optional"} {
		node := deps.Map[key]
		if node.Kind != KindList {
			t.Fatalf("%s: expected a list, got kind %d", key, node.Kind)
		}
		if len(node.List) != 0 {
			t.Fatalf("%s: expected an empty list, got %d items", key, len(node.List))
		}
	}
}

func TestLoaderInlineListWithItems(t *testing.T) {
	v, err := Load("tags: [core, hal, stable]\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tags := v.Map["tags"]
	if tags.Kind != KindList || len(tags.List) != 3 {
		t.Fatalf("unexpected tags: %#v", tags)
	}
	if tags.List[0].Str != "core" || tags.List[2].Str != "stable" {
		t.Fatalf("unexpected tag content: %#v", tags.List)
	}
}

// A quoted value that merely looks like a list must stay a string.
func TestLoaderQuotedBracketIsString(t *testing.T) {
	v, err := Load(`expression: "[]"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Map["expression"].Kind != KindString || v.Map["expression"].Str != "[]" {
		t.Fatalf("expected a string, got %#v", v.Map["expression"])
	}
}
