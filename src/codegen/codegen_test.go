//
// @file codegen_test.go
// @brief Rho Compiler - C backend tests.
//
// This file contains deterministic unit tests for the Rho C backend.
// Tests validate correct emission of C code for modules, structs,
// functions, and IR instructions.
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
    "strings"
    "testing"

    "rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Tests: Module Header                                                       */
/* -------------------------------------------------------------------------- */

func TestEmitModuleHeader(t *testing.T) {
    cg := New()
    m := ir.NewModuleIR("lighting")

    out := cg.EmitModule(m)

    if !strings.Contains(out, "module lighting") {
        t.Fatalf("expected module name in header")
    }
    if !strings.Contains(out, "#include <stdint.h>") {
        t.Fatalf("expected stdint include")
    }
    if !strings.Contains(out, "#include <stdbool.h>") {
        t.Fatalf("expected stdbool include")
    }
}

/* -------------------------------------------------------------------------- */
/*  Tests: Struct Emission                                                     */
/* -------------------------------------------------------------------------- */

func TestEmitStruct(t *testing.T) {
    cg := New()

    m := ir.NewModuleIR("lighting")
    m.Structs = append(m.Structs, ir.StructIR{
        Name: "Light",
        Fields: []ir.FieldIR{
            {Name: "pin", Type: ir.Type{Kind: ir.TypeU8, Name: "u8"}},
            {Name: "enabled", Type: ir.Type{Kind: ir.TypeBool, Name: "bool"}},
        },
    })

    out := cg.EmitModule(m)

    if !strings.Contains(out, "typedef struct") {
        t.Fatalf("expected struct typedef")
    }
    if !strings.Contains(out, "uint8_t pin;") {
        t.Fatalf("expected uint8_t pin field")
    }
    if !strings.Contains(out, "bool enabled;") {
        t.Fatalf("expected bool enabled field")
    }
    if !strings.Contains(out, "} Light;") {
        t.Fatalf("expected struct name Light")
    }
}

/* -------------------------------------------------------------------------- */
/*  Tests: Function Signature                                                  */
/* -------------------------------------------------------------------------- */

func TestEmitFunctionSignature(t *testing.T) {
    cg := New()

    m := ir.NewModuleIR("lighting")
    m.Funcs = append(m.Funcs, ir.FuncIR{
        Name: "set",
        Params: []ir.ParamIR{
            {Name: "pin", Type: ir.Type{Kind: ir.TypeU8}},
            {Name: "value", Type: ir.Type{Kind: ir.TypeBool}},
        },
        ReturnType: ir.Type{Kind: ir.TypeBool},
        Body:       []ir.Instr{},
    })

    out := cg.EmitModule(m)

    if !strings.Contains(out, "bool set(") {
        t.Fatalf("expected function return type bool")
    }
    if !strings.Contains(out, "uint8_t pin") {
        t.Fatalf("expected uint8_t pin parameter")
    }
    if !strings.Contains(out, "bool value") {
        t.Fatalf("expected bool value parameter")
    }
}

/* -------------------------------------------------------------------------- */
/*  Tests: Instruction Emission                                                */
/* -------------------------------------------------------------------------- */

func TestEmitInstructions(t *testing.T) {
    cg := New()

    m := ir.NewModuleIR("lighting")
    m.Funcs = append(m.Funcs, ir.FuncIR{
        Name:       "test",
        Params:     []ir.ParamIR{},
        ReturnType: ir.Type{Kind: ir.TypeVoid},
        Body: []ir.Instr{
            {Op: ir.OpAssign, Dest: "x", Args: []string{"y"}},
            {Op: ir.OpReturn, Args: []string{}},
        },
    })

    out := cg.EmitModule(m)

    if !strings.Contains(out, "x = y;") {
        t.Fatalf("expected assignment instruction")
    }
    if !strings.Contains(out, "return;") {
        t.Fatalf("expected return instruction")
    }
}

/* -------------------------------------------------------------------------- */
/*  Tests: Full Module                                                         */
/* -------------------------------------------------------------------------- */

func TestEmitFullModule(t *testing.T) {
    cg := New()

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
        Body: []ir.Instr{
            {Op: ir.OpNop},
        },
    })

    out := cg.EmitModule(m)

    if !strings.Contains(out, "typedef struct") {
        t.Fatalf("expected struct typedef")
    }
    if !strings.Contains(out, "void init(") {
        t.Fatalf("expected void init() function")
    }
    if !strings.Contains(out, "/* nop */") {
        t.Fatalf("expected nop instruction")
    }
}
