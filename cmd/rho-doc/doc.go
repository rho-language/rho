//
// @file doc.go
// @brief Rho Documentation Generator - Enterprise.
//
// Implements `rho doc`, a deterministic documentation generator for Rho 0.1.
// It parses the module and prints Markdown documentation based on the AST.
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

func GenerateDoc(path string) (string, error) {
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read source: %w", err)
	}

	toks, err := lexer.Lex(string(srcBytes))
	if err != nil {
		return "", fmt.Errorf("lexing failed: %w", err)
	}

	mod, err := parser.Parse(toks)
	if err != nil {
		return "", fmt.Errorf("parsing failed: %w", err)
	}

	return formatDoc(*mod), nil
}

/* -------------------------------------------------------------------------- */
/*  CLI Entry Point                                                           */
/* -------------------------------------------------------------------------- */

func main() {
	fs := flag.NewFlagSet("rho doc", flag.ExitOnError)
	fs.Parse(os.Args[1:])

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: rho doc <file.rho>")
		os.Exit(2)
	}

	in := fs.Arg(0)

	out, err := GenerateDoc(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho doc:", err)
		os.Exit(1)
	}

	fmt.Print(out)
}

/* -------------------------------------------------------------------------- */
/*  Markdown Generator                                                        */
/* -------------------------------------------------------------------------- */

func formatDoc(m ast.Module) string {
	var b strings.Builder

	b.WriteString("# Module `")
	b.WriteString(m.Name)
	b.WriteString("`\n\n")

	/* ---------------------------- Structs ---------------------------------- */
	b.WriteString("## Structs\n\n")
	for _, d := range m.Decls {
		if s, ok := d.(ast.StructDecl); ok {
			formatStruct(&b, s)
		}
	}

	/* ---------------------------- Functions -------------------------------- */
	b.WriteString("## Functions\n\n")
	for _, d := range m.Decls {
		if f, ok := d.(ast.FuncDecl); ok {
			formatFunc(&b, f)
		}
	}

	return b.String()
}

func formatStruct(b *strings.Builder, s ast.StructDecl) {
	b.WriteString("### `")
	b.WriteString(s.Name)
	b.WriteString("`\n\n")

	b.WriteString("| Field | Type |\n")
	b.WriteString("|-------|------|\n")

	for _, f := range s.Fields {
		b.WriteString("| ")
		b.WriteString(f.Name)
		b.WriteString(" | ")
		b.WriteString(f.Type)
		b.WriteString(" |\n")
	}

	b.WriteString("\n")
}

func formatFunc(b *strings.Builder, f ast.FuncDecl) {
	b.WriteString("### `")
	b.WriteString(f.Name)
	b.WriteString("()`\n\n")

	b.WriteString("**Signature:**\n\n```\nfn ")
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
	b.WriteString("\n```\n\n")
}
