//
// @file yamldoc_test.go
// @brief The positional YAML reader: the subset the files use, and the lines it reports.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"strings"
	"testing"
)

func TestParseTheSubsetTheFilesUse(t *testing.T) {
	src := `# a comment
levels:
  - { name: SYS,  title: System }
  - {name: NODE, title: "Node, the box"}
items:
- id: A-0001     # trailing comment
  title: 'It''s plain'
  statement: >
    folded over
    two lines
  literal: |
    kept
    as is
  satisfies: [SYS-0001,
              SYS-0002]
  tags:
    - x
    - "y # not a comment"
  empty:
  url: http://example.org/a#b
- id: A-0002
  title: wrapped plain
    continues here
`
	root, err := ParseYAML(src)
	if err != nil {
		t.Fatal(err)
	}
	levels := root.Get("levels").Value
	if len(levels.Items) != 2 || levels.Items[0].Str("name") != "SYS" || levels.Items[1].Str("title") != "Node, the box" {
		t.Fatalf("flow mappings in a block list: %+v", levels.Items)
	}
	items := root.Get("items").Value.Items
	first := items[0]
	checks := map[string]string{
		"id": "A-0001", "title": "It's plain", "statement": "folded over two lines\n", "literal": "kept\nas is\n", "url": "http://example.org/a#b",
	}
	for key, want := range checks {
		if got := first.Str(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if got := strings.Join(first.Strings("satisfies"), ","); got != "SYS-0001,SYS-0002" {
		t.Errorf("multi-line flow list = %q", got)
	}
	if got := strings.Join(first.Strings("tags"), ","); got != "x,y # not a comment" {
		t.Errorf("block list = %q", got)
	}
	if first.Get("empty").Value.Kind != Null {
		t.Errorf("an empty key is null")
	}
	if first.Line != 6 || first.Get("satisfies").Line != 14 || first.Get("satisfies").Value.EndLine != 15 {
		t.Errorf("lines: item %d, satisfies %d-%d", first.Line, first.Get("satisfies").Line, first.Get("satisfies").Value.EndLine)
	}
	if items[1].Str("title") != "wrapped plain continues here" {
		t.Errorf("continued plain scalar = %q", items[1].Str("title"))
	}
}

func TestParseErrorsNameTheLine(t *testing.T) {
	cases := map[string]int{
		"a: 1\na: 2\n":                     2,
		"a:\n\t- x\n":                      2,
		"a: [1, 2\n":                       1,
		"a: 1\n  b: 2\n":                   2,
		"list:\n  - x\n  y: 1\n":           3,
		"a: \"open\n":                      1,
		"a: &anchor x\n":                   1,
		"items:\n  - id: X\n     bad: 1\n": 3,
	}
	for src, line := range cases {
		_, err := ParseYAML(src)
		parseErr, ok := err.(*ParseError)
		if !ok {
			t.Errorf("%q: expected a parse error, got %v", src, err)
			continue
		}
		if parseErr.Line != line {
			t.Errorf("%q: error on line %d, want %d (%s)", src, parseErr.Line, line, parseErr.Message)
		}
	}
}
