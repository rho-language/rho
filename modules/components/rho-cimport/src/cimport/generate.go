//
// @file generate.go
// @brief Generating a project's foreign interface.
//
// This is what a caller runs: read what the project's manifest says it links,
// read the headers it named, work out which calls need a C shim, and write the
// three files. Everything it writes is a generated file, and everything it could
// not read is reported rather than left out in silence.
//
// The three files are the whole of a foreign interface:
//
//	src/ffi.rho       what the headers declare, in Rho's types
//	src/ffi_shim.h     the calls Rho cannot make, declared
//	src/ffi_shim.c     the calls Rho cannot make, made
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cimport

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
)

// Options are what a generation is asked for.
type Options struct {
	// Root is the project the interface is generated into.
	Root string
	// Headers are the C headers to read, as the manifest names them.
	Headers []string
	// IncludeDirs are where a header is found, as the manifest names them.
	IncludeDirs []string
	// Library is the C library the project links, which the shim header
	// includes.
	Library string
	// SourceDir is where the interface is written, relative to the root.
	SourceDir string
	// Module is the Rho module the interface is generated as.
	Module string
	// Force writes over a file that is already there.
	Force bool
	// Check reports what would be written without writing it.
	Check bool
}

// Result is what a generation produced.
type Result struct {
	// Files are the files that were written, relative to the root.
	Files []string
	// Declarations is how many declarations the headers made.
	Declarations int
	// Shims is how many calls Rho cannot make and the shim therefore makes.
	Shims int
	// Diagnostics is everything that could not be read or could not be mapped.
	Diagnostics []Diagnostic
}

// DefaultModule is the Rho module a generated interface is written as.
const DefaultModule = "ffi"

// DefaultSourceDir is where a generated interface is written.
const DefaultSourceDir = "src"

// Generate reads the headers a project declared and writes its foreign
// interface, the C shim that makes the calls Rho cannot, and nothing else.
func Generate(options Options) (Result, error) {
	result := Result{}
	if options.Module == "" {
		options.Module = DefaultModule
	}
	if options.SourceDir == "" {
		options.SourceDir = DefaultSourceDir
	}

	// The declared headers are read first, then the ones they include: a type
	// used by a header the project named is a type the binding needs.
	header := Header{}
	followed := map[string]bool{}
	for _, name := range options.Headers {
		resolved, found := findInclude(name, options.Root, options.includeSearch())
		if !found {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Header:  name,
				Message: "the manifest names this header and it was not found under the declared include paths",
			})
			continue
		}
		header = mergeHeader(header, ReadHeader(resolved, options.includeSearch(), followed, 0))
	}
	result.Diagnostics = append(result.Diagnostics, header.Diagnostics...)
	if len(header.Decls) == 0 {
		return result, fmt.Errorf("no declaration was read; check the headers the manifest names")
	}
	result.Declarations = len(header.Decls)

	mapper := NewTypeMapper(header)
	binding := &Interface{Module: options.Module, Header: header, Mapper: mapper}
	binding.Shims = collectShims(header)
	result.Shims = len(binding.Shims)
	result.Diagnostics = append(result.Diagnostics, mapper.Diagnostics(options.Library)...)

	files := map[string]string{
		options.SourceDir + "/" + options.Module + ".rho":    binding.Rho(),
		options.SourceDir + "/" + options.Module + "_shim.h": binding.ShimHeader(options.Headers),
		options.SourceDir + "/" + options.Module + "_shim.c": binding.ShimSource(),
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	if options.Check {
		result.Files = names
		return result, nil
	}
	for _, name := range names {
		if !options.Force && exists(filepath.Join(options.Root, filepath.FromSlash(name))) {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Header:  name,
				Message: "a generated file is already there and was kept; --force writes over it",
			})
			continue
		}
		if _, err := WriteFile(options.Root, name, files[name]); err != nil {
			return result, err
		}
		result.Files = append(result.Files, name)
	}
	return result, nil
}

// includeSearch is where a header is looked for: the project itself, then the
// paths the manifest names.
//
// A manifest names a directory relative to the project, which is what a person
// reads when they open it, so a relative path is resolved against the root before
// anything is looked for: a compiler is told absolute paths for the same reason.
func (o Options) includeSearch() []string {
	dirs := []string{o.Root}
	for _, dir := range o.IncludeDirs {
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(o.Root, filepath.FromSlash(dir))
		}
		dirs = append(dirs, dir)
	}
	return dirs
}

// collectShims works out which declarations are calls Rho cannot make.
//
// A C function pointer reached through a structure member is the whole of it:
// Rho has no type for a function pointer, so a HAL that is a table of them can
// be looked at from a binding and not called. Each such member gets a shim,
// named for what it reaches, so a caller writes the call it would have written
// if the pointer were a function.
func collectShims(header Header) []Shim {
	shims := []Shim{}
	for _, decl := range header.Decls {
		if decl.Kind != KindStruct && decl.Kind != KindUnion {
			continue
		}
		for _, field := range decl.Fields {
			if !field.Ptr {
				continue
			}
			pointer, found := findFuncPtr(header, decl.Name+"."+field.Name)
			if !found {
				continue
			}
			// The pointer's first parameter is the table itself, which the
			// shim has already been given; the rest are the call's own.
			params := []Param{}
			if len(pointer.Params) > 0 {
				params = append(params, pointer.Params[1:]...)
			}
			shims = append(shims, Shim{
				Name:   shimName(decl.Name, field.Name),
				Return: pointer.Result,
				Params: params,
				Member: field.Name,
				Owner:  decl.Name,
			})
		}
	}
	sort.SliceStable(shims, func(a, b int) bool { return shims[a].Name < shims[b].Name })
	return shims
}

// findFuncPtr is the function pointer a struct member declares. The reader
// records the member as a field, and the pointer's own signature is in the
// declarations when the header also typedefs it; where it does not, the member
// is read as a function pointer by the member reader itself.
func findFuncPtr(header Header, member string) (Decl, bool) {
	owner, name, found := strings.Cut(member, ".")
	if !found {
		return Decl{}, false
	}
	ownerDecl, ok := header.Find(owner)
	if !ok {
		return Decl{}, false
	}
	for _, field := range ownerDecl.Fields {
		if field.Name != name || !field.Ptr {
			continue
		}
		// The reader keeps what the member returned: its result, and the types
		// of its parameters, in a field of its own.
		return Decl{Result: field.Type, Params: field.Params}, true
	}
	return Decl{}, false
}

// shimName is the C name of a shim: the struct and the member it reaches, in
// the spelling a C caller would recognise.
func shimName(owner, member string) string {
	return "rho_" + strings.ToLower(owner) + "_" + strings.ToLower(member)
}

// ReadDeclared reads the headers a project's connection names, following the
// includes among them, and returns what they declare with what could not be
// read. A caller that wants to see the shape of a C interface before generating
// anything asks for it here.
func ReadDeclared(root string, connection deps.CConnection) (Header, []Diagnostic) {
	header := Header{}
	followed := map[string]bool{}
	// The same rule as a generation: a directory a manifest names is relative to
	// the project it is in.
	search := Options{Root: root, IncludeDirs: connection.IncludeDirs}.includeSearch()
	for _, name := range connection.Headers {
		resolved, found := findInclude(name, root, search)
		if !found {
			header.Diagnostics = append(header.Diagnostics, Diagnostic{
				Header:  name,
				Message: "the manifest names this header and it was not found under the declared include paths",
			})
			continue
		}
		header = mergeHeader(header, ReadHeader(resolved, search, followed, 0))
	}
	return header, header.Diagnostics
}
