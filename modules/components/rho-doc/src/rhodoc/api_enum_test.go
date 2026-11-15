//
// @file api_enum_test.go
// @brief An enum is in the reference, each variant as it is declared.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhodoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheReferenceListsEnumsAndTheirVariants(t *testing.T) {
	root := t.TempDir()
	source := `module shapes

enum Mode { Off, On = 4 }

enum Shape[T] {
    Circle(T)
    Rect { w: T, h: T }
    Empty
}
`
	if err := os.WriteFile(filepath.Join(root, "shapes.rho"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	modules, diagnostics := ReadApiSources(root)
	if len(diagnostics) != 0 || len(modules) != 1 {
		t.Fatalf("modules %v, diagnostics %v", modules, diagnostics)
	}
	enums := modules[0].Enums
	if len(enums) != 2 || enums[0].Name != "Mode" || enums[1].NameWithParams() != "Shape[T]" {
		t.Fatalf("enums: %+v", enums)
	}
	if got := strings.Join(enums[0].Variants, ", "); got != "Off = 0, On = 4" {
		t.Errorf("a plain enum's variants are %q", got)
	}
	if got := strings.Join(enums[1].Variants, ", "); got != "Circle(T), Rect { w: T, h: T }, Empty" {
		t.Errorf("an enum's variants are %q", got)
	}
	if text := ApiSearchText(modules[0]); !strings.Contains(text, "Shape") || !strings.Contains(text, "Rect") {
		t.Errorf("the search text has no enum: %q", text)
	}
	if page := RenderApiModule(modules[0]); page == nil || len(page.Blocks) == 0 {
		t.Error("no page was rendered")
	}
}

// A module's `@stability` tag and a `@deprecated` line are on its reference
// page: the stability in the sentence it opens with, a deprecation beside the
// declaration it is about.
func TestTheReferenceSaysHowSettledAModuleIsAndWhatIsDeprecated(t *testing.T) {
	root := t.TempDir()
	source := `//
// @file files.rho
// @stability experimental
//
module files

// read_all answers the file.
// @deprecated use read_bytes instead
fn read_all(path: string) string {
    return path
}

fn read_bytes(path: string) string {
    return path
}

// @deprecated use Size instead
struct OldSize {
    w: u32
}

enum Mode {
    Off
    // @deprecated use On
    Lit
    On
}
`
	if err := os.WriteFile(filepath.Join(root, "files.rho"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	modules, diagnostics := ReadApiSources(root)
	if len(diagnostics) != 0 || len(modules) != 1 {
		t.Fatalf("modules %v, diagnostics %v", modules, diagnostics)
	}
	module := modules[0]
	if module.Stability != "experimental" {
		t.Errorf("the stability is %q", module.Stability)
	}
	if module.Functions[0].Name != "read_all" || module.Functions[0].Deprecation == nil || module.Functions[1].Deprecation != nil {
		t.Errorf("functions: %+v", module.Functions)
	}
	html := renderedText(RenderApiModule(module))
	for _, want := range []string{"Stability: experimental.", "Deprecated: use read_bytes instead", "OldSize (deprecated: use Size instead)", "<code>Lit = 1</code> (deprecated: use On)"} {
		if !strings.Contains(html, want) {
			t.Errorf("the page has no %q:\n%s", want, html)
		}
	}
	if index := renderedText(RenderApiIndex(modules, "api")); !strings.Contains(index, "experimental") {
		t.Errorf("the index does not list the stability:\n%s", index)
	}
}

func renderedText(document *Document) string {
	return RenderPage(document).Body
}
