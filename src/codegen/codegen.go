//
// @file codegen.go
// @brief Rho Compiler - C backend code generator.
//
// This file implements the C backend for the Rho compiler. It consumes
// the Rho IR (ModuleIR, StructIR, FuncIR, etc.) and emits deterministic,
// portable C code. The backend is intentionally simple and explicit,
// with no hidden runtime or magic.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package codegen

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"fmt"
	"strings"

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Codegen Type                                                              */
/* -------------------------------------------------------------------------- */

// Codegen is the main C backend for the Rho compiler.
type Codegen struct {
	b strings.Builder
	// foreign maps a qualified function name to the symbol its C header declares.
	// A call site is lowered once for every backend and says the qualified name;
	// this is where C reads it as the name that exists.
	foreign map[string]string
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// New creates a new C code generator.
func New() *Codegen {
	return &Codegen{
		b: strings.Builder{},
	}
}

// EmitModule generates C code for a given IR module and returns it as a string.
func (cg *Codegen) EmitModule(m ir.ModuleIR) string {
	cg.b.Reset()

	cg.emitHeader(m)
	// A call site is lowered once, for every backend, and carries the qualified
	// name. Go wants that — every module of a program compiles into one package,
	// where two same-named functions would collide. C cannot use it, because an
	// extern's symbol is whatever its header's compiler agreed on.
	//
	// The rename is done over the whole program before anything is written,
	// rather than at each place an expression is emitted. Every instruction that
	// carries lowered text would otherwise have to remember: a call in a `let`, in
	// a bare statement, in an `if` condition, in a `while`, in a return. Missing
	// one produces an undefined reference to a name that looks like a typo rather
	// than a missing binding, which is a hard thing to read back to this.
	cg.foreign = foreignCallNames(m.Funcs)
	m.Funcs = cg.renameForeignInFuncs(m.Funcs)

	cg.emitStructs(m.Structs)
	cg.emitFuncPrototypes(m.Funcs)
	cg.emitFuncs(m.Funcs)

	return cg.b.String()
}

func (cg *Codegen) emitFuncPrototypes(funcs []ir.FuncIR) {
	for _, fn := range funcs {
		// An extern is declared by its header, and that header is included by
		// the generated interface. A prototype here would be the same function
		// declared twice — and declared in two languages, because this one is
		// written in the types Rho mapped the C ones to. Any C type spelled
		// differently in the two, which is every enum and every typedef, makes
		// the pair a compile error over a signature that was already correct.
		if fn.Extern {
			continue
		}
		cg.emitFunctionSignature(fn)
		cg.b.WriteString(";\n")
	}
	if len(funcs) > 0 {
		cg.b.WriteString("\n")
	}
}

/* -------------------------------------------------------------------------- */
/*  Internal: Header                                                          */
/* -------------------------------------------------------------------------- */

func (cg *Codegen) emitHeader(m ir.ModuleIR) {
	cg.b.WriteString("/* Rho → C backend: module ")
	cg.b.WriteString(m.Name)
	cg.b.WriteString(" */\n\n")
	cg.b.WriteString("#define _POSIX_C_SOURCE 200809L\n")
	cg.b.WriteString("#include <stdint.h>\n")
	cg.b.WriteString("#include <stdbool.h>\n")
	cg.b.WriteString("#include <stdio.h>\n")
	cg.b.WriteString("#include <stdlib.h>\n")
	cg.b.WriteString("#include <string.h>\n")
	cg.b.WriteString("#include <stddef.h>\n\n")
	for _, feature := range m.Features {
		name := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_", "/", "_").Replace(feature))
		cg.b.WriteString("#define RHO_FEATURE_")
		cg.b.WriteString(name)
		cg.b.WriteString(" 1\n")
	}
	if len(m.Features) > 0 {
		cg.b.WriteString("\n")
	}
	// `_init` and `_fini` are the C library's hooks for code that runs before and
	// after everything else. A board's startup assembly calls the library's
	// initializer, which calls them, and a build with `-nostartfiles` — which a
	// bare-metal build has, so that its own startup is the entry point rather than
	// the compiler's — has excluded the two files that define them.
	//
	// Weak, because the same generated project is also what a hosted target
	// builds, and there the C library defines them properly. A strong definition
	// would collide with it; a weak one yields to it and is used only where
	// nothing else provides them, which is exactly the bare-metal case and exactly
	// when they are needed.
	//
	// Empty, which is what they are for on a board: there is nothing to do before
	// a program's own initializers, and the library's own work is done by
	// `__libc_init_array`, which the startup calls separately.
	cg.b.WriteString("// Weak: -nostartfiles excludes the C library's own definitions of these, and a\n")
	cg.b.WriteString("// hosted C library defines them itself. Used only where nothing else does,\n")
	cg.b.WriteString("// which is a board, where the startup assembly reaches them through the\n")
	cg.b.WriteString("// library's initializer.\n")
	cg.b.WriteString("__attribute__((weak)) void _init(void);\n__attribute__((weak)) void _init(void) {}\n")
	cg.b.WriteString("__attribute__((weak)) void _fini(void);\n__attribute__((weak)) void _fini(void) {}\n\n")
	cg.b.WriteString("static char* rho_concat(const char* a, const char* b);\n")
	cg.b.WriteString("static char* rho_yaml_load(const char* path);\n\n")
	cg.b.WriteString("static char* rho_string_from_i32(int32_t value);\n\n")
	cg.b.WriteString("static bool rho_str_eq(const char* a, const char* b);\n\n")
	cg.b.WriteString("static void panicf(const char* msg);\n\n")
	cg.b.WriteString("static uint32_t rho_strlen(const char* s);\n\n")
	cg.b.WriteString("static bool rho_str_starts_with(const char* s, const char* prefix);\n\n")
	cg.b.WriteString("static const char* rho_str_trim_prefix(const char* s, const char* prefix);\n\n")
	cg.b.WriteString("static bool rho_str_contains(const char* haystack, const char* needle);\n\n")
	cg.b.WriteString("static const char* rho_env_or(const char* name, const char* fallback);\n\n")
	cg.b.WriteString("static uint8_t rho_str_at(const char* s, uint32_t i);\n\n")
	cg.b.WriteString("static int32_t rho_str_index_of(const char* haystack, const char* needle);\n\n")
	cg.b.WriteString("static const char* rho_str_slice(const char* s, uint32_t start, uint32_t length);\n\n")
	cg.b.WriteString("static const char* rho_int_to_str(uint32_t n);\n\n")
	cg.b.WriteString("static const char* rho_str_from_byte(uint8_t b);\n\n")
	cg.b.WriteString("static uint32_t rho_int_from_str(const char* s);\n\n")
}

/* -------------------------------------------------------------------------- */
/*  Internal: Structs                                                         */
/* -------------------------------------------------------------------------- */

func (cg *Codegen) emitStructs(structs []ir.StructIR) {
	for _, s := range structs {
		// An optional pointer is the pointer. It is declared as the C type it
		// is represented as rather than as a struct with a tag and a payload,
		// because that is what makes a value returned by a C library usable
		// without a translation step in either direction.
		if payload, ok := optionalPointerCType(s.Name); ok {
			cg.b.WriteString("typedef ")
			cg.b.WriteString(payload)
			cg.b.WriteString(" ")
			cg.b.WriteString(s.Name)
			cg.b.WriteString(";\n\n")
			continue
		}
		cg.b.WriteString("typedef struct {\n")
		for _, f := range s.Fields {
			cg.b.WriteString("    ")
			cg.b.WriteString(cTypeFor(f.Type))
			cg.b.WriteString(" ")
			cg.b.WriteString(f.Name)
			cg.b.WriteString(";\n")
		}
		cg.b.WriteString("} ")
		cg.b.WriteString(s.Name)
		cg.b.WriteString(";\n\n")
	}
}

/* -------------------------------------------------------------------------- */
/*  Internal: Functions                                                       */
/* -------------------------------------------------------------------------- */

func (cg *Codegen) emitFuncs(funcs []ir.FuncIR) {
	for _, fn := range funcs {
		if fn.Extern {
			// An extern is declared by the header it came from, and that header
			// is included by the generated interface. Restating it here would be
			// redundant at best and wrong at worst: the declaration here is in
			// terms of the types Rho mapped the C ones to, and any C type that is
			// not spelled the same way in both — an enum, a typedef, anything
			// whose width was inferred — makes the two declarations conflict and
			// fails the compile over a signature that was already correct.
			//
			// So nothing is emitted. The name it is called by is decided at the
			// call site, and the declaration is the header's.
			continue
		}
		// signature
		returnType := fn.ReturnType
		hostMain := fn.Name == "main" && returnType.Kind == ir.TypeVoid
		if hostMain {
			cg.b.WriteString("int")
		} else {
			cg.b.WriteString(cTypeFor(returnType))
		}
		cg.b.WriteString(" ")
		name := cSymbolFor(fn)
		cg.b.WriteString(name)
		cg.b.WriteString("(")

		for i, p := range fn.Params {
			if i > 0 {
				cg.b.WriteString(", ")
			}
			cg.b.WriteString(cTypeFor(p.Type))
			cg.b.WriteString(" ")
			cg.b.WriteString(p.Name)
		}

		cg.b.WriteString(")")
		cg.b.WriteString(" {\n")

		// body (Rho 0.1: minimal, based on IR instructions)
		for _, instr := range fn.Body {
			cg.emitInstr(instr)
		}
		if hostMain {
			// A void `main` is emitted as `int` so that a real entry point can
			// call it, which means the C compiler expects a value back. Rho's
			// `main` cannot return one — it is declared void — so falling off the
			// end means it ran to completion, which is success.
			//
			// Without this the function returns whatever happened to be in the
			// return register, and that becomes the process's exit status: a
			// program that printed exactly what it should have still reported
			// failure to whatever ran it.
			cg.b.WriteString("    return 0;\n")
		} else if len(fn.Body) == 0 && fn.ReturnType.Kind != ir.TypeVoid {
			cg.b.WriteString("    return ")
			cg.b.WriteString(defaultValueFor(returnType))
			cg.b.WriteString(";\n")
		}

		cg.b.WriteString("}\n\n")
	}

	cg.b.WriteString("static char* rho_concat(const char* a, const char* b) {\n")
	cg.b.WriteString("    size_t len_a = strlen(a);\n")
	cg.b.WriteString("    size_t len_b = strlen(b);\n")
	cg.b.WriteString("    char* out = malloc(len_a + len_b + 1);\n")
	cg.b.WriteString("    if (!out) return NULL;\n")
	cg.b.WriteString("    memcpy(out, a, len_a);\n")
	cg.b.WriteString("    memcpy(out + len_a, b, len_b + 1);\n")
	cg.b.WriteString("    return out;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static const char* rho_int_to_str(uint32_t n) {\n")
	cg.b.WriteString("    char buf[16];\n")
	cg.b.WriteString("    int len = snprintf(buf, sizeof(buf), \"%u\", n);\n")
	cg.b.WriteString("    char* out = (char*)malloc(len + 1);\n")
	cg.b.WriteString("    if (!out) return \"\";\n")
	cg.b.WriteString("    memcpy(out, buf, len);\n")
	cg.b.WriteString("    out[len] = 0;\n")
	cg.b.WriteString("    return out;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static const char* rho_str_from_byte(uint8_t b) {\n")
	cg.b.WriteString("    char* out = (char*)malloc(2);\n")
	cg.b.WriteString("    if (!out) return \"\";\n")
	cg.b.WriteString("    out[0] = (char)b;\n")
	cg.b.WriteString("    out[1] = 0;\n")
	cg.b.WriteString("    return out;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static uint32_t rho_int_from_str(const char* s) {\n")
	cg.b.WriteString("    if (!s || !*s) return 0;\n")
	cg.b.WriteString("    uint32_t total = 0;\n")
	cg.b.WriteString("    for (const char* p = s; *p; p++) {\n")
	cg.b.WriteString("        if (*p < '0' || *p > '9') return 0;\n")
	cg.b.WriteString("        total = total * 10 + (uint32_t)(*p - '0');\n")
	cg.b.WriteString("    }\n")
	cg.b.WriteString("    return total;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static bool rho_str_eq(const char* a, const char* b) {\n")
	cg.b.WriteString("    return a != NULL && b != NULL && strcmp(a, b) == 0;\n")
	cg.b.WriteString("}\n\n")
	// The panic intrinsic, so a test module can fail through the assert
	// library. abort() itself comes from libc.
	cg.b.WriteString("static void panicf(const char* msg) {\n")
	cg.b.WriteString("    fprintf(stderr, \"rho: %s\\n\", msg ? msg : \"panic\");\n")
	cg.b.WriteString("    exit(1);\n")
	cg.b.WriteString("}\n\n")
	// String and environment intrinsics, so the same Rho code builds and
	// behaves the same for every target.
	cg.b.WriteString("static uint32_t rho_strlen(const char* s) {\n")
	cg.b.WriteString("    return s ? (uint32_t)strlen(s) : 0;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static bool rho_str_starts_with(const char* s, const char* prefix) {\n")
	cg.b.WriteString("    if (!s || !prefix) return false;\n")
	cg.b.WriteString("    size_t n = strlen(prefix);\n")
	cg.b.WriteString("    return strncmp(s, prefix, n) == 0;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static const char* rho_str_trim_prefix(const char* s, const char* prefix) {\n")
	cg.b.WriteString("    if (!s || !prefix) return s;\n")
	cg.b.WriteString("    size_t n = strlen(prefix);\n")
	cg.b.WriteString("    return strncmp(s, prefix, n) == 0 ? s + n : s;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static bool rho_str_contains(const char* haystack, const char* needle) {\n")
	cg.b.WriteString("    if (!haystack || !needle) return false;\n")
	cg.b.WriteString("    return strstr(haystack, needle) != NULL;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static const char* rho_env_or(const char* name, const char* fallback) {\n")
	cg.b.WriteString("    const char* value = name ? getenv(name) : NULL;\n")
	cg.b.WriteString("    return value ? value : fallback;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static uint8_t rho_str_at(const char* s, uint32_t i) {\n")
	cg.b.WriteString("    if (!s) return 0;\n")
	cg.b.WriteString("    size_t n = strlen(s);\n")
	cg.b.WriteString("    return i < n ? (uint8_t)s[i] : 0;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static int32_t rho_str_index_of(const char* haystack, const char* needle) {\n")
	cg.b.WriteString("    if (!haystack || !needle) return -1;\n")
	cg.b.WriteString("    const char* found = strstr(haystack, needle);\n")
	cg.b.WriteString("    return found ? (int32_t)(found - haystack) : -1;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static const char* rho_str_slice(const char* s, uint32_t start, uint32_t length) {\n")
	cg.b.WriteString("    if (!s) return \"\";\n")
	cg.b.WriteString("    size_t n = strlen(s);\n")
	cg.b.WriteString("    if (start >= n) return \"\";\n")
	cg.b.WriteString("    size_t avail = n - start;\n")
	cg.b.WriteString("    size_t take = length < avail ? length : avail;\n")
	cg.b.WriteString("    char* out = (char*)malloc(take + 1);\n")
	cg.b.WriteString("    if (!out) return \"\";\n")
	cg.b.WriteString("    memcpy(out, s + start, take);\n")
	cg.b.WriteString("    out[take] = 0;\n")
	cg.b.WriteString("    return out;\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static char* rho_yaml_load(const char* path) {\n")
	cg.b.WriteString("    FILE* f = fopen(path, \"r\");\n")
	cg.b.WriteString("    char line[4096];\n")
	cg.b.WriteString("    char* result = NULL;\n")
	cg.b.WriteString("    int in_user = 0;\n")
	cg.b.WriteString("    if (!f) return strdup(\"unknown\");\n")
	cg.b.WriteString("    while (fgets(line, sizeof(line), f) != NULL) {\n")
	cg.b.WriteString("        char* trimmed = line;\n")
	cg.b.WriteString("        while (*trimmed == ' ' || *trimmed == '\\t' || *trimmed == '\\n' || *trimmed == '\\r') trimmed++;\n")
	cg.b.WriteString("        if (*trimmed == '\\0' || *trimmed == '#') continue;\n")
	cg.b.WriteString("        if (strncmp(trimmed, \"user:\", 5) == 0) { in_user = 1; continue; }\n")
	cg.b.WriteString("        if (in_user && strncmp(trimmed, \"name:\", 5) == 0) {\n")
	cg.b.WriteString("            char* value = trimmed + 5;\n")
	cg.b.WriteString("            while (*value == ' ' || *value == '\\t') value++;\n")
	cg.b.WriteString("            size_t len = strlen(value);\n")
	cg.b.WriteString("            while (len > 0 && (value[len - 1] == '\\n' || value[len - 1] == '\\r')) { value[--len] = '\\0'; }\n")
	cg.b.WriteString("            result = strdup(value);\n")
	cg.b.WriteString("            break;\n")
	cg.b.WriteString("        }\n")
	cg.b.WriteString("    }\n")
	cg.b.WriteString("    fclose(f);\n")
	cg.b.WriteString("    if (result != NULL) return result;\n")
	cg.b.WriteString("    return strdup(\"unknown\");\n")
	cg.b.WriteString("}\n\n")
	cg.b.WriteString("static char* rho_string_from_i32(int32_t value) {\n")
	cg.b.WriteString("    char buffer[32];\n")
	cg.b.WriteString("    snprintf(buffer, sizeof(buffer), \"%d\", value);\n")
	cg.b.WriteString("    return strdup(buffer);\n")
	cg.b.WriteString("}\n\n")
}

func (cg *Codegen) emitFunctionSignature(fn ir.FuncIR) {
	name := cSymbolFor(fn)
	if fn.Name == "main" && fn.ReturnType.Kind == ir.TypeVoid {
		cg.b.WriteString("int")
	} else {
		cg.b.WriteString(cTypeFor(fn.ReturnType))
	}
	cg.b.WriteString(" ")
	cg.b.WriteString(name)
	cg.b.WriteString("(")
	for i, p := range fn.Params {
		if i > 0 {
			cg.b.WriteString(", ")
		}
		cg.b.WriteString(cTypeFor(p.Type))
		cg.b.WriteString(" ")
		cg.b.WriteString(p.Name)
	}
	cg.b.WriteString(")")
}

func defaultValueFor(t ir.Type) string {
	switch t.Kind {
	case ir.TypeBool, ir.TypeU8, ir.TypeU16, ir.TypeU32, ir.TypeU64, ir.TypeI32, ir.TypeI64, ir.TypeF64:
		return "0"
	case ir.TypeString:
		return "NULL"
	case ir.TypeStruct:
		return "(" + t.Name + "){0}"
	default:
		return "0"
	}
}

/* -------------------------------------------------------------------------- */
/*  Internal: Instructions                                                    */
/* -------------------------------------------------------------------------- */

func (cg *Codegen) emitInstr(instr ir.Instr) {
	switch instr.Op {
	case ir.OpReturn:
		if len(instr.Args) == 1 {
			cg.b.WriteString("    return ")
			cg.b.WriteString(instr.Args[0])
			cg.b.WriteString(";\n")
		} else {
			cg.b.WriteString("    return;\n")
		}

	case ir.OpAssign:
		if len(instr.Args) == 1 {
			cg.b.WriteString("    ")
			cg.b.WriteString(instr.Dest)
			if strings.HasSuffix(instr.Dest, "=") {
				cg.b.WriteString(" ")
			} else {
				cg.b.WriteString(" = ")
			}
			cg.b.WriteString(instr.Args[0])
			cg.b.WriteString(";\n")
		}
	case ir.OpLet:
		cg.b.WriteString("    ")
		cg.b.WriteString(instr.Type)
		cg.b.WriteString(" ")
		cg.b.WriteString(instr.Dest)
		if len(instr.Args) == 1 && instr.Args[0] != "" {
			cg.b.WriteString(" = ")
			cg.b.WriteString(cg.rewriteForeign(instr.Args[0]))
		}
		cg.b.WriteString(";\n")

	case ir.OpExpr:
		if len(instr.Args) == 1 {
			cg.b.WriteString("    ")
			cg.b.WriteString(cg.rewriteForeign(instr.Args[0]))
			cg.b.WriteString(";\n")
		}
	case ir.OpIf:
		cg.b.WriteString("    if (")
		cg.b.WriteString(instr.Condition)
		cg.b.WriteString(") {\n")
		cg.emitNested(instr.Then)
		cg.b.WriteString("    }")
		if len(instr.Else) > 0 {
			cg.b.WriteString(" else {\n")
			cg.emitNested(instr.Else)
			cg.b.WriteString("    }")
		}
		cg.b.WriteString("\n")

	case ir.OpWhile:
		cg.b.WriteString("    while (")
		cg.b.WriteString(instr.Condition)
		cg.b.WriteString(") {\n")
		cg.emitNested(instr.Then)
		cg.b.WriteString("    }\n")

	case ir.OpBreak:
		cg.b.WriteString("    break;\n")

	case ir.OpCall:
		cg.b.WriteString("    ")
		if instr.Dest != "" {
			cg.b.WriteString(instr.Dest)
			cg.b.WriteString(" = ")
		}
		if len(instr.Args) > 0 {
			cg.b.WriteString(instr.Args[0])
			cg.b.WriteString("(")
			for i := 1; i < len(instr.Args); i++ {
				if i > 1 {
					cg.b.WriteString(", ")
				}
				cg.b.WriteString(instr.Args[i])
			}
			cg.b.WriteString(");\n")
		}

	case ir.OpNop:
		cg.b.WriteString("    /* nop */\n")
	case ir.OpRaw:
		cg.b.WriteString(instr.Dest)
		cg.b.WriteString("\n")

	default:
		cg.b.WriteString("    /* unknown op */\n")
	}
}

func (cg *Codegen) emitNested(instructions []ir.Instr) {
	child := New()
	for _, instruction := range instructions {
		child.emitInstr(instruction)
	}
	for _, line := range strings.Split(strings.TrimSuffix(child.b.String(), "\n"), "\n") {
		cg.b.WriteString("    ")
		cg.b.WriteString(line)
		cg.b.WriteString("\n")
	}
}

/* -------------------------------------------------------------------------- */
/*  Internal: Type Mapping                                                    */
/* -------------------------------------------------------------------------- */

// isUnitType reports whether a type is the synthesised empty struct that means
// "no value". A pointer to it is a pointer to nothing.
func isUnitType(t ir.Type) bool {
	if t.Kind == ir.TypeVoid {
		return true
	}
	return t.Kind == ir.TypeStruct && (t.Name == "Unit" || t.Name == "RhoUnit")
}

// cTypeFor maps IR types to C types.
func cTypeFor(t ir.Type) string {
	switch t.Kind {
	case ir.TypeVoid:
		return "void"
	case ir.TypeBool:
		return "bool"
	case ir.TypeU8:
		return "uint8_t"
	case ir.TypeU16:
		return "uint16_t"
	case ir.TypeU32:
		return "uint32_t"
	case ir.TypeU64:
		return "uint64_t"
	case ir.TypeI32:
		return "int32_t"
	case ir.TypeI64:
		return "int64_t"
	case ir.TypeF64:
		return "double"
	case ir.TypeString:
		return "const char*"
	case ir.TypeStruct:
		if t.Name != "" {
			return strings.NewReplacer("[", "_", "]", "", ",", "_").Replace(t.Name)
		}
		return "struct /* anonymous */"
	case ir.TypePtr:
		// A pointer is a pointer. A pointer to nothing — `Ptr[Unit]`, which is
		// how a foreign handle is declared — is what C calls void*, and spelling
		// it as a mangled name would make every handle undeclareable.
		if len(t.Args) == 0 || isUnitType(t.Args[0]) {
			return "void*"
		}
		return cTypeFor(t.Args[0]) + "*"
	default:
		return "void /* invalid */"
	}
}

/* -------------------------------------------------------------------------- */
/*  Debug Helpers                                                             */
/* -------------------------------------------------------------------------- */

// Dump returns the current buffer content (for debugging).
func (cg *Codegen) Dump() string {
	return cg.b.String()
}

// EmitModuleToStdout is a convenience helper for quick experiments.
func (cg *Codegen) EmitModuleToStdout(m ir.ModuleIR) {
	fmt.Println(cg.EmitModule(m))
}

// renameForeignInFuncs returns the functions with every lowered expression
// renamed to the C symbols that exist.
func (cg *Codegen) renameForeignInFuncs(funcs []ir.FuncIR) []ir.FuncIR {
	if len(cg.foreign) == 0 {
		return funcs
	}
	out := make([]ir.FuncIR, len(funcs))
	copy(out, funcs)
	for index := range out {
		body := make([]ir.Instr, len(out[index].Body))
		copy(body, out[index].Body)
		for i := range body {
			cg.renameForeignInInstr(&body[i])
		}
		out[index].Body = body
	}
	return out
}

// renameForeignInInstr renames every piece of lowered text one instruction
// carries.
func (cg *Codegen) renameForeignInInstr(instr *ir.Instr) {
	for i := range instr.Args {
		instr.Args[i] = cg.rewriteForeign(instr.Args[i])
	}
	instr.Condition = cg.rewriteForeign(instr.Condition)
	for i := range instr.Then {
		inner := instr.Then[i]
		cg.renameForeignInInstr(&inner)
		instr.Then[i] = inner
	}
	for i := range instr.Else {
		inner := instr.Else[i]
		cg.renameForeignInInstr(&inner)
		instr.Else[i] = inner
	}
}

// rewriteForeign reads a lowered expression as C.
//
// A call site is lowered once, for every backend, and carries the qualified name
// that tells two same-named functions apart — which Go needs and C cannot use,
// because an extern's symbol is whatever its header's compiler agreed on. This is
// where a C expression says the name that exists.
//
// Only a whole word is replaced. The text is not parsed, and a shorter name that
// happens to be a prefix of a longer one must not rewrite the longer one.
func (cg *Codegen) rewriteForeign(text string) string {
	if len(cg.foreign) == 0 || !strings.Contains(text, "_") {
		return text
	}
	for qualified, foreign := range cg.foreign {
		text = replaceWord(text, qualified, foreign)
	}
	return text
}

// replaceWord replaces whole-word occurrences, delimited by anything that cannot
// be part of an identifier.
func replaceWord(text, from, to string) string {
	var out strings.Builder
	for {
		at := strings.Index(text, from)
		if at < 0 {
			out.WriteString(text)
			return out.String()
		}
		out.WriteString(text[:at])
		before := byte(' ')
		if at > 0 {
			before = text[at-1]
		}
		after := byte(' ')
		if at+len(from) < len(text) {
			after = text[at+len(from)]
		}
		if isIdentifierByte(before) || isIdentifierByte(after) {
			out.WriteString(text[at : at+len(from)])
		} else {
			out.WriteString(to)
		}
		text = text[at+len(from):]
	}
}

func isIdentifierByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// foreignCallNames maps each extern's qualified name to the name its header gave
// it.
//
// A call site is lowered once, for every backend, and carries the qualified name
// because that is what distinguishes two functions of the same name — which Go
// needs, since every module of a program compiles into one package. C does not
// need it and cannot use it: an extern's symbol is whatever its header's
// compiler agreed on, and the header knows nothing about this language.
//
// So the qualified name is what the call site says, and this is where C turns it
// back into the symbol that exists.
func foreignCallNames(funcs []ir.FuncIR) map[string]string {
	if len(funcs) == 0 {
		return nil
	}
	names := map[string]string{}
	for _, fn := range funcs {
		if !fn.Extern || fn.CName == "" || fn.CName == fn.Name {
			continue
		}
		names[fn.CName] = fn.Name
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

// cSymbolFor is the name a function has in C.
//
// It is the name a header gave it when the function is an extern, and a name Rho
// chose — carrying its module — when it is not.
//
// The difference is the whole of a foreign binding. An extern is a function some
// C header already declared, and its symbol is whatever that header's compiler
// agreed on; the header does not know this language exists and could not have
// avoided the name. Prefixing it produces a call to a function that was never
// defined, and the error names a symbol that looks like a typo rather than a
// missing binding.
//
// Qualification is not redundant where it is applied. Every module of a Rho
// program is emitted into one C file, so two modules declaring their own `init`
// would otherwise collide, and a name Rho chose is one Rho may namespace.
func cSymbolFor(fn ir.FuncIR) string {
	if fn.Extern {
		return fn.Name
	}
	if fn.CName != "" {
		return fn.CName
	}
	return fn.Name
}
