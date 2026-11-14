//
// @file enums_test.go
// @brief What an enum is to the build: its warnings, and an Option of a pointer.
//
// The parity cases say an enum means the same on both backends. These say what
// only one place can: that an arm which can never run is a warning the build and
// the editor both report, and that the standard Option, now an enum, still keeps
// an Option of a pointer as the bare pointer in C — None is null, and nothing
// is tagged — which is what a C library hands back and expects.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/codegen"
)

// writeEnumProject writes a one-file project and returns its entry.
func writeEnumProject(t *testing.T, source string) string {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: enums\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: c\n  targets:\n    - c\n"
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return entry
}

// An arm that can never run is said once, as a warning, at the arm — by the
// editor's reading of the file, which is what underlines it.
func TestAnUnreachableArmIsAWarningAtTheArm(t *testing.T) {
	entry := writeEnumProject(t, `module app

enum Shape {
    Circle(i32)
    Square(i32)
}

fn size(s: Shape) i32 {
    match s {
        Shape.Circle(r) => { return r }
        Shape.Square(a) => { return a }
        _ => { return 0 }
    }
}

fn kind(s: Shape) string {
    match s {
        Shape.Circle(_) => { return "circle" }
        Shape.Circle(0) => { return "dot" }
        _ => { return "other" }
    }
}

fn main() void {
    print(kind(Shape.Circle(size(Shape.Square(2)))))
}
`)
	text, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	got := DiagnoseInProgram(entry, string(text)).Diagnostics
	if len(got) != 2 {
		t.Fatalf("want two warnings, got:\n%s", RenderDiagnostics(got, filepath.Dir(entry)))
	}
	wants := []struct {
		line, column int
		message      string
	}{
		{12, 9, "the arms above handle every value of Shape, so `_` never runs"},
		{19, 9, "the arms above already match every value this one would"},
	}
	for i, want := range wants {
		if got[i].Severity != SeverityWarning || got[i].Line != want.line || got[i].Column != want.column || !strings.Contains(got[i].Message, want.message) {
			t.Errorf("warning %d is %s, want %d:%d: warning: ...%s...", i+1, got[i].Render(filepath.Dir(entry)), want.line, want.column, want.message)
		}
	}
	// The build says the same and still builds.
	if _, _, err := compileToIR(nil, entry, nil); err != nil {
		t.Fatalf("a warning stopped the build: %v", err)
	}
}

// Option is an enum now, and an Option of a pointer is still the pointer: the
// type is a typedef of it, None is NULL, Some(p) is p, and a match tests it
// against NULL. No tag and no constructor exist for it.
func TestAnOptionalPointerIsStillThePointer(t *testing.T) {
	entry := writeEnumProject(t, `module app

struct Thing {
    value: u32
}

extern fn get_thing(name: Ptr[u8]) Option[Ptr[Thing]]
extern fn use_thing(thing: Ptr[Thing]) i32

fn pick(thing: Option[Ptr[Thing]]) i32 {
    match thing {
        Some(p) => { return use_thing(p) }
        None => { return -1 }
    }
}

fn main() void {
    let found: Option[Ptr[Thing]] = get_thing("led")
    let none: Option[Ptr[Thing]] = Option.None
    let again: Option[Ptr[Thing]] = Some(found.value)
    if found.is_some && is_some(again) {
        print(int_to_str(u32(pick(found))))
    }
    print(int_to_str(u32(pick(none))))
}
`)
	module, _, err := compileToIR(nil, entry, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := codegen.EmitC(module)
	for _, want := range []string{"typedef Thing* Option_Ptr_Thing;", "!= NULL)", "Option_Ptr_Thing none = NULL;"} {
		if !strings.Contains(c, want) {
			t.Errorf("the C has no %q:\n%s", want, c)
		}
	}
	if strings.Contains(c, "rho_enum_Option_Ptr") || strings.Contains(c, ".tag") {
		t.Errorf("an optional pointer was built as a tagged enum:\n%s", c)
	}
}
