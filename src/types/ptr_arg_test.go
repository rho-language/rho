//
// @file ptr_arg_test.go
// @brief Passing a pointer field of a slice to a foreign function.
//
// A program holds a prepared statement, a step counter, and column value
// through a `Slice` that carries them by address: `Slice[SqlStmt]{ Ptr[SqlStmt](&stmt), 1 }`
// and then hands the storage to the SQL port with `stmt_slice.ptr`. The `.ptr`
// field is the pointer `Ptr[T]` that std.core declares as a struct, while the
// foreign signature the SQL port declares names the same pointer as `Ptr[SqlStmt]`.
// The two spellings are one type, and a call that crosses that boundary has to
// accept them as the same.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package types

import (
	"strings"
	"testing"
)

// The `.ptr` field of a slice is the pointer `Ptr[T]` std.core declares, and a
// foreign function takes that same pointer under its other spelling, `Ptr[T]` as
// a C pointer. The two are one type, and a prepared-statement program passes the
// field straight through to the port.
func TestAPointerFieldIsAcceptedByAForeignCall(t *testing.T) {
	source := `module main

struct SqlDb { raw: u64 }
struct SqlStmt { raw: u64 }
struct SqlValue { kind: i32 }

struct Ptr[T] { raw: u64 }
struct Slice[T] { ptr: Ptr[T], len: u32 }

extern fn sql_prepare(db: SqlDb, sql: string, stmt: Ptr[SqlStmt]) u32
extern fn sql_run(stmt: SqlStmt, more: Ptr[u32]) u32
extern fn sql_read_column(stmt: SqlStmt, index: u32, value: Ptr[SqlValue]) u32

fn list() u32 {
    var stmt: SqlStmt
    var more: u32 = 1
    var v0: SqlValue
    let stmt_slice: Slice[SqlStmt] = Slice[SqlStmt]{ Ptr[SqlStmt](&stmt), 1 }
    let more_slice: Slice[u32] = Slice[u32]{ Ptr[u32](&more), 1 }
    let v0_slice: Slice[SqlValue] = Slice[SqlValue]{ Ptr[SqlValue](&v0), 1 }
    let prep: u32 = sql_prepare(SqlDb{0}, "INSERT", stmt_slice.ptr)
    let ok: u32 = sql_run(stmt, more_slice.ptr)
    let col: u32 = sql_read_column(stmt, 0, v0_slice.ptr)
    return prep + ok + col
}
`
	if err := New().CheckModule(*parseSource(t, source)); err != nil {
		t.Fatalf("a pointer field must satisfy a foreign call's pointer parameter: %v", err)
	}
}

// A pointer constructed in a program is mangled under the element type as Rho
// spells it — `Ptr[u32]` becomes `Ptr_u32` — and the struct the same pointer is
// declared with is mangled the same way. Spelling the element as the C backend
// does (`uint32_t`) built `Ptr_uint32_t` for the constructor while the field
// was declared `PtrU32`, so the conversion named a struct nothing declared.
func TestAPointerConstructorUsesTheRhoSpellingOfItsElement(t *testing.T) {
	source := `module main

struct Ptr[T] { raw: u64 }
struct Slice[T] { ptr: Ptr[T], len: u32 }

fn take(more: u32) Slice[u32] {
    return Slice[u32]{ Ptr[u32](&more), 1 }
}
`
	fn := lowerFunctionToIR(t, source, "take")
	text := instrText(fn)
	if strings.Contains(text, "Ptr_uint32_t") {
		t.Errorf("the pointer constructor must not use the C element spelling:\n%s", text)
	}
	if !strings.Contains(text, "Ptr_u32){ &") {
		t.Errorf("expected the conversion to build Ptr_u32, got:\n%s", text)
	}
}

// The same boundary is not relaxed into accepting a non-pointer value: passing a
// bare integer where the port wants a pointer is still a refusal, because the
// coercion is a pointer-to-pointer one and nothing else.
func TestANonPointerIsStillRefusedAtAForeignCall(t *testing.T) {
	source := `module main

struct SqlDb { raw: u64 }
struct SqlStmt { raw: u64 }

struct Ptr[T] { raw: u64 }
struct Slice[T] { ptr: Ptr[T], len: u32 }

extern fn sql_prepare(db: SqlDb, sql: string, stmt: Ptr[SqlStmt]) u32

fn list() u32 {
    var more: u32 = 1
    return sql_prepare(SqlDb{0}, "INSERT", more)
}
`
	err := New().CheckModule(*parseSource(t, source))
	if err == nil {
		t.Fatal("expected a refusal: an integer is not a pointer")
	}
	if !strings.Contains(err.Error(), "argument 3") {
		t.Errorf("the refusal should name the argument, got: %v", err)
	}
}
