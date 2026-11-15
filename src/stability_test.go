//
// @file stability_test.go
// @brief Every standard-library module says how settled it is.
//
// docs/stability.md promises that a std module tagged `@stability stable` breaks
// a program only after a deprecation warning, and that one tagged
// `experimental` may change in any minor release. A module that says neither
// makes no promise anyone can read, so a new module has to choose.
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

	"rho/src/frontend/parser"
)

func TestEveryStdModuleSaysHowStableItIs(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", "std"))
	valid := strings.Join(parser.Stabilities, " or ")
	count := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".rho" {
			return err
		}
		count++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(filepath.Dir(root), path)
		value, ok := parser.HeaderTag(string(data), parser.StabilityTag)
		if !ok {
			t.Errorf("%s has no `// @stability %s` line in its header", relative, valid)
			return nil
		}
		known := false
		for _, stability := range parser.Stabilities {
			known = known || value == stability
		}
		if !known {
			t.Errorf("%s says `@stability %s`, want %s", relative, value, valid)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no std module was found")
	}
}

// docs/language/stdlib.md lists every module with the stability its header
// says, so the page a reader sees and the tag the reference reads agree.
func TestTheStdlibPageListsEachModulesStability(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("..", "docs", "language", "stdlib.md"))
	if err != nil {
		t.Fatal(err)
	}
	root, _ := filepath.Abs(filepath.Join("..", "std"))
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".rho" {
			return err
		}
		data, _ := os.ReadFile(path)
		value, _ := parser.HeaderTag(string(data), parser.StabilityTag)
		module := moduleNameIn(string(data))
		if row := "| `" + module + "` | " + value + " |"; !strings.Contains(string(page), row) {
			t.Errorf("docs/language/stdlib.md has no row %q", row)
		}
		return nil
	})
}

func moduleNameIn(source string) string {
	for _, line := range strings.Split(source, "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}
