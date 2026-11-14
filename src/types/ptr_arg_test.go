//
// @file ptr_arg_test.go
// @brief Passing a pointer field of a slice to a foreign function.
//
// A program holds a prepared statement, a step counter, and column value
// through a `Slice` that carries them by address: `Slice[SqlStmt]{ Ptr[SqlStmt](&stmt), 1 }`
// and then hands the storage to the SQL port with `stmt_slice.ptr`. The `.ptr`
// field is a `Ptr[T]`, and the foreign signature the SQL port declares names
// `Ptr[SqlStmt]`: one pointer type, so the field passes straight through.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"strings"
	"testing"
)

// The `.ptr` field of a slice is a `Ptr[T]`, and a foreign function takes the
// same pointer type, so a prepared-statement program passes the field straight
// through to the port.
func TestAPointerFieldIsAcceptedByAForeignCall(t *testing.T) {
	source := `module main

struct SqlDb { raw: u64 }
struct SqlStmt { raw: u64 }
struct SqlValue { kind: i32 }

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

// A pointer constructed in a program is the pointer type, spelled in C as a
// pointer to the element's C type. There is no struct to build: `Ptr[u32](&more)`
// is the address `&more` already is, and a field of a slice holds it as it is.
func TestAPointerConstructorIsTheAddressItself(t *testing.T) {
	source := `module main

struct Slice[T] { ptr: Ptr[T], len: u32 }

fn take(more: u32) Slice[u32] {
    return Slice[u32]{ Ptr[u32](&more), 1 }
}
`
	fn := lowerFunctionToIR(t, source, "take")
	text := instrText(fn)
	if strings.Contains(text, "Ptr_") {
		t.Errorf("a pointer is not a struct, and no Ptr_ name should appear:\n%s", text)
	}
	if !strings.Contains(text, "(Slice_u32){ ((uint32_t*)(&(more))), 1 }") {
		t.Errorf("expected the slice to hold the address of more, got:\n%s", text)
	}
}

// The same boundary is not relaxed into accepting a non-pointer value: passing a
// bare integer where the port wants a pointer is still a refusal, because the
// coercion is a pointer-to-pointer one and nothing else.
func TestANonPointerIsStillRefusedAtAForeignCall(t *testing.T) {
	source := `module main

struct SqlDb { raw: u64 }
struct SqlStmt { raw: u64 }

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

// A pointer is made from an address or another pointer. A string, a struct or
// a float is neither, and building a pointer from one is refused rather than
// lowered into a cast that reads its bytes as an address.
func TestAPointerIsMadeOnlyFromAnAddressOrAPointer(t *testing.T) {
	refused := []string{`"text"`, `1.5`, `true`}
	for _, operand := range refused {
		source := "module main\n\nfn make() Ptr[u8] {\n    return Ptr[u8](" + operand + ")\n}\n"
		if err := New().CheckModule(*parseSource(t, source)); err == nil {
			t.Errorf("Ptr[u8](%s) should be refused", operand)
		}
	}
	accepted := `module main

fn make(n: u64, other: Ptr[u32]) Ptr[u8] {
    let a: Ptr[u8] = Ptr[u8](n)
    let b: Ptr[u8] = Ptr[u8](other)
    return Ptr[u8](0)
}
`
	if err := New().CheckModule(*parseSource(t, accepted)); err != nil {
		t.Errorf("a pointer from an address, a pointer or zero should be accepted: %v", err)
	}
}

// Every pointer operation lowers to a form both backends read: the casts name
// the element, so C reads them as what the value already is and Go knows what
// to read through an unsafe.Pointer.
func TestPointerOperationsLowerToOneForm(t *testing.T) {
	source := `module main

struct Point { x: u32, y: u32 }

fn ops(n: u64, p: Ptr[u32], pt: Ptr[Point]) u64 {
    var v: u32 = 3
    let a: Ptr[u32] = Ptr[u32](n)
    let z: Ptr[u32] = Ptr[u32](0)
    let r: Ptr[u32] = &v
    let o: Ptr[u32] = p + 2
    p[1] = *r
    pt[0].x = 4
    return u64(a) + u64(z) + u64(o)
}
`
	text := instrText(lowerFunctionToIR(t, source, "ops"))
	for _, want := range []string{
		"((uint32_t*)(uintptr_t)(n))",
		"((uint32_t*)NULL)",
		"((uint32_t*)(&(v)))",
		"((uint32_t*)(&((uint32_t*)(p))[2]))",
		"((uint32_t*)(p))[1]",
		"((uint32_t*)(r))[0]",
		"((Point*)(pt))[0].x",
		"((uint64_t)(uintptr_t)(a))",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %s in the lowered body:\n%s", want, text)
		}
	}
}
