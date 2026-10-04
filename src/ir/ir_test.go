//
// @file ir_test.go
// @brief Rho Compiler - IR subsystem tests.
//
// This file contains deterministic unit tests for the Rho Intermediate
// Representation (IR). Tests validate correct construction of IR modules,
// structs, functions, parameters, types, and instructions.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package ir

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Types                                                              */
/* -------------------------------------------------------------------------- */

func TestIRTypes(t *testing.T) {
	tU8 := Type{Kind: TypeU8, Name: "u8"}
	tBool := Type{Kind: TypeBool, Name: "bool"}

	if tU8.Kind != TypeU8 || tU8.Name != "u8" {
		t.Fatalf("unexpected TypeU8: %+v", tU8)
	}

	if tBool.Kind != TypeBool || tBool.Name != "bool" {
		t.Fatalf("unexpected TypeBool: %+v", tBool)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Struct IR                                                          */
/* -------------------------------------------------------------------------- */

func TestStructIR(t *testing.T) {
	s := StructIR{
		Name: "Light",
		Fields: []FieldIR{
			{Name: "pin", Type: Type{Kind: TypeU8, Name: "u8"}},
			{Name: "enabled", Type: Type{Kind: TypeBool, Name: "bool"}},
		},
	}

	if s.Name != "Light" {
		t.Fatalf("expected struct name 'Light', got %q", s.Name)
	}

	if len(s.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(s.Fields))
	}

	if s.Fields[0].Name != "pin" || s.Fields[0].Type.Name != "u8" {
		t.Fatalf("unexpected field 0: %+v", s.Fields[0])
	}

	if s.Fields[1].Name != "enabled" || s.Fields[1].Type.Name != "bool" {
		t.Fatalf("unexpected field 1: %+v", s.Fields[1])
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Function IR                                                        */
/* -------------------------------------------------------------------------- */

func TestFuncIR(t *testing.T) {
	fn := FuncIR{
		Name: "set",
		Params: []ParamIR{
			{Name: "pin", Type: Type{Kind: TypeU8, Name: "u8"}},
			{Name: "value", Type: Type{Kind: TypeBool, Name: "bool"}},
		},
		ReturnType: Type{Kind: TypeBool, Name: "bool"},
		Body:       []Instr{},
	}

	if fn.Name != "set" {
		t.Fatalf("expected function name 'set', got %q", fn.Name)
	}

	if len(fn.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(fn.Params))
	}

	if fn.Params[0].Name != "pin" || fn.Params[0].Type.Name != "u8" {
		t.Fatalf("unexpected param 0: %+v", fn.Params[0])
	}

	if fn.Params[1].Name != "value" || fn.Params[1].Type.Name != "bool" {
		t.Fatalf("unexpected param 1: %+v", fn.Params[1])
	}

	if fn.ReturnType.Name != "bool" {
		t.Fatalf("expected return type 'bool', got %q", fn.ReturnType.Name)
	}

	if len(fn.Body) != 0 {
		t.Fatalf("expected empty body for Rho 0.1, got %d instructions", len(fn.Body))
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: Instructions                                                        */
/* -------------------------------------------------------------------------- */

func TestInstr(t *testing.T) {
	instr := Instr{
		Op:   OpAssign,
		Dest: "x",
		Args: []string{"y"},
	}

	if instr.Op != OpAssign || instr.Dest != "x" || len(instr.Args) != 1 || instr.Args[0] != "y" {
		t.Fatalf("unexpected instruction: %+v", instr)
	}
}
