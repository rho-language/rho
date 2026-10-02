package parser

import (
	"os"
	"path/filepath"
	"testing"

	"rho/src/frontend/lexer"
)

func TestParseStandardLibrarySources(t *testing.T) {
	root := filepath.Join("..", "..", "..", "std")
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
		if _, parseErr := Parse(tokens); parseErr != nil {
			t.Errorf("%s: parse: %v", path, parseErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
