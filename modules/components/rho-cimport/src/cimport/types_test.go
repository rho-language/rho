//
// @file types_test.go
// @brief Tests for mapping C types onto Rho's.
//
// The mapping is where a binding is either typed or a pile of opaque pointers, so
// these tests are about the decisions: what a primitive is, how far an alias is
// followed, what a pointer to a library structure is, and what an enumeration is
// when the language has no enum to name it with.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cimport

import (
	"strings"
	"testing"
)

// mapperFor builds a mapper from header text.
func mapperFor(t *testing.T, text string) *TypeMapper {
	t.Helper()
	return NewTypeMapper(header(t, text))
}

func TestAPrimitiveMapsToItsRhoType(t *testing.T) {
	mapper := mapperFor(t, "typedef struct S { int a; } S;\n")
	for cType, want := range map[string]string{
		"uint8_t":  "u8",
		"uint32_t": "u32",
		"int32_t":  "i32",
		"double":   "f64",
		"bool":     "bool",
		"void":     "Unit",
	} {
		if got, ok := mapper.TryRho(cType); !ok || got != want {
			t.Errorf("expected %s to map to %s, got %s (%v)", cType, want, got, ok)
		}
	}
}

func TestAnAliasIsResolvedToWhatItFinallyIs(t *testing.T) {
	// A portable interface is written in its own vocabulary, and the mapper has to
	// follow it: rh_size is a rh_u32, which is a uint32_t, which is a u32.
	mapper := mapperFor(t, `typedef uint32_t rh_u32;
typedef rh_u32 rh_size;
typedef struct S { int a; } S;
`)
	if got, ok := mapper.TryRho("rh_size"); !ok || got != "u32" {
		t.Fatalf("expected rh_size to resolve to u32, got %s (%v)", got, ok)
	}
}

func TestAPointerToAPrimitiveIsATypedPointer(t *testing.T) {
	mapper := mapperFor(t, "typedef struct S { int a; } S;\n")
	for cType, want := range map[string]string{
		"uint8_t*":    "Ptr[u8]",
		"const char*": "Ptr[u8]",
		"rh_u32*":     "Ptr[Unit]",
	} {
		got, _ := mapper.TryRho(cType)
		if got != want {
			t.Errorf("expected %s to map to %s, got %s", cType, want, got)
		}
	}
}

func TestAPointerToALibraryStructureIsNamed(t *testing.T) {
	// The structure is the library's, so the name is the library's: the C the
	// backend writes resolves against the header the interface includes.
	mapper := mapperFor(t, "typedef struct DeviceSPI { int a; } DeviceSPI;\n")
	if got, _ := mapper.TryRho("DeviceSPI*"); got != "Ptr[DeviceSPI]" {
		t.Fatalf("expected the library's type to be named, got %s", got)
	}
}

func TestAnEnumerationIsTheWholeNumberItIs(t *testing.T) {
	// A C enum is a whole number, and a Rho enum is not a language declaration
	// yet, so the integer is what a signature can carry. The names are carried as
	// documentation by the emitter.
	mapper := mapperFor(t, "typedef enum { A = 0, B = 1 } Kind;\ntypedef struct S { int a; } S;\n")
	if got, ok := mapper.TryRho("Kind"); !ok || got != "i32" {
		t.Fatalf("expected an enumeration to map to i32, got %s (%v)", got, ok)
	}
}

func TestATypeTheMapperDoesNotKnowIsReported(t *testing.T) {
	// A type the interface never declared has no Rho spelling, and saying so is
	// better than inventing one: the caller learns the binding is incomplete.
	mapper := mapperFor(t, "typedef struct S { int a; } S;\n")
	// Rho, not TryRho: the recording path is the one a generator uses, and it is
	// what turns an unmapped type into a diagnostic rather than an opaque pointer
	// nobody hears about.
	if got := mapper.Rho("SomethingElse"); got != "Ptr[Unit]" {
		t.Fatalf("expected the opaque fallback, got %s", got)
	}
	diagnostics := mapper.Diagnostics("lib.h")
	if len(diagnostics) == 0 {
		t.Fatal("expected the unknown type to be reported")
	}
	if !strings.Contains(diagnostics[0].Message, "SomethingElse") {
		t.Fatalf("expected the type to be named, got %q", diagnostics[0].Message)
	}
}

func TestUnitIsNotAPointerToALibraryType(t *testing.T) {
	// `Ptr[Unit]` is how an opaque foreign handle is written, and the C backend
	// has to spell it as a pointer to nothing rather than a mangled name.
	mapper := mapperFor(t, "typedef struct S { int a; } S;\n")
	if got, _ := mapper.TryRho("void*"); got != "Ptr[Unit]" {
		t.Fatalf("expected void* to be an opaque pointer, got %s", got)
	}
}
