//
// @file types_test.go
// @brief Tests for Rho Type System - Enterprise v1.
//
// Validates:
//   - primitive types
//   - named types
//   - function types
//   - type names
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package analysis

import (
	"testing"
)

func TestPrimitiveType(t *testing.T) {
	tp := NewPrimitiveType("int")

	if tp.Name() != "int" {
		t.Fatalf("expected 'int', got %s", tp.Name())
	}
}

func TestNamedType(t *testing.T) {
	tp := NewNamedType("MyStruct")

	if tp.Name() != "MyStruct" {
		t.Fatalf("expected 'MyStruct', got %s", tp.Name())
	}
}

func TestFunctionType(t *testing.T) {
	tp := NewFunctionType(
		[]Type{TypeInt, TypeBool},
		TypeVoid,
	)

	if tp.Name() != "fn(int, bool) -> void" {
		t.Fatalf("unexpected function type name: %s", tp.Name())
	}
}

func TestBuiltinTypes(t *testing.T) {
	if TypeInt.Name() != "int" {
		t.Fatalf("builtin int type mismatch")
	}
	if TypeVoid.Name() != "void" {
		t.Fatalf("builtin void type mismatch")
	}
}
