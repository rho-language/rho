//
// @file ffi_string_test.go
// @brief Passing text to a foreign function.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package types

import (
	"strings"
	"testing"

	"rho/src/ir"
)

// A string is passed where a C pointer is wanted, at a foreign call, for no
// runtime cost at all: a literal is already an address in the program's
// read-only data, and a string built at run time is the `const char*` that C
// strings are.
func TestAStringCoercesToAPointerAtAForeignCall(t *testing.T) {
	source := `module main

extern fn take_name(name: Ptr[u8]) i32

fn main() void {
    let a: i32 = take_name("user_led")
    let b: i32 = take_name("system_timer")
}
`
	if err := New().CheckModule(*parseSource(t, source)); err != nil {
		t.Fatalf("a string must be accepted where a C pointer is wanted: %v", err)
	}
}

// The same program is refused when the pointer is not to a byte. A pointer to a
// struct is not text, and a string is not a struct: letting it through would
// produce a C call that passes four bytes where a struct is expected.
func TestAStringIsRefusedWhereAPointerIsNotToBytes(t *testing.T) {
	source := `module main

struct Row { value: i32 }

extern fn take_row(row: Ptr[Row]) i32

fn main() void {
    let a: i32 = take_row("not a row")
}
`
	err := New().CheckModule(*parseSource(t, source))
	if err == nil {
		t.Fatal("expected a refusal: a string is not a struct pointer")
	}
	if !strings.Contains(err.Error(), "argument 1") {
		t.Errorf("the refusal should name the argument, got: %v", err)
	}
}

// Inside Rho a string is not a pointer. A Rho string knows its length and a C
// one does not, and a program that treated them as the same thing inside Rho
// would lose the difference that every C caller depends on.
func TestAStringIsNotAPointerOutsideAForeignCall(t *testing.T) {
	source := `module main

fn takes_pointer(p: Ptr[u8]) void {
}

fn main() void {
    takes_pointer("a literal")
}
`
	err := New().CheckModule(*parseSource(t, source))
	if err == nil {
		t.Fatal("expected a refusal: this is an ordinary Rho function, not a foreign one")
	}
}

// `Ptr[u8]` is a pointer in C. It was being mangled to the name `Ptr_u8`, which
// a C compiler has never heard of, so a binding generated for a C library
// declared functions taking a struct and nothing could call them.
func TestAPointerIsSpelledAsAPointerInC(t *testing.T) {
	tc := New()
	tc.types["CustomerRow"] = ir.Type{Kind: ir.TypeStruct, Name: "CustomerRow"}

	for _, written := range []string{"Ptr[u8]", "Ptr[CustomerRow]"} {
		if got := tc.cTypeForTypeName(written); !strings.HasSuffix(got, "*") {
			t.Errorf("%s should be a C pointer, got %q", written, got)
		}
	}

	// The same spelling on the type path, which is what a declaration is read
	// by. Two paths resolving the same written type differently is how a
	// language ends up with a rule that depends on where a type was mentioned.
	resolved := tc.declaredType("Ptr[u8]")
	if resolved.Kind != ir.TypePtr {
		t.Errorf("Ptr[u8] should resolve to a pointer type, got kind %v", resolved.Kind)
	}
}

// A pointer to nothing is a handle, and a handle is named rather than raw.
// `Ptr[Unit]` is how a foreign handle is held — a value that is only ever
// compared and passed on, and that C would spell `void*`. It stays a name here
// because a program has to be able to declare a field of that type and assign a
// null to it, which `void*` does not give it as cleanly as a name does.
func TestAPointerToNothingIsAHandle(t *testing.T) {
	got := New().cTypeForTypeName("Ptr[Unit]")
	if strings.HasSuffix(got, "*") {
		t.Errorf("Ptr[Unit] is a handle and is named rather than raw, got %q", got)
	}
}
