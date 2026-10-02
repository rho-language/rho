//
// @file emit_c_test.go
// @brief Rho Compiler - C emission facade tests.
//
// This file contains deterministic unit tests for the high-level
// C emission API (EmitC, EmitCToFile). Tests validate that the
// facade correctly delegates to the backend and produces stable
// C output and files.
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
	"os"
	"strings"
	"testing"

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Tests: EmitC                                                              */
/* -------------------------------------------------------------------------- */

func TestEmitC(t *testing.T) {
	m := ir.NewModuleIR("lighting")
	m.Structs = append(m.Structs, ir.StructIR{
		Name: "Light",
		Fields: []ir.FieldIR{
			{Name: "pin", Type: ir.Type{Kind: ir.TypeU8}},
		},
	})
	m.Funcs = append(m.Funcs, ir.FuncIR{
		Name:       "init",
		Params:     []ir.ParamIR{},
		ReturnType: ir.Type{Kind: ir.TypeVoid},
		Body:       []ir.Instr{{Op: ir.OpNop}},
	})

	out := EmitC(m)

	if !strings.Contains(out, "module lighting") {
		t.Fatalf("expected module name in emitted C")
	}
	if !strings.Contains(out, "typedef struct") {
		t.Fatalf("expected struct typedef in emitted C")
	}
	if !strings.Contains(out, "void init(") {
		t.Fatalf("expected function init in emitted C")
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: EmitCToFile                                                        */
/* -------------------------------------------------------------------------- */

func TestEmitCToFile(t *testing.T) {
	m := ir.NewModuleIR("lighting")
	m.Funcs = append(m.Funcs, ir.FuncIR{
		Name:       "test",
		Params:     []ir.ParamIR{},
		ReturnType: ir.Type{Kind: ir.TypeVoid},
		Body:       []ir.Instr{{Op: ir.OpNop}},
	})

	tmp := "emit_c_test_output.c"
	defer os.Remove(tmp)

	if err := EmitCToFile(tmp, m); err != nil {
		t.Fatalf("EmitCToFile failed: %v", err)
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("failed to read emitted file: %v", err)
	}

	out := string(data)
	if !strings.Contains(out, "void test(") {
		t.Fatalf("expected function test in emitted C file")
	}
	if !strings.Contains(out, "/* nop */") {
		t.Fatalf("expected nop instruction in emitted C file")
	}
}

// A `main` declared void is emitted as `int`, so that something can call it and
// read an exit status. Its `return` carries no value, because Rho's `main` has
// none to carry — so the C has to supply one. Emitting the bare `return;` instead
// is a warning the compiler accepts, and an exit status that is whatever
// happened to be in the register.
func TestAVoidMainReturnsZero(t *testing.T) {
	m := ir.NewModuleIR("hosted")
	m.Funcs = append(m.Funcs, ir.FuncIR{
		Name:       "main",
		ReturnType: ir.Type{Kind: ir.TypeVoid},
		Body: []ir.Instr{
			// Lowered the way the front end lowers a `return` with no expression:
			// one operand, and that operand empty. Nested, because a nested block is
			// emitted by a child generator and the rule has to reach it as well.
			{
				Op:        ir.OpIf,
				Condition: "ready",
				Then:      []ir.Instr{{Op: ir.OpReturn, Args: []string{""}}},
			},
			{Op: ir.OpReturn, Args: []string{""}},
		},
	})

	out := EmitC(m)
	if !strings.Contains(out, "int main()") {
		t.Fatalf("a void main should be emitted as int:\n%s", out)
	}
	if strings.Contains(out, "return;") {
		t.Errorf("a void main emitted as int should not return without a value:\n%s", out)
	}
	if !strings.Contains(out, "return 0;") {
		t.Errorf("a void main's return should be the success status:\n%s", out)
	}
}

// The value belongs to `main` alone. Every other void function returns void in C
// as well, and giving its `return` a value is a different mistake.
func TestAVoidFunctionThatIsNotMainReturnsBare(t *testing.T) {
	m := ir.NewModuleIR("helper")
	m.Funcs = append(m.Funcs, ir.FuncIR{
		Name:       "stop",
		ReturnType: ir.Type{Kind: ir.TypeVoid},
		Body: []ir.Instr{
			{Op: ir.OpReturn, Args: []string{""}},
		},
	})

	out := EmitC(m)
	if !strings.Contains(out, "void stop()") {
		t.Fatalf("a void function should stay void:\n%s", out)
	}
	if !strings.Contains(out, "return;") {
		t.Errorf("a void function's return should carry nothing:\n%s", out)
	}
}
