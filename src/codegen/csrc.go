//
// @file csrc.go
// @brief The C runtime, as C source in the repository rather than as Go strings.
//
// The static half of the code generator. Everything here is a real `.c` or `.h`
// file, embedded at build time, so that the C a Rho program is compiled against
// can be read, linted, formatted and navigated as C — by an editor, by
// clang-format, by a static analyser — instead of being assembled from
// `WriteString` calls where the escaping is where the mistakes were.
//
// The dynamic half is `Emitter`: the program itself, emitted programmatically from
// the IR. Neither approach is sufficient alone. A template is the right tool for
// a preamble and the wrong one for an expression tree, and a string builder is the
// reverse; this file is the former and `emitter.go` is the latter.
//
// Each runtime fragment is separate so that only the ones a program actually calls
// are emitted. Not because `--gc-sections` would leave the flash behind — it would
// not — but because a generated file full of functions a program does not call is
// a generated file that produces a `-Wunused-function` diagnostic on every build and
// makes the real code harder to find in the output.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	_ "embed"
	"strings"
	"text/template"
)

// The preamble every generated file starts with. `{{.Module}}` and
// `{{.Features}}` are the only substitutions; the rest is C.
//
//go:embed csrc/preamble.h
var preambleSource string

//go:embed csrc/init.c
var initSource string

//go:embed csrc/print.c
var printSource string

//go:embed csrc/strings.c
var stringsSource string

//go:embed csrc/panic.c
var panicSource string

//go:embed csrc/env.c
var envSource string

//go:embed csrc/yaml.c
var yamlSource string

//go:embed csrc/bits.c
var bitsSource string


// RuntimeFragment is one piece of the runtime, and the symbols that pull it in.
//
// `provides` is what a program has to name for this fragment to be worth
// emitting, and `needs` is what this fragment in turn needs. The two are separate
// because `panic` needs `print` and nothing needs `panic` on its own account: a
// program that panics prints on the way out, and a program that prints does not
// necessarily have a panic path.
type RuntimeFragment struct {
	// Name is what the fragment is called, for diagnostics.
	Name string
	// Source is the C, verbatim.
	Source string
	// Provides are the symbols a program may call.
	Provides []string
	// Needs are symbols this fragment calls and therefore that must be emitted
	// with it.
	Needs []string
	// Always means the fragment is emitted whatever the program calls.
	//
	// For the two hooks a bare-metal link cannot do without: nothing in a
	// generated file refers to them, because what refers to them is the C
	// library's initializer in another archive, so gating on the program's own
	// calls would leave them out of the program that needs them most.
	Always bool
	// Decls are the C prototypes this fragment contributes to the program that
	// calls it.
	//
	// Written out rather than generated from `Provides`, because a prototype is
	// C: the const-ness and the pointer depth are part of the declaration, and a
	// generator that guessed them would produce a program that compiles with a
	// warning and misbehaves.
	Decls []string
}

// runtimeFragments is the runtime, in emission order.
//
// Ordered by what depends on what rather than alphabetically, because a C
// translation unit has to see a declaration before a use, and a fragment that
// calls another must come after the declaration of it. The preamble declares
// `rho_print`; everything else here defines.
var runtimeFragments = []RuntimeFragment{
	{
		Name:   "init",
		Source: initSource,
		Always: true,
	},
	{
		Name:     "print",
		Source:   printSource,
		Provides: []string{"rho_print", "rho_print_i32", "rho_print_u32"},
		Decls: []string{
			"void rho_print(const char* s);",
			"void rho_print_i32(int32_t value);",
			"void rho_print_u32(uint32_t value);",
		},
	},
	{
		Name:     "panic",
		Source:   panicSource,
		Provides: []string{"panicf"},
		Needs:    []string{"rho_print"},
		Decls:    []string{"static void panicf(const char* msg);"},
	},
	{
		Name:   "strings",
		Source: stringsSource,
		Provides: []string{
			"rho_concat", "rho_string_from_i32", "rho_str_from_byte",
			"rho_int_from_str", "rho_int_to_str", "rho_strlen", "rho_str_eq", "rho_str_starts_with",
			"rho_str_trim_prefix", "rho_str_contains", "rho_str_at",
			"rho_str_index_of", "rho_str_slice",
		},
		Decls: []string{
			"char* rho_concat(const char* a, const char* b);",
			"char* rho_string_from_i32(int32_t value);",
			"char* rho_str_from_byte(uint8_t b);",
			"uint32_t rho_int_from_str(const char* s);",
			"const char* rho_int_to_str(uint32_t value);",
			"uint32_t rho_strlen(const char* s);",
			"bool rho_str_eq(const char* a, const char* b);",
			"bool rho_str_starts_with(const char* s, const char* prefix);",
			"const char* rho_str_trim_prefix(const char* s, const char* prefix);",
			"bool rho_str_contains(const char* haystack, const char* needle);",
			"uint8_t rho_str_at(const char* s, uint32_t i);",
			"int32_t rho_str_index_of(const char* haystack, const char* needle);",
			"char* rho_str_slice(const char* s, uint32_t start, uint32_t length);",
		},
	},
	{
		Name:     "bits",
		Source:   bitsSource,
		Provides: []string{"rho_f32_to_bits", "rho_f32_from_bits", "rho_f64_to_bits", "rho_f64_from_bits"},
		Decls: []string{
			"uint32_t rho_f32_to_bits(float value);",
			"float rho_f32_from_bits(uint32_t bits);",
			"uint64_t rho_f64_to_bits(double value);",
			"double rho_f64_from_bits(uint64_t bits);",
		},
	},
	{
		Name:     "env",
		Source:   envSource,
		Provides: []string{"rho_env_or"},
		Decls:    []string{"static const char* rho_env_or(const char* name, const char* fallback);"},
	},
	{
		Name:     "yaml",
		Source:   yamlSource,
		Provides: []string{"rho_yaml_load"},
		Needs:    []string{"rho_strlen"},
		Decls:    []string{"static char* rho_yaml_load(const char* path);"},
	},
}

// runtimeDecls are the prototypes for the runtime a program calls.
//
// Emitted into the program rather than left to the runtime file, because C reads a
// translation unit top to bottom: a program that calls `rho_yaml_load` before the
// file that defines it is visible gets an implicit declaration, which is a
// different function with a different return type, and the result is a program
// that compiles with a warning and hands back an integer where a pointer was
// expected.
func runtimeDecls(emitted string) string {
	called := calledRuntimeSymbols(emitted)
	out := &strings.Builder{}
	for _, fragment := range runtimeFragments {
		if !emits(fragment, called) {
			continue
		}
		for _, decl := range fragment.Decls {
			out.WriteString(decl)
			out.WriteString("\n")
		}
	}
	return out.String()
}

// calledRuntimeSymbols is every runtime symbol a program's own text names.
func calledRuntimeSymbols(emitted string) map[string]bool {
	called := map[string]bool{}
	for _, fragment := range runtimeFragments {
		for _, symbol := range fragment.Provides {
			if callsSymbol(emitted, symbol) {
				called[symbol] = true
			}
		}
	}
	// A fixed point rather than one pass: the fragments depend on one another in a
	// chain, and a chain walked once is a chain half-walked.
	for changed := true; changed; {
		changed = false
		for _, fragment := range runtimeFragments {
			if !emits(fragment, called) {
				continue
			}
			for _, need := range fragment.Needs {
				if !called[need] {
					called[need] = true
					changed = true
				}
			}
		}
	}
	return called
}

// RuntimeFragmentNames is every runtime fragment, for `--list-runtime` and for a
// diagnostic that says what could have been emitted.
func RuntimeFragmentNames() []string {
	names := make([]string, 0, len(runtimeFragments))
	for _, fragment := range runtimeFragments {
		names = append(names, fragment.Name)
	}
	return names
}

// runtimeFor is the runtime a program needs, given everything it emitted.
//
// Detected from the emitted text rather than from a feature list, because the
// feature list is what a manifest declares and the answer to "does this program
// call `rho_str_slice`" is a fact about the program. A program whose features
// declaration is incomplete — or a library that lowers to a runtime call the
// manifest never mentioned — still gets what it uses, and a program that declares a
// feature and does not use it does not carry the code.
//
// A name is matched as a whole word followed by `(`, so that a program's own
// function called `rho_str_something_else` is not mistaken for the runtime's.
func runtimeFor(emitted string) string {
	// No early exit on "the program calls nothing": the fragments marked `Always`
	// are needed by a link that has not started yet, so an empty call set still
	// produces a runtime.
	called := calledRuntimeSymbols(emitted)
	out := &strings.Builder{}
	for _, fragment := range runtimeFragments {
		if !fragment.Always && !emits(fragment, called) {
			continue
		}
		out.WriteString(fragment.banner())
		out.WriteString(strings.TrimSpace(fragment.Source))
		out.WriteString("\n\n")
	}
	return out.String()
}

// emits is whether a fragment has anything of its own worth emitting: a symbol of
// its own that is called, or one that another emitted fragment needs.
func emits(fragment RuntimeFragment, called map[string]bool) bool {
	for _, symbol := range fragment.Provides {
		if called[symbol] {
			return true
		}
	}
	return false
}

// callsSymbol is whether `emitted` contains a call to a named symbol.
func callsSymbol(emitted, symbol string) bool {
	return strings.Contains(emitted, symbol+"(")
}

// banner is the comment that says where a fragment came from, so that a reader
// looking at generated output can tell the runtime from the program.
func (f RuntimeFragment) banner() string {
	return "/* --- rho runtime: " + f.Name + " --- */\n"
}

// preambleData is what the preamble template is rendered with.
type preambleData struct {
	Module   string
	Features []string
}

// preambleTemplate is the preamble, parsed once.
//
// text/template rather than string replacement because the file has two
// substitutions and a loop, and a loop is not something `ReplaceAll` can express
// without the caller reimplementing it — which is how a header ends up with a
// literal `{{range}}` in a firmware.
var preambleTemplate = template.Must(template.New("preamble").Parse(preambleSource))

// renderPreamble is the static header, with the module name and its features.
func renderPreamble(module string, features []string) string {
	out := &strings.Builder{}
	err := preambleTemplate.Execute(out, preambleData{Module: module, Features: features})
	if err != nil {
		// A template this package compiled cannot fail to parse, and the only way
		// Execute fails on parsed text is a broken function map, of which there is
		// none. Panicking rather than emitting a half-rendered header: a build that
		// stops here says why, and a build that proceeds with a broken preamble
		// produces an error naming a header line.
		panic("rho codegen: preamble template: " + err.Error())
	}
	return out.String()
}
