package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// A platform's plugin may ship ports of its own: Go source for the host
// primitives that platform's programs call, which only a host can supply. The
// toolchain carries the ports every project may ask for (logging, a store, SQL,
// routing) and none that belong to one ecosystem.
//
// A shipped port is a complete Go file in package main. It is read for its
// imports and its declarations, which are merged into the project's single port
// file beside the built-in ones, so the plugin's source is ordinary Go that can
// be formatted, vetted and tested where it lives.

// SplitPortSource splits a Go source file into the imports it declares and the
// text after them.
func SplitPortSource(name, source string) ([]string, string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, source, parser.ImportsOnly|parser.ParseComments)
	if err != nil {
		return nil, "", fmt.Errorf("port %s: %w", name, err)
	}
	if file.Name.Name != "main" {
		return nil, "", fmt.Errorf("port %s: a port is in package main, not %s", name, file.Name.Name)
	}
	var imports []string
	end := fset.Position(file.Name.End()).Offset
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok || generic.Tok != token.IMPORT {
			break
		}
		for _, spec := range generic.Specs {
			imported := spec.(*ast.ImportSpec)
			text := imported.Path.Value
			if imported.Name != nil {
				text = imported.Name.Name + " " + text
			}
			imports = append(imports, text)
		}
		end = fset.Position(generic.End()).Offset
	}
	return imports, strings.TrimLeft(source[end:], "\n"), nil
}
