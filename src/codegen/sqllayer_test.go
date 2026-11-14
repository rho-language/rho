//
// @file sqllayer_test.go
// @brief Tests for the language parts a database surface needs, as the backends
//        see them.
//
// A generated query layer is written in a particular shape — a row struct per
// query, a slice of them written by the caller, a result carrying a count or an
// error, and a handle the language cannot name. These are the cases that had to
// work for such a program to compile, and each of them failed at least once.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package codegen

import (
	"strings"
	"testing"

	"rho/src/ir"
)

// A signed 64-bit integer is what a database column is, and the C spelling has
// to say so rather than falling back to a double.
func TestASignedSixtyFourBitIntegerHasItsOwnSpelling(t *testing.T) {
	wide := ir.Type{Kind: ir.TypeI64, Name: "i64"}
	if got := cTypeFor(wide); got != "int64_t" {
		t.Errorf("an i64 is %q in C, want int64_t", got)
	}
	if got := goTypeForIR(wide); got != "int64" {
		t.Errorf("an i64 is %q in Go, want int64", got)
	}
	if got := goZeroValue(wide); got != "0" {
		t.Errorf("the zero of an i64 is %q in Go, want 0", got)
	}
}

// A pointer is how a foreign library's handle is written down, and `Unit` is
// what makes it one: a pointer to nothing, which C has a name for.
func TestAPointerToNothingIsAnOpaqueHandle(t *testing.T) {
	if !isUnitType(ir.Type{Kind: ir.TypeStruct, Name: "Unit"}) {
		t.Error("Unit is not recognised as the type of nothing")
	}
	handle := ir.Type{Kind: ir.TypePtr, Args: []ir.Type{{Kind: ir.TypeStruct, Name: "Unit"}}}
	if got := cTypeFor(handle); got != "void*" {
		t.Errorf("a handle renders as %q, which is not a C handle", got)
	}
}

// The unsafe import follows what the file actually says, because Go refuses a
// file with an unused import. Every pointer is an unsafe.Pointer, so a module
// that names one imports the package, and a module that names none does not.
func TestTheGoBackendImportsUnsafeOnlyWhenAPointerIsNamed(t *testing.T) {
	row := ir.Type{Kind: ir.TypeStruct, Name: "CustomerRow"}
	named := ir.ModuleIR{
		Name: "handles",
		Structs: []ir.StructIR{{
			Name:   "CustomerRow",
			CName:  "CustomerRow",
			Fields: []ir.FieldIR{{Name: "id", Type: ir.Type{Kind: ir.TypeI64, Name: "i64"}}},
		}},
		Funcs: []ir.FuncIR{{
			Name:       "rows_of",
			Params:     []ir.ParamIR{{Name: "p", Type: ir.Type{Kind: ir.TypePtr, Name: "Ptr[CustomerRow]", Args: []ir.Type{row}}}},
			ReturnType: ir.Type{Kind: ir.TypePtr, Name: "Ptr[CustomerRow]", Args: []ir.Type{row}},
			Body:       []ir.Instr{{Op: ir.OpReturn, Args: []string{"p"}}},
		}},
	}
	with := EmitGo(named)
	if !strings.Contains(with, `"unsafe"`) || !strings.Contains(with, "func rowsOf(p unsafe.Pointer) unsafe.Pointer") {
		t.Errorf("a module that names a pointer must spell it unsafe.Pointer and import it:\n%s", with)
	}

	plain := EmitGo(greetIR())
	if strings.Contains(plain, `"unsafe"`) {
		t.Error("a module using no pointer carries an unused import")
	}
}

// A single import stays on one line, which is the shape a reader expects in
// generated code and the shape the existing emission test asserts.
func TestASingleGoImportIsWrittenOnOneLine(t *testing.T) {
	if !strings.Contains(EmitGo(greetIR()), "\nimport \"fmt\"\n") {
		t.Error("a single import is not written on one line")
	}
}
