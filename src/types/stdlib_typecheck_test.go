package types

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
)

func TestTypecheckStandardLibrarySources(t *testing.T) {
	root := filepath.Join("..", "..", "std")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".rho" {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		tokens, lexErr := lexer.Lex(string(source))
		if lexErr != nil {
			t.Errorf("%s: lex: %v", path, lexErr)
			return nil
		}
		module, parseErr := parser.Parse(tokens)
		if parseErr != nil {
			t.Errorf("%s: parse: %v", path, parseErr)
			return nil
		}
		// A module that imports another is checked with its imports loaded,
		// by TestEveryStandardLibraryBodyTypeChecks in package src: alone, the
		// names it imports are not there.
		if len(module.Imports) > 0 {
			return nil
		}
		if checkErr := New().CheckModule(*module); checkErr != nil {
			t.Errorf("%s: typecheck: %v", path, checkErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A standard library module's function bodies are checked when it is loaded,
// as a program's are, so a mistake in one is found without a program calling it.
func TestAStandardLibraryBodyIsTypeChecked(t *testing.T) {
	source := "module std.broken\n\nfn twice(x: u32) u32 {\n    return \"two\"\n}\n"
	tokens, err := lexer.Lex(source)
	if err != nil {
		t.Fatal(err)
	}
	module, err := parser.Parse(tokens)
	if err != nil {
		t.Fatal(err)
	}
	if err := New().CheckModule(*module); err == nil {
		t.Fatal("a std function whose body returns a string for a u32 was accepted")
	}
}
