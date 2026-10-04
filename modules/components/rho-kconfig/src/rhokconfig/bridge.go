//
// @file bridge.go
// @brief Resolving one project's configuration.
//
// This is the whole bridge: read the fragments a project declared, read the
// layers that set values on them, merge, and write the files the dialect's
// toolchain expects. A generated project's build calls it; a person calls it
// with `rho kconfig`.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhokconfig

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
	"rho/src/plugins"
)

// Options are what a merge is asked for.
type Options struct {
	// Root is the project the configuration is for.
	Root string
	// Fragments are the Kconfig files that define the symbols, in the order
	// they are read.
	Fragments []string
	// Defconfig is a `key=value` file of values a project commits.
	Defconfig string
	// Values is a map of values, which a build passes from a manifest.
	Values map[string]string
	// Module is a path to a Rho module of constants that set values, relative to
	// the root.
	Module string
	// Dialect is which headers are written.
	Dialect string
	// Overwrite writes over a file that is already there.
	Overwrite bool
	// Plugins are the extensions this run hands its resolved configuration to.
	// A run with none writes the dialects it has, which is most runs.
	Plugins []plugins.Plugin
}

// ComponentName is this component's own name, which is what a plugin has to say
// in order to be loaded by it.
const ComponentName = "rho-kconfig"

// HookRender is the hook a plugin answers to render one more dialect: it runs
// once, after the fragments are merged and before anything is written, and it
// answers with files.
const HookRender = "render"

// RenderRequest is what a plugin is asked about a resolved configuration.
type RenderRequest struct {
	// Dialect is the one the build asked for, so a plugin can tell whether the
	// project wants what it renders.
	Dialect string `json:"dialect"`
	// Values are the resolved symbols: what a symbol is, what it holds, and the
	// layer that set it.
	Values []RenderValue `json:"values"`
}

// RenderValue is one resolved symbol as a plugin sees it.
type RenderValue struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	From  string `json:"from"`
}

// RenderResponse is what a plugin answers the render hook with: files to write
// beside the dialect's own.
type RenderResponse struct {
	Files []RenderFile `json:"files"`
}

// RenderFile is one file a plugin contributes.
type RenderFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Result is what a merge produced.
type Result struct {
	// Configuration is the resolved configuration.
	Configuration Configuration
	// Written are the files that were written, relative to the root.
	Written []string
	// Kept are the files that were already there and were not written.
	Kept []string
	// Diagnostics is everything worth saying, in the order it was found. A
	// caller fails on the problems, and may print the notes.
	Diagnostics []Diagnostic
}

// Problems are the diagnostics that changed the answer, which is what a build
// fails on.
func (r Result) Problems() []Diagnostic {
	out := []Diagnostic{}
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Problem() {
			out = append(out, diagnostic)
		}
	}
	return out
}

// Merge resolves a project's configuration and writes the files its dialect
// produces.
//
// Everything it found wrong is a diagnostic rather than an error, except a
// configuration that cannot be read at all: a build that stops for a missing
// fragment is right, and a build that stops for an unset symbol would be wrong,
// because a symbol with a default is not unset.
func Merge(options Options) (Result, error) {
	result := Result{}

	fragment, diagnostics := ReadFragments(options.Root, options.Fragments)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if len(fragment.Symbols) == 0 {
		return result, fmt.Errorf("no Kconfig symbol was declared; check the fragments a project declared")
	}

	sources := []Source{}

	// The layer a project commits.
	if options.Defconfig != "" {
		data, err := os.ReadFile(resolve(options.Root, options.Defconfig))
		if err != nil {
			return result, fmt.Errorf("cannot read the defconfig: %w", err)
		}
		source, diagnostics := ParseDefconfig(string(data), "defconfig")
		source.Path = options.Defconfig
		sources = append(sources, source)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
	}

	// The layer a manifest sets.
	if len(options.Values) > 0 {
		sources = append(sources, Source{Name: "module.yml", Path: "module.yml", Values: options.Values})
	}

	// The layer a Rho module sets, read with the compiler's own front end: a
	// value written in Rho is one the compiler would accept.
	if options.Module != "" {
		source, diagnostics := readRhoModule(options.Root, options.Module)
		sources = append(sources, source)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
	}

	configuration, diagnostics := Resolve(fragment, sources)
	result.Configuration = configuration
	result.Diagnostics = append(result.Diagnostics, diagnostics...)

	dialect, err := dialectByName(options.Dialect)
	if err != nil {
		return result, err
	}
	files := dialect.Files(configuration)
	// A plugin renders one more dialect, and what it answers is written with the
	// dialect's own: the run has one place that writes files, so a plugin cannot
	// write somewhere a person who reads the manifest would not expect.
	pluginFiles, diagnostics := runRender(options.Plugins, dialect.Name(), configuration)
	files = append(files, pluginFiles...)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)

	// The list of files the merge was asked for is written too, so a build can
	// depend on a file that is always produced.
	written, writeDiagnostics := Write(options.Root, files, options.Overwrite)
	result.Written = append(result.Written, written...)
	result.Diagnostics = append(result.Diagnostics, writeDiagnostics...)
	for _, file := range files {
		if !contains(written, file.Path) {
			result.Kept = append(result.Kept, file.Path)
		}
	}
	return result, nil
}

// readRhoModule reads a module of constants as a layer of values.
//
// Rho has no top-level constant, so a module of configuration is a module that
// binds them, and the binding is a `let` wherever it appears:
//
//	module config
//
//	extern fn board() u32
//
//	fn init() {
//	    let CONFIG_UART_BAUD: u32 = 115200
//	    let CONFIG_SPI_MASTER: bool = true
//	    let CONFIG_LOG_LEVEL: string = "info"
//	}
//
// The file is read with the compiler's own parser, so it is held to the same
// syntax the compiler holds it to, and a value is a literal: a number, a string,
// or a flag written as a boolean. A binding that names a symbol is a
// configuration value; anything else is the program's own state and is left
// alone.
func readRhoModule(root, path string) (Source, []Diagnostic) {
	source := Source{Name: "config module", Path: path, Values: map[string]string{}}
	diagnostics := []Diagnostic{}

	resolved := resolve(root, path)
	data, err := os.ReadFile(resolved)
	if err != nil {
		diagnostics = append(diagnostics, problem(path, 0, "cannot read the configuration module: "+err.Error()))
		return source, diagnostics
	}
	tokens, err := lexer.Lex(string(data))
	if err != nil {
		diagnostics = append(diagnostics, problem(path, 0, "lexing: "+err.Error()))
		return source, diagnostics
	}
	module, err := parser.Parse(tokens)
	if err != nil {
		diagnostics = append(diagnostics, problem(path, 0, "parsing: "+err.Error()))
		return source, diagnostics
	}

	// A binding anywhere in the module is read, because a program that reads its
	// configuration wants the values where it uses them, and a config module
	// that keeps them all in one function is only one of the ways to write it.
	for _, declaration := range module.Decls {
		function, ok := declaration.(ast.FuncDecl)
		if !ok {
			continue
		}
		for _, statement := range function.Body {
			binding, ok := statement.(ast.LetStmt)
			if !ok || !strings.HasPrefix(binding.Name, "CONFIG_") {
				// A binding that does not name a symbol is the program's own
				// state, not a configuration value, so it is left alone rather
				// than reported: a config module may hold both.
				continue
			}
			literal, ok := literalOf(binding.Value)
			if !ok {
				diagnostics = append(diagnostics, problem(path, 0,
					fmt.Sprintf("%s is bound to something that is not a literal; a configuration value is a number, a string or a flag", binding.Name)))
				continue
			}
			source.Values[binding.Name] = literal
		}
	}
	return source, diagnostics
}

// literalOf reads a value the language can write as a literal, which is what a
// configuration value is: it has to survive being written into a header that
// something else will read.
func literalOf(expression ast.Expr) (string, bool) {
	switch value := expression.(type) {
	case ast.NumberExpr:
		return value.Value, true
	case ast.StringExpr:
		return value.Value, true
	case ast.BoolExpr:
		if value.Value {
			return "y", true
		}
		return "n", true
	}
	return "", false
}

// runRender is the hook a plugin renders one more dialect through.
//
// A plugin that fails is reported and its files are left out: the configuration
// the project declared is still written, which is the run a person asked for.
func runRender(found []plugins.Plugin, dialect string, configuration Configuration) ([]File, []Diagnostic) {
	files := []File{}
	diagnostics := []Diagnostic{}
	if len(found) == 0 {
		return files, diagnostics
	}
	payload := RenderRequest{Dialect: dialect}
	for _, name := range configuration.Names() {
		value, _ := configuration.Get(name)
		payload.Values = append(payload.Values, RenderValue{
			Name: name, Type: string(value.Type), Value: value.Value, From: value.From,
		})
	}

	answer := plugins.RunHooks(found, ComponentName, HookRender, payload)
	for _, diagnostic := range answer.Diagnostics {
		diagnostics = append(diagnostics, problem("", 0, diagnostic.String()))
	}
	for _, raw := range answer.Payload {
		response := RenderResponse{}
		if err := json.Unmarshal(raw, &response); err != nil {
			diagnostics = append(diagnostics, problem("", 0,
				fmt.Sprintf("a plugin answered the render hook with something that is not the protocol: %v", err)))
			continue
		}
		for _, file := range response.Files {
			if file.Path == "" {
				diagnostics = append(diagnostics, problem("", 0, "a plugin answered the render hook with a file that has no path"))
				continue
			}
			files = append(files, File{Path: file.Path, Content: file.Content})
		}
	}
	return files, diagnostics
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  Reading a project's declaration                                           */
/* -------------------------------------------------------------------------- */

// ProjectConfig is the bridge a project declared, read from its manifest.
type ProjectConfig struct {
	// Configured reports whether the project declared a bridge at all.
	Configured bool
	// Dialect is which headers it asked for.
	Dialect string
	// Fragments are the Kconfig files it declared, relative to the project.
	Fragments []string
	// Defconfig is the file of values it committed, when it has one.
	Defconfig string
	// Values are the values it set in its manifest.
	Values map[string]string
	// Module is a Rho module of constants it named.
	Module string
}

// ReadProjectConfig reads the Kconfig bridge a project declared.
//
// A project that declared none is a project that wants no configuration, and
// this says so rather than guessing: a build with no bridge is a build that
// configures nothing, and that is a decision rather than a failure.
func ReadProjectConfig(root string) (ProjectConfig, error) {
	config := ProjectConfig{}
	manifest, err := deps.LoadManifest(deps.ManifestPath(root))
	if err != nil {
		return config, err
	}
	if manifest == nil || manifest.Config == nil {
		return config, nil
	}
	declared := manifest.Config
	config.Configured = true
	config.Dialect = declared.Dialect
	config.Fragments = declared.Fragments
	config.Values = declared.Values
	config.Module = declared.Module

	// A defconfig is a file of values a project commits, so it is read when the
	// project has one. It is looked for beside the fragments a project declared,
	// which is where a generated project's own values live.
	if len(declared.Fragments) > 0 {
		config.Defconfig = filepath.Join(filepath.Dir(declared.Fragments[0]), "rho-defconfig")
		if _, err := os.Stat(filepath.Join(root, config.Defconfig)); err != nil {
			config.Defconfig = ""
		}
	}
	return config, nil
}

/* -------------------------------------------------------------------------- */
/*  Reporting                                                                 */
/* -------------------------------------------------------------------------- */

// List renders the configuration as a person reads it: every symbol, its value,
// and where the value came from, so a question about a setting is answered
// without reading the fragments.
func List(configuration Configuration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d symbol(s)\n", len(configuration.Values))
	for _, name := range configuration.Names() {
		value, _ := configuration.Get(name)
		if value.Value == "" {
			fmt.Fprintf(&b, "  %-44s = (unset)          from %s\n", name, value.From)
			continue
		}
		fmt.Fprintf(&b, "  %-44s = %-16s from %s\n", name, value.Value, value.From)
	}
	return b.String()
}

// Set changes one value in a defconfig file, keeping the file's order as far as
// it can and reporting a symbol nothing declares.
func Set(root, defconfig, name, value string, configuration Configuration) (string, []Diagnostic) {
	diagnostics := []Diagnostic{}
	if _, known := configuration.Get(name); !known {
		diagnostics = append(diagnostics, Diagnostic{
			Path: name, Message: "no Kconfig fragment declares this symbol, so setting it would write a value nothing reads"})
		return "", diagnostics
	}
	path := filepath.Join(root, filepath.FromSlash(defconfig))
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}
	values := map[string]string{}
	source, _ := ParseDefconfig(existing, "existing")
	values = source.Values
	values[name] = value

	names := make([]string, 0, len(values))
	for key := range values {
		names = append(names, key)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("# Set by rho kconfig. DO NOT EDIT BY HAND.\n")
	for _, key := range names {
		fmt.Fprintf(&b, "%s=%s\n", key, values[key])
	}
	return b.String(), diagnostics
}
