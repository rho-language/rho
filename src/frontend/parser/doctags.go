//
// @file doctags.go
// @brief The `@tags` a doc comment carries, read beside the parse.
//
// A declaration's doc comment is the `//` lines written directly above it, with
// no blank line between. A line of it that begins with a tag says something a
// tool reads rather than a person:
//
//	// read_all answers every byte of a file.
//	// @deprecated use read_bytes instead
//	fn read_all(path: string) -> Slice[u8] { ... }
//
// The lexer drops comments, so the tags are read from the source text by line,
// after the parse has said where each declaration begins. A module's own tags
// are in the comment header above its `module` line, which is where `@backends`
// and `@port` already are.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package parser

import (
	"strings"

	"rho/src/frontend/ast"
)

// DeprecatedTag is the tag that marks a declaration, or a module, deprecated.
const DeprecatedTag = "@deprecated"

// StabilityTag is the tag a standard-library module's header says how settled
// its API is with: `stable` or `experimental`.
const StabilityTag = "@stability"

// Stabilities are the values a `@stability` tag may take.
var Stabilities = []string{"stable", "experimental"}

// AttachDocTags reads the doc-comment tags of a parsed file's declarations
// from its source and records them on the tree: a `@deprecated` line on a
// function, struct, enum, enum variant, constant or type alias.
func AttachDocTags(module *ast.Module, source string) {
	if module == nil || !strings.Contains(source, DeprecatedTag) {
		return
	}
	lines := strings.Split(source, "\n")
	for i, decl := range module.Decls {
		switch d := decl.(type) {
		case ast.FuncDecl:
			d.Deprecated = deprecationAbove(lines, d.Pos)
			module.Decls[i] = d
		case ast.StructDecl:
			d.Deprecated = deprecationAbove(lines, d.Pos)
			module.Decls[i] = d
		case ast.EnumDecl:
			d.Deprecated = deprecationAbove(lines, d.Pos)
			variants := append([]ast.EnumVariant{}, d.Variants...)
			for j := range variants {
				variants[j].Deprecated = deprecationAbove(lines, variants[j].Pos)
			}
			d.Variants = variants
			module.Decls[i] = d
		case ast.ConstDecl:
			d.Deprecated = deprecationAbove(lines, d.Pos)
			module.Decls[i] = d
		case ast.TypeDecl:
			d.Deprecated = deprecationAbove(lines, d.Pos)
			module.Decls[i] = d
		}
	}
}

// ModuleDeprecation is the `@deprecated` line of a module's header — the
// comments above its `module` line — or nil when it has none.
func ModuleDeprecation(source string) *ast.Deprecation {
	if note, ok := HeaderTag(source, DeprecatedTag); ok {
		return &ast.Deprecation{Note: note}
	}
	return nil
}

// HeaderTag is the value of a tag in a module's header: the `//` comments
// before its `module` line. A file with no `module` line has no header. A colon
// after the tag is allowed, `@stability: stable`.
func HeaderTag(source, tag string) (string, bool) {
	value, found := "", false
	for _, raw := range strings.Split(source, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "module ") {
			return value, found
		}
		if !strings.HasPrefix(line, "//") || found {
			continue
		}
		value, found = tagValue(strings.TrimSpace(strings.TrimPrefix(line, "//")), tag)
	}
	return "", false
}

// deprecationAbove reads the doc comment directly above a position — the run
// of `//` lines that ends on the line before it — for a `@deprecated` line. A
// declaration that does not begin its line has no doc comment of its own: the
// comment above `enum Mode { Off, On }` is the enum's, not `On`'s.
func deprecationAbove(lines []string, at ast.Position) *ast.Deprecation {
	if at.Line < 2 || at.Line > len(lines) || !beginsLine(lines[at.Line-1], at.Column) {
		return nil
	}
	for line := at.Line - 2; line >= 0; line-- {
		text := strings.TrimSpace(lines[line])
		if !strings.HasPrefix(text, "//") {
			return nil
		}
		if note, ok := tagValue(strings.TrimSpace(strings.TrimPrefix(text, "//")), DeprecatedTag); ok {
			return &ast.Deprecation{Note: note}
		}
	}
	return nil
}

// beginsLine reports whether only white space comes before a column.
func beginsLine(line string, column int) bool {
	count := 0
	for _, r := range line {
		count++
		if count >= column {
			return true
		}
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

// tagValue is the text after a tag at the start of a comment line, or false
// when the line does not begin with the tag.
func tagValue(comment, tag string) (string, bool) {
	if !strings.HasPrefix(comment, tag) {
		return "", false
	}
	rest := comment[len(tag):]
	if rest != "" && rest[0] != ' ' && rest[0] != ':' && rest[0] != '\t' {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), ":")), true
}
