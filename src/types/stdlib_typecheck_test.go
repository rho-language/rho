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
		if checkErr := New().CheckModule(*module); checkErr != nil {
			t.Errorf("%s: typecheck: %v", path, checkErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
