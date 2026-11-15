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
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
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
//
// Every comment in the source is in the result. A source whose comments the
// printer would not give back exactly is refused with an error rather than
// formatted, so a file is never written with a comment missing.
func FormatSource(src string) (string, error) {
	toks, list, err := lexer.LexComments(src)
	if err != nil {
		return "", fmt.Errorf("lexing failed: %w", err)
	}
	toks, tests := stripTestMarkers(toks)
	mod, err := parser.Parse(toks)
	if err != nil {
		return "", fmt.Errorf("parsing failed: %w", err)
	}
	comments = newCommentSet(toks, list)
	comments.tests = tests
	defer func() { comments = nil }()
	formatted := formatModule(*mod)
	if err := sameComments(list, formatted); err != nil {
		return "", err
	}
	return formatted, nil
}

// ParseRho parses Rho source, so callers can compare a formatted file against
// the original by tree rather than by text.
func ParseRho(src string) (*ast.Module, error) {
	toks, err := lexer.Lex(src)
	if err != nil {
		return nil, err
	}
	toks, _ = stripTestMarkers(toks)
	return parser.Parse(toks)
}

// stripTestMarkers takes the `test` off each `test fn` declaration of a test
// file, which the compiler's parser does not read (the test runner removes it
// before a build), and says which functions carried it, by the position of
// their `fn`, so the printer can put it back.
func stripTestMarkers(toks []lexer.Token) ([]lexer.Token, map[[2]int]bool) {
	tests := map[[2]int]bool{}
	out := make([]lexer.Token, 0, len(toks))
	depth := 0
	for i, t := range toks {
		switch t.Kind {
		case lexer.TokenLBrace, lexer.TokenLParen, lexer.TokenLBracket:
			depth++
		case lexer.TokenRBrace, lexer.TokenRParen, lexer.TokenRBracket:
			depth--
		}
		// The marker begins a declaration, so it begins its line: `module test`
		// followed by a function is a module named test.
		beginsLine := i > 0 && toks[i-1].Line < t.Line && toks[i-1].Kind != lexer.TokenModule &&
			toks[i-1].Kind != lexer.TokenDot && toks[i-1].Kind != lexer.TokenImport
		if depth == 0 && beginsLine && t.Kind == lexer.TokenIdent && t.Lex == "test" &&
			i+1 < len(toks) && toks[i+1].Kind == lexer.TokenFn {
			tests[[2]int{toks[i+1].Line, toks[i+1].Column}] = true
			continue
		}
		out = append(out, t)
	}
	return out, tests
}

// isTest reports whether a function was declared `test fn`.
func isTest(x ast.FuncDecl) bool {
	return comments != nil && comments.tests[[2]int{x.Pos.Line, x.Pos.Column}]
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
		// With neither -w, -l nor -check the result is the output, whether or
		// not it differs, so a pipeline always gets the file.
		printing := !*write && !*list && !*check
		if formatted == string(original) {
			if printing {
				fmt.Print(formatted)
			}
			continue
		}
		changed = true
		// -check names what needs formatting and prints no source: it is the
		// question a build asks, and the answer is a list and an exit status.
		if *list || *check {
			fmt.Println(in)
		}
		if *write {
			if err := os.WriteFile(in, []byte(formatted), 0644); err != nil {
				fmt.Fprintln(os.Stderr, "rho fmt:", err)
				os.Exit(1)
			}
		}
		if printing {
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
// written in, and the comments where they were written.
func formatModule(m ast.Module) string {
	var b strings.Builder

	// The header above `module` is the file's own comment: its tags, its
	// licence. It is printed as it was, before the module line.
	flushBefore(&b, 0, 0, true)

	declStarts := declStartTokens(m)
	firstDecl := -1
	if len(declStarts) > 0 {
		firstDecl = declStarts[0]
	}
	items := headerItems(m, firstDecl)
	eof := -1
	if comments != nil {
		eof = len(comments.toks) - 1
	}

	// The module declaration is dotted, for example `module a.b.c`.
	var starts []int
	for _, item := range items {
		starts = append(starts, item.start)
	}
	starts = append(starts, declStarts...)
	moduleEnd := eof - 1
	if len(starts) > 0 {
		moduleEnd = starts[0] - 1
		if starts[0] < 0 {
			moduleEnd = -1
		}
	}
	if comments == nil {
		moduleEnd = -1
	}
	b.WriteString("module " + strings.Join(strings.Split(m.Name, "."), "."))
	endLine(&b, moduleEnd, 0)

	// The namespace, includes, imports and features belong with the module, in
	// source order, one block separated from the module line and from the first
	// declaration.
	ends := unitEnds(starts, eof)
	for i, item := range items {
		if i == 0 {
			b.WriteString("\n")
		}
		flushBefore(&b, item.start, 0, true)
		b.WriteString(item.text)
		endLine(&b, ends[i], 0)
	}

	// A blank line separates each declaration. The printer already ends a
	// declaration with a newline, so nothing more is added here: a file ends
	// with exactly one.
	for i, decl := range m.Decls {
		b.WriteString("\n")
		start, end := declStarts[i], ends[len(items)+i]
		flushBefore(&b, start, 0, true)
		printDecl(&b, decl, start, end)
	}
	flushRest(&b)
	return b.String()
}

// headerItem is one line between the module line and the declarations.
type headerItem struct {
	text  string
	start int
}

// headerItems are the namespace, includes, imports and features in the order
// the parser reads them, which is the order they are written in, each with the
// token it begins at in the source.
func headerItems(m ast.Module, firstDecl int) []headerItem {
	var items []headerItem
	if m.Namespace != "" {
		items = append(items, headerItem{text: "namespace " + m.Namespace})
	}
	for _, include := range m.Includes {
		items = append(items, headerItem{text: "include " + quoteRhoString(include.Path)})
	}
	for _, importName := range m.Imports {
		items = append(items, headerItem{text: "import " + importName.Name})
	}
	for _, feature := range m.Features {
		items = append(items, headerItem{text: "feature " + quoteRhoString(feature)})
	}
	if comments == nil {
		for i := range items {
			items[i].start = -1
		}
		return items
	}
	limit := firstDecl
	if limit < 0 {
		limit = len(comments.toks) - 1
	}
	var found []int
	for i := 1; i < limit; i++ {
		t := comments.toks[i]
		switch {
		case t.Kind == lexer.TokenIdent && t.Lex == "namespace" && m.Namespace != "" && len(found) == 0:
			found = append(found, i)
		case t.Kind == lexer.TokenInclude, t.Kind == lexer.TokenImport:
			found = append(found, i)
		case t.Kind == lexer.TokenIdent && t.Lex == "feature" && kindAt(i+1) == lexer.TokenString:
			found = append(found, i)
		}
	}
	for i := range items {
		items[i].start = -1
		if len(found) == len(items) {
			items[i].start = found[i]
		}
	}
	return items
}

// declStartTokens is the token each declaration begins at. A message and an rpc
// are not positioned by the parser, so they are found by their keyword and name.
func declStartTokens(m ast.Module) []int {
	starts := make([]int, len(m.Decls))
	prev := 0
	for i, decl := range m.Decls {
		start := tokOf(decl.Position())
		if start < 0 && comments != nil {
			keyword, name := "", ""
			switch x := decl.(type) {
			case ast.MessageDecl:
				keyword, name = "message", x.Name
			case ast.RpcDecl:
				keyword, name = "rpc", x.Name
			}
			for j := prev + 1; keyword != "" && j+1 < len(comments.toks); j++ {
				t := comments.toks[j]
				if t.Kind == lexer.TokenIdent && t.Lex == keyword && comments.toks[j+1].Lex == name {
					start = j
					break
				}
			}
		}
		// Declarations are in source order; one found out of order is not used.
		if start >= 0 && start <= prev && i > 0 {
			start = -1
		}
		if start >= 0 {
			prev = start
		}
		starts[i] = start
	}
	return starts
}

// printDecl prints one top-level declaration, which begins at the token start
// and ends at the token end in the source (-1 when not known).
func printDecl(b *strings.Builder, decl ast.Decl, start, end int) {
	// A declaration on one line has every comment inside it printed above it.
	single := func() {
		if !hasClosure(start, end) {
			flushBefore(b, end, 0, false)
		}
	}
	switch x := decl.(type) {
	case ast.TypeDecl:
		single()
		b.WriteString("type " + x.Name + " = " + x.Type.String())
		endLine(b, end, 0)

	case ast.RouteDecl:
		single()
		b.WriteString("route " + x.Method + " \"" + x.Path + "\" => " + x.Handler)
		endLine(b, end, 0)

	case ast.ConstDecl:
		single()
		b.WriteString("const " + x.Name)
		if x.Type.Name != "" || len(x.Type.Args) > 0 ||
			x.Type.ArrayLen != nil || len(x.Type.Dims) > 0 {
			b.WriteString(": " + typeWithArgs(x.Type))
		}
		if x.Value != nil {
			b.WriteString(" = ")
			statementDepth = 0
			printExpr(b, x.Value, 0)
		}
		endLine(b, end, 0)

	case ast.StructDecl:
		printFieldBlock(b, "struct "+x.Name+typeParams(x.TypeParams, x.TypeParamDefaults), x.Fields, start, end, 0)

	case ast.MessageDecl:
		printFieldBlock(b, "message "+x.Name, x.Fields, start, end, 0)

	case ast.RpcDecl:
		printRpc(b, x, start, end)

	case ast.EnumDecl:
		open := braceFrom(start)
		close := pairOf(open)
		flushBefore(b, open, 0, false)
		b.WriteString("enum " + x.Name + typeParams(x.TypeParams, x.TypeParamDefaults) + " {")
		endLine(b, open, 1)
		starts := make([]int, len(x.Variants))
		for i, variant := range x.Variants {
			starts[i] = tokOf(variant.Pos)
		}
		ends := unitEnds(starts, close)
		for i, variant := range x.Variants {
			flushBefore(b, starts[i], 1, true)
			flushBefore(b, ends[i], 1, false)
			b.WriteString(indent + variant.Name)
			switch {
			case variant.Named:
				parts := make([]string, len(variant.Fields))
				for i, f := range variant.Fields {
					parts[i] = f.Name + ": " + fieldType(f)
				}
				b.WriteString(" { " + strings.Join(parts, ", ") + " }")
			case len(variant.Fields) > 0:
				parts := make([]string, len(variant.Fields))
				for i, f := range variant.Fields {
					parts[i] = fieldType(f)
				}
				b.WriteString("(" + strings.Join(parts, ", ") + ")")
			case variant.Explicit:
				b.WriteString(fmt.Sprintf(" = %d", variant.Value))
			}
			endLine(b, ends[i], 1)
		}
		flushBefore(b, close, 1, false)
		b.WriteString("}")
		endLine(b, end, 0)

	case ast.FuncDecl:
		if x.Extern {
			single()
			b.WriteString("extern fn " + x.Name + boundedTypeParams(x.TypeParams, x.TypeParamDefaults, x.TypeParamBounds))
			b.WriteString("(")
			printParams(b, x.Params)
			b.WriteString(")")
			if x.ReturnType != "" {
				b.WriteString(" -> " + x.ReturnType)
			}
			endLine(b, end, 0)
			return
		}
		open := braceFrom(start)
		close := pairOf(open)
		flushBefore(b, open, 0, false)
		if isTest(x) {
			b.WriteString("test ")
		}
		b.WriteString("fn " + x.Name + boundedTypeParams(x.TypeParams, x.TypeParamDefaults, x.TypeParamBounds))
		b.WriteString("(")
		printParams(b, x.Params)
		b.WriteString(")")
		if x.ReturnType != "" {
			b.WriteString(" " + x.ReturnType)
		}
		b.WriteString(" {")
		endLine(b, open, 1)
		printStmts(b, x.Body, 1, close)
		b.WriteString("}")
		endLine(b, end, 0)

	default:
		b.WriteString("/* unprinted declaration */\n")
	}
}

// printFieldBlock prints a header and a braced list of fields, one a line: a
// struct, a message, an rpc's request or response.
func printFieldBlock(b *strings.Builder, header string, fields []ast.Field, start, end, depth int) {
	pad := strings.Repeat(indent, depth)
	open := braceFrom(start)
	close := pairOf(open)
	flushBefore(b, open, depth, false)
	b.WriteString(pad + header + " {")
	endLine(b, open, depth+1)
	starts := fieldStarts(open, close, len(fields))
	ends := unitEnds(starts, close)
	for i, f := range fields {
		flushBefore(b, starts[i], depth+1, true)
		flushBefore(b, ends[i], depth+1, false)
		b.WriteString(pad + indent + f.Name + ": " + fieldType(f))
		if f.BitWidth != nil {
			b.WriteString(fmt.Sprintf(" : %d", *f.BitWidth))
		}
		endLine(b, ends[i], depth+1)
	}
	flushBefore(b, close, depth+1, false)
	b.WriteString(pad + "}")
	endLine(b, end, depth)
}

// printRpc prints an rpc's request and response, in the order they were
// written.
func printRpc(b *strings.Builder, x ast.RpcDecl, start, end int) {
	open := braceFrom(start)
	close := pairOf(open)
	flushBefore(b, open, 0, false)
	b.WriteString("rpc " + x.Name + " {")
	endLine(b, open, 1)
	type section struct {
		name  string
		start int
	}
	var sections []section
	if open >= 0 && close >= 0 {
		for i := open + 1; i < close; i++ {
			t := comments.toks[i]
			if (t.Lex == "request" || t.Lex == "response") && kindAt(i+1) == lexer.TokenLBrace {
				sections = append(sections, section{t.Lex, i})
				i = pairOf(i + 1)
				if i < 0 {
					sections = nil
					break
				}
			}
		}
	}
	if sections == nil {
		sections = []section{{"request", -1}, {"response", -1}}
	}
	starts := make([]int, len(sections))
	for i, s := range sections {
		starts[i] = s.start
	}
	ends := unitEnds(starts, close)
	for i, s := range sections {
		fields := x.Request
		if s.name == "response" {
			fields = x.Response
		}
		flushBefore(b, s.start, 1, true)
		printFieldBlock(b, s.name, fields, s.start, ends[i], 1)
	}
	flushBefore(b, close, 1, false)
	b.WriteString("}")
	endLine(b, end, 0)
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
	return boundedTypeParams(params, defaults, nil)
}

// boundedTypeParams is a function's type parameters with the constraints
// written on them: `[T: Ordered + Equal, U = Unit]`.
func boundedTypeParams(params []string, defaults map[string]string, bounds map[string][]ast.TypeBound) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, 0, len(params))
	for _, param := range params {
		part := param
		if written := bounds[param]; len(written) > 0 {
			part += ": " + ast.BoundsText(written)
		}
		if def, ok := defaults[param]; ok && def != "" {
			part += " = " + def
		}
		parts = append(parts, part)
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
