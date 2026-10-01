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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/plugins"
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
	// SourceDir is where the Rho interface is written, relative to the root. It
	// is under the project's build directory: it is generated, so it does not
	// belong among the sources somebody wrote.
	SourceDir string
	// ShimDir is where the C shim is written, relative to the root. It is the
	// generated C project, because the shim is part of what that project
	// compiles — a C file sitting in a Rho project's `src/` is neither a source
	// of that project nor part of anything that builds.
	ShimDir string
	// Module is the Rho module the interface is generated as.
	Module string
	// Force writes over a file that is already there.
	Force bool
	// Check reports what would be written without writing it.
	Check bool
	// Plugins are the extensions this run hands the declarations to. A run with
	// none maps the types it knows, which is most runs.
	Plugins []plugins.Plugin
}

// ComponentName is this component's own name, which is what a plugin has to say
// in order to be loaded by it.
const ComponentName = "rho-cimport"

// HookMap is the hook a plugin answers to add a type mapping: it runs once, with
// the declarations that were read, and answers with the types it knows.
const HookMap = "map"

// MapRequest is what a plugin is asked about the declarations a run read.
type MapRequest struct {
	// Headers are the files that were read.
	Headers []string `json:"headers"`
	// Declarations are what they declared, which is what a plugin needs to see
	// before it can say what a type means.
	Declarations []MapDeclaration `json:"declarations"`
}

// MapDeclaration is one declaration as a plugin sees it.
type MapDeclaration struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Type   string `json:"type,omitempty"`
	Result string `json:"result,omitempty"`
}

// MapResponse is what a plugin answers the map hook with: C types and what they
// are in Rho.
type MapResponse struct {
	Types map[string]string `json:"types"`
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
//
// Under `build/`, and not beside the program's own sources. A generated file in
// `src/` is a file the author did not write sitting among the files they did,
// which is a thing to keep out of a version control diff, a review, and an
// editor's go-to-definition. It is worse for the C than for the Rho: `src/` is
// where a project keeps its sources, so a shim generated there looks like one
// somebody meant to maintain, and deleting it breaks the build in a way that
// looks like an accident.
//
// Everything a build generates belongs under the build directory, where a
// project knows it is disposable: `rho build` deletes it and writes it again, and
// nothing in it is ever edited by hand.
const DefaultSourceDir = "build/generated/modules"

// DefaultShimDir is where the generated C is written.
//
// The generated C project, because that is what compiles it. The project layout
// is the descriptor's, so this is the same directory `rho build` writes
// `app.c` and `CMakeLists.txt` into — the shim is one more file in a project
// that already exists rather than a second project with a file in it.
const DefaultShimDir = "build/generated/c"

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
	if options.ShimDir == "" {
		options.ShimDir = DefaultShimDir
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
	mapper.Provided = runMap(options.Plugins, header)
	binding := &Interface{Module: options.Module, Header: header, Mapper: mapper}
	binding.Shims = collectShims(header)
	result.Shims = len(binding.Shims)
	result.Diagnostics = append(result.Diagnostics, mapper.Diagnostics(options.Library)...)

	files := map[string]string{
		options.SourceDir + "/" + options.Module + ".rho":  binding.Rho(),
		options.ShimDir + "/" + options.Module + "_shim.h": binding.ShimHeader(options.Headers),
		options.ShimDir + "/" + options.Module + "_shim.c": binding.ShimSource(),
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

// runMap is the hook a plugin adds a type mapping through.
//
// The mapping is what a plugin knows about a library the generator has never
// seen: a vendor type the header names and the language has no primitive for.
// A plugin that fails is reported and its mappings are left out.
func runMap(found []plugins.Plugin, header Header) map[string]string {
	provided := map[string]string{}
	if len(found) == 0 {
		return provided
	}
	payload := MapRequest{Headers: header.Includes}
	for _, decl := range header.Decls {
		payload.Declarations = append(payload.Declarations, MapDeclaration{
			Kind: string(decl.Kind), Name: decl.Name, Type: decl.Type, Result: decl.Result,
		})
	}

	answer := plugins.RunHooks(found, ComponentName, HookMap, payload)
	for _, raw := range answer.Payload {
		response := MapResponse{}
		if err := json.Unmarshal(raw, &response); err != nil {
			continue
		}
		for cType, rhoType := range response.Types {
			provided[cType] = rhoType
		}
	}
	return provided
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

// RecordPresetSources writes the sources the project's selected preset declares,
// and the directories a build needs in order to compile them.
//
// A preset says which vendor sources belong to a build: for a board, the startup
// assembly for that part and the system file that goes with it; for a family, the
// family's own. Those files are what make the image runnable — the startup is
// the entry point — and nothing else in a build has read the preset, so nothing
// else can say what to compile.
//
// They are recorded rather than reported because a build reads them after this
// command has finished. A build that asked again would be reading the preset
// twice and could disagree with itself about which board it was building.
//
// The include directories are recorded beside them, for the same reason: a source
// compiled without the headers it includes cannot be built, and both come from
// the same declaration.
func RecordPresetSources(root, destination, presetName string) error {
	// The preset the build selected, not the one the manifest offers as its
	// default. A project declares every preset it can be built for and a build
	// picks one; reading the default would record the sources of a board this
	// build is not for, and the failure would be a link step naming a startup
	// symbol nobody defined.
	resolver := &PresetResolver{ProjectRoot: root}
	var preset Preset
	// The platform is the module that provides the preset, and a board belongs to
	// the platform rather than to the project: a project depends on a platform and
	// does not carry one inside itself.
	platformRoot := root
	if presetName != "" {
		found, err := resolver.LookupPreset(presetName)
		if err != nil {
			return err
		}
		preset = found
		if declared, rootErr := resolver.RootOf(presetName); rootErr == nil {
			platformRoot = declared
		}
	} else {
		read, err := ReadPreset(root)
		if err != nil {
			return err
		}
		preset = read
	}
	if preset.Name == "" {
		return fmt.Errorf("this project declares no preset, so there are no sources to record; name one with -preset, or add a presets section to the manifest")
	}

	cache, err := CacheDir(platformRoot)
	if err != nil {
		return err
	}

	sources := &Resolver{ProjectRoot: root, Cache: cache, BoardName: preset.Board}
	// Fetching is not this step's decision. A build that has already fetched what
	// it needs must not depend on whether a generation ran first, so a dependency
	// that is not there is reported rather than fetched.
	sources.Out = nil

	resolution, err := sources.Resolve(preset)
	if err != nil {
		return err
	}

	var recorded strings.Builder
	for _, source := range resolution.SourceFiles {
		recorded.WriteString(source)
		recorded.WriteString("\n")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(destination, []byte(recorded.String()), 0o644); err != nil {
		return err
	}

	var includes strings.Builder
	for _, dir := range resolution.IncludeDirs {
		includes.WriteString(dir)
		includes.WriteString("\n")
	}
	// A board's own support package, found by the name its preset carries and
	// reading the directories that board declares.
	//
	// The board is the only place a board-specific support package is known: the
	// platform implements every board of a family at once and cannot depend on
	// all of their packages, and the family preset says nothing about which board
	// is on it. So the preset names the board, and the board's own manifest says
	// where its headers are.
	var boardSources []string
	for _, dir := range boardDirectories(platformRoot, preset) {
		includes.WriteString(dir)
		includes.WriteString("\n")
	}
	// The board's own sources, which are its part of the image.
	//
	// A board is a library like any other and its manifest says what it is made
	// of. Nothing compiled them: the platform's library does not contain them and
	// the program does not, so the board's contribution to the build was missing
	// entirely — and the symbol that reports it missing is the one that registers
	// the board, which is the first thing a program calls.
	if presetName != "" || preset.Board != "" {
		boardSources = boardSourcesOf(platformRoot, preset)
	}
	includesFile := strings.TrimSuffix(destination, ".txt") + "-includes.txt"
	if err := os.WriteFile(includesFile, []byte(includes.String()), 0o644); err != nil {
		return err
	}
	if len(boardSources) == 0 {
		return nil
	}
	sourcesFile := strings.TrimSuffix(destination, ".txt") + "-board-sources.txt"
	return os.WriteFile(sourcesFile, []byte(strings.Join(boardSources, "\n")+"\n"), 0o644)
}

// boardSourcesOf returns the files the board a preset names declares, resolved
// against that board's own root.
//
// The board declares them by name rather than as a directory: a directory says
// "everything in here", and what a board contributes is a fixed small set of
// files that a project has no business extending.
func boardSourcesOf(root string, preset Preset) []string {
	name := strings.TrimSpace(preset.Board)
	if name == "" {
		return nil
	}
	for _, candidate := range []string{name, strings.ToLower(name)} {
		boardRoot := filepath.Join(root, "boards", candidate)
		manifest, err := deps.LoadManifest(deps.ManifestPath(boardRoot))
		if err != nil || manifest == nil {
			continue
		}
		files := []string{}
		for _, declared := range manifest.BuildSources {
			path := declared
			if !filepath.IsAbs(path) {
				path = filepath.Join(boardRoot, path)
			}
			if _, err := os.Stat(path); err != nil {
				continue
			}
			files = append(files, path)
		}
		return files
	}
	return nil
}

// boardDirectories returns what the board a preset names declares its build
// needs.
//
// The board is located by the name the preset carries, which is what a build was
// asked for. A preset that names no board — an architecture preset, built for a
// family rather than a board — contributes nothing, because there is no board to
// read a contract from.
func boardDirectories(root string, preset Preset) []string {
	name := strings.TrimSpace(preset.Board)
	if name == "" {
		return nil
	}

	// The name a board carries in a preset is its designation, which is written in
	// the case the datasheet uses; the module is named in lower case. Matching on
	// that is what lets the two say the same thing in the way each of them already
	// does, rather than requiring the preset to be written a second way.
	for _, candidate := range []string{name, strings.ToLower(name)} {
		boardRoot := filepath.Join(root, "boards", candidate)
		manifest, err := deps.LoadManifest(deps.ManifestPath(boardRoot))
		if err != nil || manifest == nil {
			continue
		}
		dirs := []string{}
		for _, dir := range manifest.BuildIncludeDirs {
			if filepath.IsAbs(dir) {
				dirs = append(dirs, dir)
				continue
			}
			dirs = append(dirs, filepath.Join(boardRoot, dir))
		}
		return dirs
	}
	return nil
}
