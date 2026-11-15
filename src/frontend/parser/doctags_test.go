//
// @file doctags_test.go
// @brief A doc comment's tags are read for the declaration directly below it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package parser

import (
	"testing"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
)

func TestADeprecatedLineMarksTheDeclarationBelowIt(t *testing.T) {
	source := `//
// @file lib.rho
// @deprecated: use other instead
// @stability experimental
//
module lib

// old does what it did.
// @deprecated use new instead
fn old() void {}

// @deprecated

fn spaced() void {}

fn plain() void {}

// @deprecatedly not a tag
const A: u32 = 1

// @deprecated use B2
const B: u32 = 2

// @deprecated use Size
struct Old { w: u32 }

// @deprecated
type Handle = u64

enum Mode {
    Off
    // @deprecated use On
    Lit
    On
}

// The comment above a one-line enum is the enum's.
enum Flag { Up, Down }
`
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatal(err)
	}
	module, err := ParseFile(tokens, "lib.rho")
	if err != nil {
		t.Fatal(err)
	}
	AttachDocTags(module, source)
	got := map[string]*ast.Deprecation{}
	for _, decl := range module.Decls {
		switch d := decl.(type) {
		case ast.FuncDecl:
			got[d.Name] = d.Deprecated
		case ast.ConstDecl:
			got[d.Name] = d.Deprecated
		case ast.StructDecl:
			got[d.Name] = d.Deprecated
		case ast.TypeDecl:
			got[d.Name] = d.Deprecated
		case ast.EnumDecl:
			got[d.Name] = d.Deprecated
			for _, variant := range d.Variants {
				got[d.Name+"."+variant.Name] = variant.Deprecated
			}
		}
	}
	want := map[string]string{"old": "use new instead", "B": "use B2", "Old": "use Size", "Handle": "", "Mode.Lit": "use On"}
	for name, note := range want {
		if got[name] == nil || got[name].Note != note {
			t.Errorf("%s: got %+v, want deprecated %q", name, got[name], note)
		}
	}
	for _, name := range []string{"spaced", "plain", "A", "Mode", "Mode.Off", "Mode.On", "Flag", "Flag.Up", "Flag.Down"} {
		if got[name] != nil {
			t.Errorf("%s was marked deprecated: %+v", name, got[name])
		}
	}
	if d := ModuleDeprecation(source); d == nil || d.Note != "use other instead" {
		t.Errorf("the module's deprecation is %+v", d)
	}
	if value, ok := HeaderTag(source, StabilityTag); !ok || value != "experimental" {
		t.Errorf("the stability is %q, %v", value, ok)
	}
	if _, ok := HeaderTag("module x\n// @stability stable\n", StabilityTag); ok {
		t.Error("a tag after the module line was read as the header's")
	}
}
