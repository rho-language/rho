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
		if len(fn.Body) == 0 && (fn.ReturnType.Kind != ir.TypeVoid || hostMain) {
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
	case ir.TypeBool, ir.TypeU8, ir.TypeU16, ir.TypeU32, ir.TypeU64, ir.TypeI32, ir.TypeF64:
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
	case ir.TypeF64:
		return "double"
	case ir.TypeString:
		return "const char*"
	case ir.TypeStruct:
		if t.Name != "" {
			return strings.NewReplacer("[", "_", "]", "", ",", "_").Replace(t.Name)
		}
		return "struct /* anonymous */"
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
