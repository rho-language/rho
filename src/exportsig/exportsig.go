//
// @file exportsig.go
// @brief What a module's exports are, version by version.
//
// A module's exports are a promise to the modules that import it: a function
// with these parameters, a struct with these fields, a constant of this type. A
// release that removes one or changes it breaks that promise, which is what a
// major version says. This package reads the promise from a module's sources,
// compares two versions of it, finds the release a working tree is compared
// with, and finds the modules that would feel a change.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package exportsig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
)

// FileName is the record of a published version's exports, kept beside the
// version in the local registry.
const FileName = "exports.json"

// Entry is one exported name and what it promises.
type Entry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	// Signature is what is compared: the types a caller depends on, without the
	// names it does not (a parameter's name, a constant's value).
	Signature string `json:"signature"`
	// Display is the declaration as a reader writes it.
	Display string `json:"display"`
}

// Set is a module's exports at one version.
type Set struct {
	Module  string           `json:"module"`
	Version string           `json:"version"`
	Import  string           `json:"import,omitempty"`
	Entries map[string]Entry `json:"exports"`
}

// Read reads the exports of the module whose manifest is in root.
//
// The exports are the names `provides.exports` lists, or, for a module without
// a list, every function, type and constant it declares. An interface's
// functions are part of the promise too: its providers implement them.
func Read(root string) (Set, error) {
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil {
		return Set{}, err
	}
	set := Set{Module: manifest.ModuleName, Version: manifest.Version, Entries: map[string]Entry{}}
	var listed []string
	if manifest.Provides != nil {
		set.Import = manifest.Provides.Import
		listed = manifest.Provides.Exports
	}
	declared := map[string]Entry{}
	// A component's main is the program it builds, not a Rho source.
	if strings.HasSuffix(manifest.Main, ".rho") {
		visited := map[string]bool{}
		if err := readDecls(filepath.Join(root, manifest.Main), root, visited, declared); err != nil {
			return Set{}, err
		}
	}
	if listed == nil {
		for name, entry := range declared {
			set.Entries[name] = entry
		}
	} else {
		for _, name := range listed {
			entry, found := declared[name]
			if !found {
				entry = Entry{Name: name, Kind: "missing", Signature: "missing", Display: name + " (declared nowhere)"}
			}
			set.Entries[name] = entry
		}
	}
	if manifest.Interface != nil {
		for _, function := range manifest.Interface.Functions {
			name := "interface " + function.Name
			signature := normalise(function.Signature)
			set.Entries[name] = Entry{Name: name, Kind: "interface", Signature: signature, Display: "interface fn " + function.Name + function.Signature}
		}
	}
	return set, nil
}

// readDecls reads one source file's declarations, and those of the files of the
// same module it includes. An include of another module is not followed: what
// that module declares is its promise, not this one's.
func readDecls(path, root string, visited map[string]bool, out map[string]Entry) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if visited[abs] {
		return nil
	}
	visited[abs] = true
	source, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	tokens, err := lexer.Lex(string(source))
	if err != nil {
		return fmt.Errorf("%s: %w", abs, err)
	}
	module, err := parser.Parse(tokens)
	if err != nil {
		return fmt.Errorf("%s: %w", abs, err)
	}
	for _, decl := range module.Decls {
		if entry, ok := entryFor(decl); ok {
			out[entry.Name] = entry
		}
	}
	for _, include := range module.Includes {
		if !strings.ContainsAny(include.Path, "/\\") && !strings.HasSuffix(include.Path, ".rho") {
			continue
		}
		next := filepath.Join(filepath.Dir(abs), include.Path)
		if rel, err := filepath.Rel(root, next); err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if err := readDecls(next, root, visited, out); err != nil {
			return err
		}
	}
	return nil
}

// entryFor is what one declaration promises.
func entryFor(decl ast.Decl) (Entry, bool) {
	switch declaration := decl.(type) {
	case ast.FuncDecl:
		if declaration.Extern {
			return Entry{}, false
		}
		types := make([]string, len(declaration.Params))
		named := make([]string, len(declaration.Params))
		for i, param := range declaration.Params {
			types[i] = normalise(param.Type)
			named[i] = param.Name + ": " + param.Type
		}
		generic := ""
		if len(declaration.TypeParams) > 0 {
			generic = "[" + strings.Join(declaration.TypeParams, ", ") + "]"
		}
		returns := normalise(declaration.ReturnType)
		return Entry{
			Name:      declaration.Name,
			Kind:      "fn",
			Signature: "fn" + generic + "(" + strings.Join(types, ", ") + ") " + returns,
			Display:   "fn " + declaration.Name + generic + "(" + strings.Join(named, ", ") + ") " + declaration.ReturnType,
		}, true
	case ast.StructDecl:
		// A struct literal is positional, so the order of the fields is part of
		// the promise as much as their types.
		fields := make([]string, len(declaration.Fields))
		for i, field := range declaration.Fields {
			fields[i] = field.Name + ": " + normalise(field.Type)
		}
		generic := ""
		if len(declaration.TypeParams) > 0 {
			generic = "[" + strings.Join(declaration.TypeParams, ", ") + "]"
		}
		body := "{ " + strings.Join(fields, ", ") + " }"
		return Entry{Name: declaration.Name, Kind: "struct", Signature: "struct" + generic + " " + body, Display: "struct " + declaration.Name + generic + " " + body}, true
	case ast.EnumDecl:
		variants := make([]string, len(declaration.Variants))
		for i, variant := range declaration.Variants {
			variants[i] = fmt.Sprintf("%s = %d", variant.Name, variant.Value)
		}
		body := "{ " + strings.Join(variants, ", ") + " }"
		return Entry{Name: declaration.Name, Kind: "enum", Signature: "enum " + body, Display: "enum " + declaration.Name + " " + body}, true
	case ast.TypeDecl:
		target := normalise(declaration.Type.String())
		return Entry{Name: declaration.Name, Kind: "type", Signature: "type = " + target, Display: "type " + declaration.Name + " = " + target}, true
	case ast.ConstDecl:
		typ := normalise(declaration.Type.String())
		return Entry{Name: declaration.Name, Kind: "const", Signature: "const " + typ, Display: "const " + declaration.Name + ": " + typ}, true
	}
	return Entry{}, false
}

// normalise is a written type or signature without the spacing that does not
// change what it says.
func normalise(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

/* -------------------------------------------------------------------------- */
/*  Comparing two versions                                                     */
/* -------------------------------------------------------------------------- */

// Change is one export that is not what it was.
type Change struct {
	Name   string
	Before Entry
	After  Entry
}

// Diff is what changed between two versions of a module's exports.
type Diff struct {
	Removed []Entry
	Changed []Change
	Added   []Entry
}

// Breaking reports whether a module that used the older version can stop
// building against the newer one.
func (d Diff) Breaking() bool {
	return len(d.Removed) > 0 || len(d.Changed) > 0
}

// Names are the exports a breaking change touches.
func (d Diff) Names() []string {
	names := []string{}
	for _, entry := range d.Removed {
		names = append(names, entry.Name)
	}
	for _, change := range d.Changed {
		names = append(names, change.Name)
	}
	sort.Strings(names)
	return names
}

// Compare is what changed from before to after.
func Compare(before, after Set) Diff {
	diff := Diff{}
	for name, old := range before.Entries {
		current, kept := after.Entries[name]
		switch {
		// Still listed under exports and declared nowhere is gone all the same.
		case !kept || current.Kind == "missing":
			diff.Removed = append(diff.Removed, old)
		case current.Signature != old.Signature:
			diff.Changed = append(diff.Changed, Change{Name: name, Before: old, After: current})
		}
	}
	for name, current := range after.Entries {
		if _, existed := before.Entries[name]; !existed {
			diff.Added = append(diff.Added, current)
		}
	}
	sort.Slice(diff.Removed, func(a, b int) bool { return diff.Removed[a].Name < diff.Removed[b].Name })
	sort.Slice(diff.Changed, func(a, b int) bool { return diff.Changed[a].Name < diff.Changed[b].Name })
	sort.Slice(diff.Added, func(a, b int) bool { return diff.Added[a].Name < diff.Added[b].Name })
	return diff
}

// CheckVersion says whether a version may carry a diff from the version before
// it. A breaking change needs a new major version — or, before 1.0.0, a new
// minor one, which is what a 0.x version promises instead.
func CheckVersion(before, after string, diff Diff) error {
	if !diff.Breaking() {
		return nil
	}
	old, err := deps.ParseSemver(before)
	if err != nil {
		return nil
	}
	current, err := deps.ParseSemver(after)
	if err != nil {
		return nil
	}
	if old.Major == 0 && current.Major == 0 {
		if current.Minor > old.Minor {
			return nil
		}
		return fmt.Errorf("%s changes exports that %s promised (%s), which needs at least %d.%d.0", after, before, strings.Join(diff.Names(), ", "), 0, old.Minor+1)
	}
	if current.Major > old.Major {
		return nil
	}
	return fmt.Errorf("%s changes exports that %s promised (%s), which needs %d.0.0", after, before, strings.Join(diff.Names(), ", "), old.Major+1)
}

/* -------------------------------------------------------------------------- */
/*  Records                                                                    */
/* -------------------------------------------------------------------------- */

// Write records a set in dir.
func Write(dir string, set Set) error {
	data, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, FileName), append(data, '\n'), 0o644)
}

// Load reads the set recorded in dir.
func Load(dir string) (Set, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return Set{}, err
	}
	set := Set{}
	if err := json.Unmarshal(data, &set); err != nil {
		return Set{}, fmt.Errorf("%s: %w", filepath.Join(dir, FileName), err)
	}
	return set, nil
}
