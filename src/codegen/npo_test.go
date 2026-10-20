//
// @file npo_test.go
// @brief An optional pointer, which costs nothing.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"strings"
	"testing"

	"rho/src/ir"
)

// optionalPointerModule is a module holding one optional pointer, which is
// everything these tests need: a foreign function returning a pointer that may
// be null, and a local holding the answer.
func optionalPointerModule() ir.ModuleIR {
	m := ir.NewModuleIR("main")
	handle := ir.Type{Kind: ir.TypeStruct, Name: "Handle"}
	optional := ir.Type{Kind: ir.TypeStruct, Name: "Option_Ptr_Handle", Args: []ir.Type{ir.Type{Kind: ir.TypePtr, Args: []ir.Type{handle}}}}

	// The specialisation the standard library's `Option[Ptr[T]]` is emitted as.
	// It is declared here as the struct it is written as, because that is what a
	// backend sees: the point is that the declaration is emitted as a pointer and
	// not as the two fields the source was written with.
	m.Structs = append(m.Structs, ir.StructIR{
		Name: "Option_Ptr_Handle",
		Fields: []ir.FieldIR{
			{Name: "is_some", Type: ir.Type{Kind: ir.TypeBool}},
			{Name: "value", Type: ir.Type{Kind: ir.TypePtr, Args: []ir.Type{handle}}},
		},
	})

	m.Funcs = append(m.Funcs, ir.FuncIR{
		Name:       "find",
		Extern:     true,
		Params:     []ir.ParamIR{{Name: "name", Type: ir.Type{Kind: ir.TypePtr, Args: []ir.Type{{Kind: ir.TypeU8}}}}},
		ReturnType: optional,
	})
	m.Funcs = append(m.Funcs, ir.FuncIR{
		Name:       "is_present",
		Params:     []ir.ParamIR{{Name: "o", Type: optional}},
		ReturnType: ir.Type{Kind: ir.TypeBool},
		Body: []ir.Instr{{
			Op:   ir.OpRaw,
			Args: []string{"return ((o) != NULL);"},
		}},
	})
	return m
}

// `Option[Ptr[T]]` is the pointer. Not a struct with a tag and a payload — the
// pointer — so that a value a C library returned arrives as the value C
// returned, and a null still means absent.
func TestAnOptionalPointerIsDeclaredAsThePointer(t *testing.T) {
	out := EmitC(optionalPointerModule())

	if !strings.Contains(out, "typedef Handle* Option_Ptr_Handle;") {
		t.Errorf("an optional pointer should be declared as the pointer it is")
	}
	if strings.Contains(out, "is_some;") || strings.Contains(out, "is_none;") {
		t.Error("an optional pointer should have no fields; the absence is the null")
	}
}

// A foreign function is not declared here. Its header declares it, and that
// header is included by the generated interface; declaring it again would state
// the same function twice in two languages, and any C type spelled differently in
// the two — every enum, every typedef — makes that a compile error over a
// signature that was already correct.
func TestAForeignFunctionIsNotRedeclared(t *testing.T) {
	out := EmitC(optionalPointerModule())

	if strings.Contains(out, "find(") {
		t.Errorf("a foreign function is declared by its header, not here:\n%s", firstLines(out, 40))
	}
	// The type is still declared: a program may name it, and the specialisation
	// is what the value is represented as.
	if !strings.Contains(out, "typedef Handle* Option_Ptr_Handle;") {
		t.Error("the optional pointer should still be declared")
	}
}

// firstLines is a short excerpt for a failure message, which reads better than a
// whole emitted file.
func firstLines(out string, n int) string {
	lines := strings.Split(out, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// A pointer to a pointer is a pointer to the pointer: the C type of
// `Ptr_Ptr_Thing` is `Thing**`, and a name that stopped at one star would be a
// declaration of a different type than the program means.
func TestAMangledPointerToAPointerKeepsBothStars(t *testing.T) {
	cases := map[string]string{
		"Ptr_Thing":     "Thing*",
		"Ptr_u8":        "uint8_t*",
		"Thing_Ptr":     "Thing*",
		"Ptr_Ptr_Thing": "Thing**",
		"Ptr_Ptr_u8":    "uint8_t**",
	}
	for name, want := range cases {
		got, ok := mangledPointerCType(name)
		if !ok {
			t.Errorf("expected %q to be a pointer", name)
			continue
		}
		if got != want {
			t.Errorf("expected %q to be %q, got %q", name, want, got)
		}
	}
}
