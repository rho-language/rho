//
// @file malformed_test.go
// @brief Tests for documents the loader cannot read.
//
// A loader is given files it did not write: a manifest a person edited, one a
// script mangled, one from another tool. The one thing it must never do is
// crash, because a crash takes the build with it and says nothing about the file
// that caused it. Every case here is one that used to be a panic.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package yaml

import "testing"

// A key indented as if it were the content of something that is not a mapping.
// This is what a description leaves behind when it is half-rewritten: the
// continuation line is still there, indented under a key that no longer owns it.
func TestAKeyUnderSomethingThatIsNotAMappingIsAnError(t *testing.T) {
	// The shape a hand-edited manifest takes: a list item, and its keys written
	// at the same indent as the `- ` rather than under it. Every line looks
	// reasonable on its own and together they describe nothing.
	document := "depends:\n    - name: a\n    type: c\n    path: /x\n"

	// The panic this replaces was an assignment into a nil map.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the loader panicked on a malformed document: %v", r)
		}
	}()

	if _, err := Load(document); err == nil {
		t.Fatalf("a document with a key under a list was accepted")
	}
}

// The error says which line, because the reader of it is looking at the file and
// wants to be told where to look.
func TestAMalformedDocumentSaysWhichLine(t *testing.T) {
	_, err := Load("a: 1\nb:\n  - x\n  c: 2\n")
	if err == nil {
		t.Fatalf("a document with a key under a list was accepted")
	}
	if got := err.Error(); !contains(got, "line 4") {
		t.Errorf("the error is %q, and does not say which line is wrong", got)
	}
}

// The shapes that are legitimate keep working: a list of mappings under a key is
// the most ordinary thing a manifest contains, and it is the shape the guard
// above must not break.
func TestTheOrdinaryShapesStillLoad(t *testing.T) {
	shapes := map[string]string{
		"list of mappings":    "presets:\n    - name: one\n      target: a\n    - name: two\n      target: b\n",
		"mapping of mappings": "build:\n  language: c\n  defines:\n    A: 1\n    B: 2\n",
		"block scalar":        "description: >-\n  some words\n  more words\nname: x\n",
		"inline list":         "tags: [a, b, c]\n",
		"empty key then list": "presets:\n- one\n- two\n",
	}
	for label, document := range shapes {
		t.Run(label, func(t *testing.T) {
			if _, err := Load(document); err != nil {
				t.Fatalf("a legitimate document was refused: %v", err)
			}
		})
	}
}

// contains reports whether a value is among a string's substrings.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
