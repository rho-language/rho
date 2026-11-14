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
