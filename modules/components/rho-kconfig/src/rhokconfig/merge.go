//
// @file merge.go
// @brief Merging a project's configuration.
//
// A configuration is the fragments' symbols with values set on them, and a value
// comes from somewhere. Where is the whole of the design, so it is written in
// one place: sources are read in order, and a later source wins.
//
//	1. defaults, from the fragments themselves
//	2. defconfig, the baseline a project commits
//	3. the values a module.yml sets
//	4. a module of Rho constants, which is a project's own opinion
//
// Every set value is checked against the symbol it names: a value for a symbol
// no fragment declares is a diagnostic rather than a value nothing reads, and a
// value of the wrong type is a diagnostic rather than a header that does not
// compile four files later.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhokconfig

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Value is one resolved symbol.
type Value struct {
	Name  string
	Type  Type
	Value string
	// From names the layer the value came from, so a person can see why a
	// symbol is what it is.
	From string
	// Prompt, Help, Depends and Default are what the fragments declared, carried
	// through so a dialect can write them and a report can explain them.
	Prompt  string
	Help    string
	Default string
	Depends string
}

// Bool reports whether a symbol is set, which is what a bool symbol is asked.
func (v Value) Bool() bool {
	switch strings.ToLower(v.Value) {
	case "y", "yes", "true", "1", "on":
		return true
	}
	return false
}

// Int reports the value as an integer, for an int or hex symbol.
func (v Value) Int() (int64, bool) {
	base := 10
	if v.Type == TypeHex {
		base = 16
	}
	trimmed := strings.TrimPrefix(strings.TrimSpace(v.Value), "0x")
	if v.Type == TypeHex {
		trimmed = strings.TrimPrefix(strings.TrimSpace(v.Value), "0x")
	}
	number, err := strconv.ParseInt(trimmed, base, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// Diagnostic is something found while reading or merging a configuration. A
// problem is one that changed the answer; a note is one that did not, and a
// caller decides what to do about each.
type Diagnostic struct {
	Path    string
	Line    int
	Message string
	// severity is a note or a problem. A note is reported and does not fail a
	// build: a file that was kept is something a person may want to know, not a
	// configuration that is wrong.
	severity severity
}

// severity is how much a diagnostic matters.
type severity int

const (
	// severityNote is reported and does not fail a build.
	severityNote severity = iota
	// severityProblem is reported and fails a build.
	severityProblem
)

// Problem reports whether a diagnostic is one that changes the answer, which is
// what a build fails on.
func (d Diagnostic) Problem() bool { return d.severity == severityProblem }

// note is a diagnostic that is not a problem.
func note(path, message string) Diagnostic {
	return Diagnostic{Path: path, Message: message, severity: severityNote}
}

// problem is a diagnostic that changes the answer.
func problem(path string, line int, message string) Diagnostic {
	return Diagnostic{Path: path, Line: line, Message: message, severity: severityProblem}
}

func (d Diagnostic) String() string {
	switch {
	case d.Path == "" && d.Line > 0:
		return fmt.Sprintf("line %d: %s", d.Line, d.Message)
	case d.Path == "":
		return d.Message
	case d.Line > 0:
		return fmt.Sprintf("%s:%d: %s", d.Path, d.Line, d.Message)
	default:
		return fmt.Sprintf("%s: %s", d.Path, d.Message)
	}
}

// Source is one layer of values, with the name a report shows.
type Source struct {
	// Name is what the layer is called, in a diagnostic: `defconfig`, `module.yml`.
	Name string
	// Path is where the values came from, when they came from a file.
	Path string
	// Values are the symbols this layer sets.
	Values map[string]string
}

// Configuration is a merged configuration: every symbol a project may set, with
// the value it was given.
type Configuration struct {
	// Values are the resolved symbols, sorted by name.
	Values []Value
	// Sources are the layers that were read, in the order they were read.
	Sources []string
}

// Get returns one resolved symbol by name.
func (c Configuration) Get(name string) (Value, bool) {
	for _, value := range c.Values {
		if value.Name == name {
			return value, true
		}
	}
	return Value{}, false
}

// Names are the symbols that are set, sorted, which is what a listing shows.
func (c Configuration) Names() []string {
	names := make([]string, 0, len(c.Values))
	for _, value := range c.Values {
		names = append(names, value.Name)
	}
	sort.Strings(names)
	return names
}

// Resolve merges the fragments' symbols with the layers that set values on
// them.
//
// The fragments are the vocabulary: a symbol nothing declares cannot be set,
// and a symbol nothing sets keeps its default.
func Resolve(fragment Fragment, sources []Source) (Configuration, []Diagnostic) {
	configuration := Configuration{Sources: fragment.Sources}
	diagnostics := []Diagnostic{}

	resolved := map[string]Value{}
	for _, symbol := range fragment.Symbols {
		value := Value{
			Name:    symbol.Name,
			Type:    symbol.Type,
			Value:   symbol.Default,
			From:    "default",
			Prompt:  symbol.Prompt,
			Help:    symbol.Help,
			Default: symbol.Default,
			Depends: symbol.Depends,
		}
		if value.Value == "" {
			// A symbol with no default is off if it is a flag, and zero
			// otherwise, which is what the tooling that reads these headers
			// expects to find.
			if symbol.Type == TypeBool {
				value.Value = "n"
			} else {
				value.Value = ""
			}
		}
		resolved[symbol.Name] = value
	}

	// A later source wins, and each set value is checked against the symbol it
	// names: a value for a symbol nothing declares is a mistake worth reporting,
	// not a line in a header nothing reads.
	for _, source := range sources {
		names := make([]string, 0, len(source.Values))
		for name := range source.Values {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			raw := source.Values[name]
			existing, known := resolved[name]
			if !known {
				diagnostics = append(diagnostics, problem(source.Path, 0,
					fmt.Sprintf("%s sets %s, which no Kconfig fragment declares", source.Name, name)))
				continue
			}
			value, err := coerce(existing, raw)
			if err != nil {
				diagnostics = append(diagnostics, problem(source.Path, 0,
					fmt.Sprintf("%s sets %s to %q: %v", source.Name, name, raw, err)))
				continue
			}
			value.From = source.Name
			resolved[name] = value
		}
	}

	// A bool symbol that a fragment selects is on even when nothing set it: that
	// is what select means, and a header that ignored it would be wrong.
	for _, symbol := range fragment.Symbols {
		for _, selected := range symbol.Selects {
			if value, known := resolved[selected]; known && value.Value == "n" {
				value.Value = "y"
				value.From = "selected by " + symbol.Name
				resolved[selected] = value
			}
		}
	}

	for _, value := range resolved {
		configuration.Values = append(configuration.Values, value)
	}
	sort.Slice(configuration.Values, func(i, j int) bool { return configuration.Values[i].Name < configuration.Values[j].Name })
	return configuration, diagnostics
}

// coerce reads a set value as the type its symbol declares, and refuses a value
// that is not of that type.
func coerce(symbol Value, raw string) (Value, error) {
	trimmed := strings.TrimSpace(raw)
	switch symbol.Type {
	case TypeBool:
		switch strings.ToLower(trimmed) {
		case "y", "yes", "true", "1", "on":
			symbol.Value = "y"
		case "n", "no", "false", "0", "off":
			symbol.Value = "n"
		default:
			return symbol, fmt.Errorf("a bool is yes or no, not %q", raw)
		}
	case TypeInt:
		number, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return symbol, fmt.Errorf("an int is a decimal number, not %q", raw)
		}
		symbol.Value = strconv.FormatInt(number, 10)
	case TypeHex:
		digits := strings.TrimPrefix(strings.ToLower(trimmed), "0x")
		number, err := strconv.ParseInt(digits, 16, 64)
		if err != nil {
			return symbol, fmt.Errorf("a hex is a hexadecimal number, not %q", raw)
		}
		symbol.Value = fmt.Sprintf("0x%x", number)
	case TypeString:
		symbol.Value = raw
	default:
		symbol.Value = raw
	}
	return symbol, nil
}

/* -------------------------------------------------------------------------- */
/*  Reading a layer                                                           */
/* -------------------------------------------------------------------------- */

// ParseDefconfig reads a `key=value` file, which is the format every dialect
// reads, so a project's values need no syntax of their own.
//
// A quoted value is unwrapped, and a bare line is read as a symbol that is on:
// `CONFIG_SPI_MASTER` and `CONFIG_SPI_MASTER=y` mean the same thing, which is
// how defconfig files are written.
func ParseDefconfig(text, name string) (Source, []Diagnostic) {
	source := Source{Name: name, Values: map[string]string{}}
	diagnostics := []Diagnostic{}
	for number, raw := range strings.Split(text, "\n") {
		line := number + 1
		trimmed := strings.TrimSpace(stripComment(raw))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, found := strings.Cut(trimmed, "=")
		key = strings.TrimSpace(key)
		if !found {
			// A bare symbol is a bool that is on.
			source.Values[key] = "y"
			continue
		}
		if key == "" {
			diagnostics = append(diagnostics, problem(name, line, "a value is set with no symbol name"))
			continue
		}
		source.Values[key] = strings.TrimSpace(value)
	}
	return source, diagnostics
}
