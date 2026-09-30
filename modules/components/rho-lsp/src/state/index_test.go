//
// @file index_test.go
// @brief What a file declares, read from the compiler's own tree.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package state

import "testing"

// A file's declarations come from the tree the compiler builds, so a name a
// completion offers is a name the compiler knows.
func TestIndexReadsDeclarationsFromRealSource(t *testing.T) {
	source := "module shop\n\nstruct Point { x: i32, y: i32 }\n\nfn origin() i32 {\n  let a: i32 = 1;\n  return a;\n}\n"
	ix := BuildIndex("file:///test.rho", source)
	if ix == nil {
		t.Fatal("a well-formed file was not indexed")
	}
	for _, name := range []string{"Point", "origin", "a"} {
		if !ix.Symbols.Local[name] {
			t.Errorf("%q is declared and was not indexed", name)
		}
	}
	// A struct's fields are what a completion after a dot offers, and which
	// struct each came from is what makes the list usable without types.
	var fields []string
	for _, field := range ix.Fields {
		if field.Struct == "Point" {
			fields = append(fields, field.Name)
		}
	}
	if len(fields) != 2 {
		t.Fatalf("Point's fields are %v, want two", fields)
	}
}

// A use and a declaration are different facts, and references turn on the
// difference.
func TestIndexRecordsUsesAsWellAsDeclarations(t *testing.T) {
	source := "module shop\n\nfn twice(n: i32) i32 {\n  return twice(n);\n}\n"
	ix := BuildIndex("file:///test.rho", source)
	uses := ix.Symbols.LocalRefs["twice"]
	if len(uses) < 2 {
		t.Fatalf("the uses of 'twice' are %d, want the declaration and the call", len(uses))
	}
	// And each is a distinct place, or a count is a number nobody can use.
	seen := map[string]bool{}
	for _, use := range uses {
		key := string(rune(use.Range.Start.Line)) + ":" + string(rune(use.Range.Start.Character))
		if seen[key] {
			t.Errorf("the same place is reported twice: %v", use)
		}
		seen[key] = true
	}
}

// A cursor is answered from the same coordinates the compiler reports in.
func TestIndexFindsWhatIsUnderTheCursor(t *testing.T) {
	source := "module shop\n\nfn origin() i32 {\n  return 1;\n}\n"
	ix := BuildIndex("file:///test.rho", source)
	// `origin` is the third line counting from one, at its second character, and
	// the index stores what the protocol stores: from zero.
	found, ok := ix.At(2, 2)
	if !ok {
		t.Fatal("nothing was found under the cursor")
	}
	if found.Name != "origin" {
		t.Errorf("found %q under the cursor, want origin", found.Name)
	}
	if found.Role != RoleFunction {
		t.Errorf("found a %q, want a function", found.Role)
	}
	// And nothing, where there is nothing.
	if _, ok := ix.At(0, 0); ok {
		t.Error("something was found where there is nothing")
	}
}

// A file with a mistake in it is indexed as far as it goes: what it declares
// before the mistake is real, and a completion that offers it is more useful than
// one that offers nothing because a later line is wrong.
func TestIndexKeepsWhatItCanWhenAFileIsBroken(t *testing.T) {
	ix := BuildIndex("file:///test.rho", "module shop\n\nfn origin( i32 {\n  return 1;\n}\n")
	if ix == nil {
		t.Fatal("a broken file was not indexed at all")
	}
	if !ix.Symbols.Local["origin"] {
		t.Error("the declaration before the mistake was lost")
	}
}
