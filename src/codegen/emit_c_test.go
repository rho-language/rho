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
