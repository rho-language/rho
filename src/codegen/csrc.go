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
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
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

// The string arena: where every string the runtime builds lives, and what the
// compiler releases at the end of a loop iteration or a statement.
//
//go:embed csrc/arena.c
var arenaSource string

//go:embed csrc/strings.c
var stringsSource string

//go:embed csrc/text.c
var textSource string

//go:embed csrc/lock.c
var lockSource string

//go:embed csrc/panic.c
var panicSource string

//go:embed csrc/env.c
var envSource string

//go:embed csrc/yaml.c
var yamlSource string

// The file reader and writer, separate from the strings because it is the only
// part of the runtime that touches a file, and a program that never does should
// not carry it.
//
//go:embed csrc/file.c
var fileSource string

// The program runner, separate from the file reader because it is the only
// part of the runtime that runs another program.
//
//go:embed csrc/exec.c
var execSource string

//go:embed csrc/crypto.c
var cryptoSource string

//go:embed csrc/clock.c
var clockSource string

//go:embed csrc/bits.c
var bitsSource string

// The floating-point functions, separate because they are the only part of the
// runtime that needs the C library's maths, which a hosted build links apart.
//
//go:embed csrc/math.c
var mathSource string

// The process and its file descriptors, for `std.os`.
//
//go:embed csrc/os.c
var osSource string

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
	//
	// None of them is `static`. The fragment is emitted into the runtime's own
	// translation unit and the call to it is emitted into the program's, so a
	// `static` prototype in the program declares a symbol that unit does not
	// define: the call is compiled as an internal one and the link fails on a
	// name that was declared right there. A fragment whose symbols only the
	// runtime calls is not declared here at all.
	Decls []string
	// Libraries are what a hosted link needs besides the C library for this
	// fragment: `m` for the maths, which glibc keeps in a library of its own.
	Libraries []string
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
		Provides: []string{"rho_print", "rho_print_i32", "rho_print_u32", "rho_print_i64"},
		Decls: []string{
			"void rho_print(const char* s);",
			"void rho_print_i32(int32_t value);",
			"void rho_print_u32(uint32_t value);",
			"void rho_print_i64(int64_t value);",
		},
	},
	{
		Name:     "panic",
		Source:   panicSource,
		Provides: []string{"panicf"},
		Needs:    []string{"rho_print"},
		Decls:    []string{"void panicf(const char* msg);"},
	},
	{
		// After the panic, because running out of a board's arena is one, and
		// before everything that builds a string.
		Name:   "arena",
		Source: arenaSource,
		Provides: []string{
			"rho_str_alloc", "rho_str_dup", "rho_str_adopt",
			"rho_arena_mark", "rho_arena_release", "rho_arena_keep", "rho_arena_release_cond", "rho_arena_used",
		},
		Needs: []string{"panicf"},
		Decls: []string{
			"char* rho_str_alloc(size_t n);",
			"size_t rho_arena_mark(void);",
			"void rho_arena_release(size_t mark);",
			"const char* rho_arena_keep(size_t mark, const char* s);",
			"bool rho_arena_release_cond(size_t mark, bool value);",
			"size_t rho_arena_used(void);",
		},
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
		Needs: []string{"rho_str_alloc"},
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
		Name:   "text",
		Source: textSource,
		Provides: []string{
			"rho_str_trim", "rho_str_url_encode", "rho_str_url_decode", "rho_form_field",
			"rho_milli_valid", "rho_milli_parse", "rho_milli_format", "rho_milli_round",
			"rho_i64_to_str", "rho_i64_from_str", "rho_str_lower", "rho_str_upper", "rho_str_field", "rho_str_replace",
			"rho_json_str_field", "rho_json_num_field", "rho_app_workspace",
		},
		Needs: []string{"rho_str_alloc"},
		Decls: []string{
			"char* rho_str_trim(const char* s);",
			"char* rho_str_url_encode(const char* s);",
			"char* rho_str_url_decode(const char* s);",
			"char* rho_form_field(const char* body, const char* name);",
			"bool rho_milli_valid(const char* text);",
			"int64_t rho_milli_parse(const char* text);",
			"char* rho_milli_format(int64_t milli);",
			"int64_t rho_milli_round(int64_t milli, uint32_t places);",
			"char* rho_i64_to_str(int64_t value);",
			"int64_t rho_i64_from_str(const char* text);",
			"char* rho_str_lower(const char* s);",
			"char* rho_str_upper(const char* s);",
			"char* rho_str_field(const char* s, const char* sep, uint32_t index);",
			"char* rho_str_replace(const char* s, const char* from, const char* to);",
			"char* rho_json_str_field(const char* doc, const char* key);",
			"char* rho_json_num_field(const char* doc, const char* key);",
			"char* rho_app_workspace(void);",
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
		Name:      "math",
		Source:    mathSource,
		Provides:  []string{"rho_sqrt", "rho_pow", "rho_sin", "rho_cos", "rho_atan2", "rho_fabs", "rho_floor", "rho_clamp_f64", "rho_lerp"},
		Libraries: []string{"m"},
		Decls: []string{
			"double rho_sqrt(double x);",
			"double rho_pow(double x, double y);",
			"double rho_sin(double x);",
			"double rho_cos(double x);",
			"double rho_atan2(double y, double x);",
			"double rho_fabs(double x);",
			"double rho_floor(double x);",
			"double rho_clamp_f64(double value, double low, double high);",
			"double rho_lerp(double a, double b, double t);",
		},
	},
	{
		Name:     "os",
		Source:   osSource,
		Provides: []string{"rho_exit", "rho_time_now_ns", "rho_fd_open", "rho_fd_close", "rho_fd_write", "rho_fd_read"},
		Decls: []string{
			"void rho_exit(uint32_t code);",
			"uint64_t rho_time_now_ns(void);",
			"int32_t rho_fd_open(const uint8_t* path, uint32_t flags);",
			"uint32_t rho_fd_close(uint32_t fd);",
			"uint32_t rho_fd_write(uint32_t fd, const uint8_t* buf, uint32_t len);",
			"uint32_t rho_fd_read(uint32_t fd, uint8_t* buf, uint32_t len);",
		},
	},
	{
		Name:     "lock",
		Source:   lockSource,
		Provides: []string{"rho_lock", "rho_unlock"},
		Decls:    []string{"void rho_lock(const char* name);", "void rho_unlock(const char* name);"},
	},
	{
		Name:     "env",
		Source:   envSource,
		Provides: []string{"rho_env_or", "rho_getenv", "rho_setenv"},
		Needs:    []string{"rho_str_dup"},
		Decls: []string{
			"const char* rho_env_or(const char* name, const char* fallback);",
			"char* rho_getenv(const char* name);",
			"void rho_setenv(const char* name, const char* value);",
		},
	},
	{
		Name:   "crypto",
		Source: cryptoSource,
		Provides: []string{
			"rho_hmac_sha256_hex",
			"rho_random_hex",
		},
		Needs: []string{"rho_str_alloc"},
		Decls: []string{
			"char* rho_hmac_sha256_hex(const char* key_hex, const char* message_hex);",
			"char* rho_random_hex(unsigned int bytes);",
		},
	},
	{
		Name:   "exec",
		Source: execSource,
		Provides: []string{
			"rho_exec",
			"rho_exec_args",
			"rho_exec_capture",
			"rho_sleep_ms",
		},
		Needs: []string{"rho_strlen", "rho_str_adopt"},
		Decls: []string{
			"unsigned int rho_exec(const char* program, const char* arguments, const char* output_path);",
			"unsigned int rho_exec_args(const char* program, const char* arguments, const char* output_path);",
			"char* rho_exec_capture(const char* program, const char* arguments);",
			"void rho_sleep_ms(unsigned int milliseconds);",
		},
	},
	{
		Name:   "clock",
		Source: clockSource,
		Provides: []string{
			"rho_now_s",
			"rho_local_date",
			"rho_local_clock",
			"rho_utc_offset_at",
		},
		Needs: []string{"rho_str_dup"},
		Decls: []string{
			"unsigned int rho_now_s(void);",
			"char* rho_local_date(void);",
			"char* rho_local_clock(void);",
			"long long rho_utc_offset_at(long long epoch);",
		},
	},
	{
		Name:   "file",
		Source: fileSource,
		Provides: []string{
			"rho_read_text",
			"rho_write_text",
			"rho_write_secret",
			"rho_append_text",
		},
		Needs: []string{"rho_strlen", "rho_str_adopt"},
		Decls: []string{
			"char* rho_read_text(const char* path);",
			"int rho_write_text(const char* path, const char* text);",
			"int rho_write_secret(const char* path, const char* text);",
			"int rho_append_text(const char* path, const char* text);",
		},
	},
	{
		Name:   "yaml",
		Source: yamlSource,
		Provides: []string{
			"rho_yaml_load",
			"rho_yaml_value",
			"rho_yaml_kind",
			"rho_yaml_keys",
		},
		Needs: []string{"rho_strlen", "rho_str_adopt"},
		Decls: []string{
			"char* rho_yaml_load(const char* path);",
			"char* rho_yaml_value(const char* path, const char* key);",
			"char* rho_yaml_kind(const char* path, const char* key);",
			"char* rho_yaml_keys(const char* path, const char* key);",
		},
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

// RuntimeLibraries are the libraries a link of this runtime needs besides the C
// library, in the order the fragments are emitted: what the generated build adds
// to the program's link line. Read from the runtime the generator produced, by
// the banner each emitted fragment carries.
func RuntimeLibraries(runtime string) []string {
	libraries := []string{}
	seen := map[string]bool{}
	for _, fragment := range runtimeFragments {
		if !strings.Contains(runtime, fragment.banner()) {
			continue
		}
		for _, library := range fragment.Libraries {
			if !seen[library] {
				seen[library] = true
				libraries = append(libraries, library)
			}
		}
	}
	return libraries
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
