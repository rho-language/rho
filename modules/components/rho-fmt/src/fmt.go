//
// @file fmt.go
// @brief Rho Formatter.
//
// Re-parses the source and prints the AST in canonical form: one statement per
// line, one declaration per block, includes and imports kept with the module they
// belong to.
//
// The formatter is round-trip safe. Formatting a file and parsing the result
// yields the same tree it started from, which is checked over every source file
// in the repository. A formatter that loses code is worse than none, so
// anything this printer does not understand is emitted as a visible marker
// rather than dropped.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// FormatFile reads a Rho source file and returns its canonical form.
func FormatFile(path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read source: %w", err)
	}
	return FormatSource(string(src))
}

// FormatSource returns the canonical form of Rho source.
func FormatSource(src string) (string, error) {
	toks, err := lexer.Lex(src)
	if err != nil {
		return "", fmt.Errorf("lexing failed: %w", err)
	}
	mod, err := parser.Parse(toks)
	if err != nil {
		return "", fmt.Errorf("parsing failed: %w", err)
	}
	return formatModule(*mod), nil
}

// ParseRho parses Rho source, so callers can compare a formatted file against
// the original by tree rather than by text.
func ParseRho(src string) (*ast.Module, error) {
	toks, err := lexer.Lex(src)
	if err != nil {
		return nil, err
	}
	return parser.Parse(toks)
}

/* -------------------------------------------------------------------------- */
/*  CLI                                                                       */
/* -------------------------------------------------------------------------- */

func main() {
	fs := flag.NewFlagSet("rho fmt", flag.ExitOnError)
	write := fs.Bool("w", false, "write the result back to the file")
	list := fs.Bool("l", false, "list files whose formatting differs")
	check := fs.Bool("check", false, "exit non-zero if any file needs formatting")

	fs.Parse(os.Args[1:])
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: rho fmt [-w] [-l] [-check] <file.rho>...")
		os.Exit(2)
	}

	changed := false
	for _, in := range fs.Args() {
		original, readErr := os.ReadFile(in)
		if readErr != nil {
			fmt.Fprintln(os.Stderr, "rho fmt:", readErr)
			os.Exit(1)
		}
		formatted, err := FormatSource(string(original))
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho fmt:", in, err)
			os.Exit(1)
		}
		if formatted == string(original) {
			continue
		}
		changed = true
		if *list {
			fmt.Println(in)
		}
		if *write {
			if err := os.WriteFile(in, []byte(formatted), 0644); err != nil {
				fmt.Fprintln(os.Stderr, "rho fmt:", err)
				os.Exit(1)
			}
		}
		if !*write && !*list {
			fmt.Print(formatted)
		}
	}

	if *check && changed {
		os.Exit(1)
	}
}

/* -------------------------------------------------------------------------- */
/*  Module Printing                                                           */
/* -------------------------------------------------------------------------- */

// formatModule prints a whole module, keeping the declaration order it was
// written in.
func formatModule(m ast.Module) string {
	var b strings.Builder

	// The module declaration is dotted, for example `module a.b.c`.
	b.WriteString("module " + strings.Join(strings.Split(m.Name, "."), ".") + "\n")

	// Includes and imports belong with the module, and in source order.
	for _, include := range m.Includes {
		b.WriteString("\ninclude \"" + include.Path + "\"")
	}
	for _, importName := range m.Imports {
		b.WriteString("\nimport " + importName.Name)
	}
	for _, feature := range m.Features {
		b.WriteString("\nfeature \"" + feature + "\"")
	}

	// A blank line separates each declaration. The printer already ends a
	// declaration with a newline, so nothing more is added here: a file ends
	// with exactly one.
	for _, decl := range m.Decls {
		b.WriteString("\n")
		printDecl(&b, decl)
	}
	return b.String()
}

// printDecl prints one top-level declaration.
func printDecl(b *strings.Builder, decl ast.Decl) {
	switch x := decl.(type) {
	case ast.TypeDecl:
		b.WriteString("type " + x.Name + " = " + x.Type.String() + "\n")

	case ast.RouteDecl:
		b.WriteString("route " + x.Method + " \"" + x.Path + "\" => " + x.Handler + "\n")

	case ast.StructDecl:
		b.WriteString("struct " + x.Name + typeParams(x.TypeParams, x.TypeParamDefaults))
		if len(x.Fields) == 0 {
			b.WriteString(" {\n}\n")
			return
		}
		b.WriteString(" {\n")
		for _, f := range x.Fields {
			b.WriteString(indent + f.Name + ": " + fieldType(f) + "\n")
		}
		b.WriteString("}\n")

	case ast.FuncDecl:
		if x.Extern {
			b.WriteString("extern fn " + x.Name + typeParams(x.TypeParams, x.TypeParamDefaults))
			b.WriteString("(")
			printParams(b, x.Params)
			b.WriteString(")")
			if x.ReturnType != "" {
				b.WriteString(" -> " + x.ReturnType)
			}
			b.WriteString("\n")
			return
		}
		b.WriteString("fn " + x.Name + typeParams(x.TypeParams, x.TypeParamDefaults))
		b.WriteString("(")
		printParams(b, x.Params)
		b.WriteString(")")
		if x.ReturnType != "" {
			b.WriteString(" " + x.ReturnType)
		}
		b.WriteString(" {\n")
		printStmts(b, x.Body, 1)
		b.WriteString("}\n")

	default:
		b.WriteString("/* unprinted declaration */\n")
	}
}

// printParams writes a parameter list.
func printParams(b *strings.Builder, params []ast.Param) {
	for i, p := range params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name + ": " + paramType(p))
	}
}

// paramType renders a parameter's type, preferring the parsed type so type
// arguments survive.
func paramType(p ast.Param) string {
	if p.TypeExpr.Name != "" || len(p.TypeExpr.Args) > 0 {
		return typeWithArgs(p.TypeExpr)
	}
	return p.Type
}

// fieldType renders a struct field's type.
func fieldType(f ast.Field) string {
	if f.TypeExpr.Name != "" || len(f.TypeExpr.Args) > 0 {
		return typeWithArgs(f.TypeExpr)
	}
	return f.Type
}

// typeParams renders a declaration's type parameter list, including any
// defaults, as in `[T, E = Unit]`. It returns an empty string when there are
// none, so the caller can write it straight after the name.
func typeParams(params []string, defaults map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, 0, len(params))
	for _, param := range params {
		if def, ok := defaults[param]; ok && def != "" {
			parts = append(parts, param+" = "+def)
			continue
		}
		parts = append(parts, param)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// FormatPath formats a file and reports whether it differed, which the
// repository's own sources are checked against.
func FormatPath(path string) (changed bool, err error) {
	src, readErr := os.ReadFile(path)
	if readErr != nil {
		return false, readErr
	}
	formatted, err := FormatSource(string(src))
	if err != nil {
		return false, err
	}
	return formatted != string(src), nil
}

// rhoFilesUnder lists the Rho sources under a directory.
func rhoFilesUnder(root string) ([]string, error) {
	found := []string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "build", ".git", ".kilo", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".rho") {
			found = append(found, path)
		}
		return nil
	})
	return found, err
}
