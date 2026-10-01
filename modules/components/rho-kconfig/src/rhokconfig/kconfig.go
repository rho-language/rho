//
// @file kconfig.go
// @brief Reading Kconfig fragments.
//
// A Kconfig file is a menu of symbols: each one has a type, a prompt, a default
// and a condition, and a fragment is a tree of them. Reading one is therefore
// reading a small configuration language, and this is its reader.
//
// The subset read here is the one a project actually writes:
//
//	config SYMBOL            a symbol, typed by what follows
//	bool NAME / int NAME / hex NAME / string NAME
//	default VALUE            what the symbol starts as
//	range A..B               what an int may be
//	prompt "text" help "text"  what a person is asked
//	depends on EXPR          the condition a symbol is available under
//	select SYMBOL             a symbol this one turns on
//	choice / endchoice       a set of symbols of which one is chosen
//	source "file"            another fragment to read
//	menu "title" / endmenu    a titled group
//
// Anything else is a diagnostic naming the line, rather than a fragment read
// half-way: a configuration a project cannot read is a build that should stop
// with a reason, not one that quietly configures nothing.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhokconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Type is a symbol's kind, which decides how its value is written and read.
type Type string

const (
	// TypeBool is a yes or no symbol, written `=y` or `=n` in every dialect.
	TypeBool Type = "bool"
	// TypeInt is a decimal integer.
	TypeInt Type = "int"
	// TypeHex is a hexadecimal integer.
	TypeHex Type = "hex"
	// TypeString is free text.
	TypeString Type = "string"
)

// Symbol is one configuration symbol.
type Symbol struct {
	// Name is the symbol, written CONFIG_UART_BAUD in every dialect.
	Name string
	// Type is what kind of value it holds.
	Type Type
	// Prompt is what a person is asked when configuring it by hand.
	Prompt string
	// Help is the text under the prompt.
	Help string
	// Default is the value it starts as.
	Default string
	// Range is the span an int or hex may take, empty when unbounded.
	Range string
	// Depends is the condition the symbol is available under, as written.
	Depends string
	// Selects are the symbols this one turns on.
	Selects []string
	// InChoice names the choice this symbol belongs to, empty when it belongs to
	// none.
	InChoice string
	// Line is the line the symbol was declared on, so a diagnostic can point at
	// it.
	Line int
	// Source is the fragment the symbol came from, so a value can be traced back
	// to the file that asked for it.
	Source string
}

// Fragment is a set of symbols read from one or more files.
type Fragment struct {
	// Symbols are the symbols that were read, sorted by name.
	Symbols []Symbol
	// Sources are the files that were read, in the order they were read.
	Sources []string
}

// Fragment returns one symbol by name.
func (f Fragment) Fragment(name string) (Symbol, bool) {
	for _, symbol := range f.Symbols {
		if symbol.Name == name {
			return symbol, true
		}
	}
	return Symbol{}, false
}

/* -------------------------------------------------------------------------- */
/*  Reading                                                                   */
/* -------------------------------------------------------------------------- */

// ReadFragments reads Kconfig files in order, following the `source` lines
// they name, and returns what they declare.
//
// The order matters: two fragments may declare the same symbol, and the later
// one is what a project gets, because a fragment is a statement of what this
// project wants and a project's own fragments come after the ones a target
// ships.
func ReadFragments(root string, paths []string) (Fragment, []Diagnostic) {
	fragment := Fragment{}
	diagnostics := []Diagnostic{}
	seen := map[string]bool{}
	for _, path := range paths {
		readFragmentInto(root, path, &fragment, seen, &diagnostics, 0)
	}
	sort.Slice(fragment.Symbols, func(i, j int) bool { return fragment.Symbols[i].Name < fragment.Symbols[j].Name })
	return fragment, diagnostics
}

// readFragmentInto reads one file, following what it sources.
func readFragmentInto(root, path string, out *Fragment, seen map[string]bool, diagnostics *[]Diagnostic, depth int) {
	if depth > 32 {
		// A fragment that sources itself would otherwise read forever. The bound
		// is a diagnostic rather than a crash, because the author of the fragment
		// is the one who can fix it.
		*diagnostics = append(*diagnostics, Diagnostic{Path: path, Message: "Kconfig fragments are sourced too deeply; check for a cycle"})
		return
	}
	resolved := resolve(root, path)
	if seen[resolved] {
		// A fragment sourced twice is read once: its symbols are declared once,
		// and reading it again would only repeat them.
		return
	}
	seen[resolved] = true

	data, err := os.ReadFile(resolved)
	if err != nil {
		*diagnostics = append(*diagnostics, Diagnostic{Path: path, Message: "cannot read the fragment: " + err.Error()})
		return
	}
	out.Sources = append(out.Sources, resolved)
	readText(string(data), resolved, out, seen, diagnostics, depth)
}

// readText reads the body of one fragment.
func readText(text, path string, out *Fragment, seen map[string]bool, diagnostics *[]Diagnostic, depth int) {
	choice := ""
	// A type written on the line after `config SYMBOL` is the common spelling,
	// so the reader keeps the next line's type until the line is read.
	lines := strings.Split(text, "\n")
	pending := &pendingType{}
	// last is the symbol a prompt, a default or a help belongs to: the one just
	// declared. A keyword before any config has no symbol to belong to, which is
	// a fragment that is not shaped like Kconfig, and is reported below.
	last := func() *Symbol {
		if len(out.Symbols) == 0 {
			return nil
		}
		return &out.Symbols[len(out.Symbols)-1]
	}
	for number, raw := range lines {
		line := number + 1
		trimmed := strings.TrimSpace(stripComment(raw))
		if trimmed == "" {
			continue
		}

		// A quoted string on a line of its own is the help of the symbol
		// before it, which is how a multi-line help is written.
		if strings.HasPrefix(trimmed, "\"") {
			if symbol := last(); symbol != nil {
				symbol.Help = unquote(trimmed)
			}
			continue
		}

		keyword, rest := split(trimmed)
		switch keyword {
		case "config":
			name := firstWord(rest)
			if name == "" {
				*diagnostics = append(*diagnostics, problem(path, line, "config declares no symbol"))
				continue
			}
			// The type follows the name on the same line, or the next line when
			// it does not, which is how Kconfig is normally written.
			kind := TypeBool
			if trailing := firstWord(rest); trailing != "" {
				if typed, ok := parseType(trailing); ok {
					kind = typed
				}
			} else if declared, ok := pending.Peek(); ok {
				kind = declared
				pending.Consume()
			}
			out.Symbols = append(out.Symbols, Symbol{
				Name:     symbolName(name),
				Type:     kind,
				InChoice: choice,
				Line:     line,
				Source:   path,
			})

		case "bool", "int", "hex", "string", "tristate":
			// A type is followed by the prompt on the same line, which is the
			// usual spelling: bool "SPI master".
			typed, isType := parseType(keyword)
			if !isType {
				continue
			}
			// The type belongs to the symbol just declared when it is on the
			// line after it, and to the symbol after it when it is on its own
			// line, which is the other spelling of the same thing.
			if symbol := last(); symbol != nil && symbol.Line == line-1 {
				symbol.Type = typed
				if rest != "" {
					symbol.Prompt = unquote(rest)
				}
				break
			}
			pending.Push(typed)

		case "prompt":
			if symbol := last(); symbol != nil {
				symbol.Prompt = unquote(rest)
			}
		case "help":
			if symbol := last(); symbol != nil {
				symbol.Help = unquote(rest)
			}

		case "default":
			if symbol := last(); symbol != nil {
				symbol.Default = unquote(rest)
			}
		case "range":
			if symbol := last(); symbol != nil {
				symbol.Range = rest
			}
		case "depends":
			if symbol := last(); symbol != nil {
				symbol.Depends = rest
			}
		case "select":
			if symbol := last(); symbol != nil {
				symbol.Selects = append(symbol.Selects, rest)
			}
		case "imply":
			// A soft dependency has no effect on the value written, so it is
			// read and dropped rather than refused: a fragment written for a
			// newer tooling still declares its symbols.
		case "choice":
			choice = firstWord(rest)
		case "endchoice":
			choice = ""
		case "menu", "comment", "endmenu", "mainmenu", "option", "optional", "visible", "modules":
			// Grouping and menu keywords carry no value a build needs, so they
			// are read and passed over.
		case "source", "rsource", "orsource":
			// A sourced fragment is named relative to the fragment that sources
			// it, which is what makes a fragment self-contained once it is
			// copied into a generated project.
			sourced := unquote(firstWord(rest))
			if !filepath.IsAbs(sourced) {
				sourced = filepath.Join(filepath.Dir(path), sourced)
			}
			readFragmentInto("", sourced, out, seen, diagnostics, depth+1)
		default:
			*diagnostics = append(*diagnostics, Diagnostic{
				Path: path, Line: line,
				Message: fmt.Sprintf("cannot read %q; a Kconfig fragment uses config, bool, int, hex, string, menu, choice, source", keyword),
			})
		}
	}
}

// pendingType is a type read on the line after the symbol it belongs to.
type pendingType struct {
	kind  Type
	held  bool
	count int
}

func (p *pendingType) Push(kind Type) { p.kind, p.held, p.count = kind, true, 1 }
func (p *pendingType) Peek() (Type, bool) {
	if !p.held {
		return "", false
	}
	return p.kind, true
}

// Consume takes the type, and lets go of it: a type names the symbol that
// follows it, not the one after that.
func (p *pendingType) Consume() {
	p.held = false
	p.kind = ""
}

// parseType reads a symbol type as it is written.
func parseType(word string) (Type, bool) {
	switch word {
	case "bool", "tristate":
		return TypeBool, true
	case "int":
		return TypeInt, true
	case "hex":
		return TypeHex, true
	case "string":
		return TypeString, true
	}
	return "", false
}

// split reads a line into its first word and the rest.
func split(line string) (string, string) {
	fields := strings.SplitN(line, " ", 2)
	if len(fields) == 1 {
		return fields[0], ""
	}
	return fields[0], strings.TrimSpace(fields[1])
}

// unquote removes the quotes a value may be written in, and keeps the text
// inside them: a Kconfig value may be written "info" or bare, and the two mean
// the same thing.
func unquote(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		if value, err := strconv.Unquote(trimmed); err == nil {
			return value
		}
		return trimmed[1 : len(trimmed)-1]
	}
	return trimmed
}

func firstWord(text string) string {
	word := text
	if index := strings.IndexAny(word, " \t"); index >= 0 {
		word = word[:index]
	}
	return word
}

// stripComment removes a comment from a line, respecting a quoted string so a
// `#` in a prompt is not read as the start of a comment.
func stripComment(line string) string {
	quoted := false
	for index := 0; index < len(line); index++ {
		switch line[index] {
		case '"':
			quoted = !quoted
		case '#':
			if !quoted {
				return line[:index]
			}
		case '/':
			// A comment that starts with `//` is a Kconfig comment as well; a
			// path is never written this way inside a fragment.
			if !quoted && index+1 < len(line) && line[index+1] == '/' {
				return line[:index]
			}
		}
	}
	return line
}

// resolve is a fragment path relative to the tree it was declared in, with the
// generated project's own directory first: a fragment a project writes lives
// beside the project, and one a target ships is referenced relative to it.
func resolve(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	// A path is the project's own, which is how a project's fragments arrive
	// with the project.
	if root != "" {
		if candidate := filepath.Join(root, filepath.FromSlash(path)); fileExists(candidate) {
			return candidate
		}
	}
	// A bare name is a fragment of the component's own, which is where a
	// target's fragments are kept.
	return filepath.Join(fragmentLibrary(), path)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// fragmentLibrary is where the fragments a target ships are kept.
func fragmentLibrary() string {
	if dir := os.Getenv("RHO_KCONFIG_FRAGMENTS"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "kconfig"
	}
	return filepath.Join(home, ".rho", "kconfig", "fragments")
}

// symbolName is the name a symbol is known by everywhere outside a fragment.
//
// A fragment writes `config RH_HAL_UART` — that is how Kconfig spells it, and
// every fragment in every project does. Everything else spells it the way the
// resulting header defines it: `CONFIG_RH_HAL_UART`, in a defconfig, in a Rho
// module of constants, in a driver's own `#if`.
//
// Storing what was written made those two spellings different symbols. A project
// that set a symbol got told nothing declared it, while the same symbol resolved
// from its default — so the value was found, and it was not the value that was
// asked for.
func symbolName(written string) string {
	return "CONFIG_" + strings.TrimPrefix(written, "CONFIG_")
}
