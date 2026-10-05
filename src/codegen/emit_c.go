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
// SPDX-License-Identifier: Apache-2.0
//

package codegen

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"fmt"
	"os"
	"rho/src/ir"
	"unicode"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// Parts is a module compiled: the program, and the runtime it calls into.
//
// Two translation units rather than one file with a marker in it, because that is
// what the build wants and because a caller that has to find the boundary by
// searching for the text `static char* rho_concat` is a caller one runtime change
// away from producing two files that do not compile.
type Parts struct {
	// Program is the application: its structs, its functions, its foreign calls.
	Program string
	// Runtime is the C those functions call, feature-gated to what they use.
	Runtime string
}

// Emit compiles a module into its two translation units.
func Emit(m ir.ModuleIR) Parts {
	cg := New()
	program := cg.EmitModule(m)
	return Parts{Program: program, Runtime: cg.Runtime()}
}

// EmitC generates C code for a given IR module.
// This is the stable API used by the compiler pipeline.
//
// The whole thing — program then runtime — because that is what a caller
// compiling one file wants. A caller generating a project wants `Emit`, which
// keeps them apart.
func EmitC(m ir.ModuleIR) string {
	parts := Emit(m)
	return parts.Program + "\n" + parts.Runtime
}

// EmitCToFile writes the generated C code to a file.
// The compiler orchestrator uses this to produce .c outputs.
func EmitCToFile(path string, m ir.ModuleIR) error {
	if err := ValidateModule(m); err != nil {
		return err
	}
	// Program and runtime into one file, because this is the single-file entry
	// point: a caller that wants them apart is generating a project and uses
	// `Emit`.
	parts := Emit(m)
	return writeFile(path, parts.Program+"\n"+parts.Runtime)
}

func ValidateModule(m ir.ModuleIR) error {
	seen := map[string]bool{}
	for _, structure := range m.Structs {
		if err := validateIdentifier(structure.Name); err != nil {
			return fmt.Errorf("struct %s: %w", structure.Name, err)
		}
		if seen[structure.Name] {
			return fmt.Errorf("duplicate C declaration %q", structure.Name)
		}
		seen[structure.Name] = true
		for _, field := range structure.Fields {
			if err := validateIdentifier(field.Name); err != nil {
				return fmt.Errorf("field %s: %w", field.Name, err)
			}
		}
	}
	for _, function := range m.Funcs {
		name := function.CName
		if name == "" {
			name = function.Name
		}
		if err := validateIdentifier(name); err != nil {
			return fmt.Errorf("function %s: %w", name, err)
		}
		if seen[name] {
			return fmt.Errorf("duplicate C declaration %q", name)
		}
		seen[name] = true
		for _, parameter := range function.Params {
			if err := validateIdentifier(parameter.Name); err != nil {
				return fmt.Errorf("parameter %s: %w", parameter.Name, err)
			}
		}
	}
	return nil
}

func validateIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("empty identifier")
	}
	for index, character := range name {
		if (index == 0 && !(character == '_' || unicode.IsLetter(character))) || (index > 0 && !(character == '_' || unicode.IsLetter(character) || unicode.IsDigit(character))) {
			return fmt.Errorf("invalid C identifier")
		}
	}
	return nil
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
