//
// @file fmt.go
// @brief Rho Formatter - Enterprise.
//
// Deterministic formatter for Rho 0.1.
// Re-parses the input file, then prints the AST in canonical form.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

func FormatFile(path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read source: %w", err)
	}

	toks, err := lexer.Lex(string(src))
	if err != nil {
		return "", fmt.Errorf("lexing failed: %w", err)
	}

	mod, err := parser.Parse(toks)
	if err != nil {
		return "", fmt.Errorf("parsing failed: %w", err)
	}

	return formatModule(*mod), nil
}

/* -------------------------------------------------------------------------- */
/*  CLI Entry Point                                                           */
/* -------------------------------------------------------------------------- */

func main() {
	fs := flag.NewFlagSet("rho fmt", flag.ExitOnError)
	write := fs.Bool("w", false, "write result back to file")

	fs.Parse(os.Args[1:])
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: rho fmt [-w] <file.rho>")
		os.Exit(2)
	}

	in := fs.Arg(0)

	out, err := FormatFile(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho fmt:", err)
		os.Exit(1)
	}

	if *write {
		os.WriteFile(in, []byte(out), 0644)
		return
	}

	fmt.Print(out)
}

/* -------------------------------------------------------------------------- */
/*  Formatter Implementation                                                  */
/* -------------------------------------------------------------------------- */

func formatModule(m ast.Module) string {
	var b strings.Builder

	b.WriteString("module ")
	b.WriteString(m.Name)
	b.WriteString("\n\n")

	for i, d := range m.Decls {
		if i > 0 {
			b.WriteString("\n")
		}
		switch x := d.(type) {
		case ast.StructDecl:
			formatStruct(&b, x)
		case ast.FuncDecl:
			formatFunc(&b, x)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func formatStruct(b *strings.Builder, s ast.StructDecl) {
	b.WriteString("struct ")
	b.WriteString(s.Name)
	b.WriteString(" {\n")

	for _, f := range s.Fields {
		b.WriteString("    ")
		b.WriteString(f.Name)
		b.WriteString(": ")
		b.WriteString(f.Type)
		b.WriteString("\n")
	}

	b.WriteString("}")
}

func formatFunc(b *strings.Builder, f ast.FuncDecl) {
	b.WriteString("fn ")
	b.WriteString(f.Name)
	b.WriteString("(")

	for i, p := range f.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name)
		b.WriteString(": ")
		b.WriteString(p.Type)
	}

	b.WriteString(") ")
	b.WriteString(f.ReturnType)
	b.WriteString(" {\n")
	b.WriteString("}")
}
