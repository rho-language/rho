//
// @file emit_c.go
// @brief Rho Compiler - C emission facade.
//
// This file provides a stable, high-level API for emitting C code from
// Rho IR modules. It wraps the lower-level Codegen backend and exposes
// a deterministic interface used by the compiler orchestrator.
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
    "rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// EmitC generates C code for a given IR module.
// This is the stable API used by the compiler pipeline.
func EmitC(m ir.ModuleIR) string {
    cg := New()
    return cg.EmitModule(m)
}

// EmitCToFile writes the generated C code to a file.
// The compiler orchestrator uses this to produce .c outputs.
func EmitCToFile(path string, m ir.ModuleIR) error {
    cg := New()
    out := cg.EmitModule(m)
    return writeFile(path, out)
}

/* -------------------------------------------------------------------------- */
/*  Internal: File IO                                                         */
/* -------------------------------------------------------------------------- */

// writeFile is a minimal deterministic file writer.
// No hidden buffering, no implicit directory creation.
func writeFile(path, data string) error {
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()

    _, err = f.WriteString(data)
    return err
}
