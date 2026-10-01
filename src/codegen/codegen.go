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
	cg.emitStructs(m.Structs)
	cg.emitFuncPrototypes(m.Funcs)
	cg.emitFuncs(m.Funcs)

	return cg.b.String()
}

func (cg *Codegen) emitFuncPrototypes(funcs []ir.FuncIR) {
	for _, fn := range funcs {
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
			cg.emitFunctionSignature(fn)
			cg.b.WriteString(";\n\n")
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
		name := fn.CName
		if name == "" {
			name = fn.Name
		}
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
	name := fn.CName
	if name == "" {
		name = fn.Name
	}
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
			cg.b.WriteString(instr.Args[0])
		}
		cg.b.WriteString(";\n")

	case ir.OpExpr:
		if len(instr.Args) == 1 {
			cg.b.WriteString("    ")
			cg.b.WriteString(instr.Args[0])
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
