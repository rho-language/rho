//
// @file pointer.go
// @brief How a pointer is lowered: one type, one spelling per backend.
//
// `Ptr[T]` is a single IR type, `ir.TypePtr`, wherever it is written: a field, a
// parameter, a local, the result of `&`, a foreign signature. The lowered text is
// the C the C backend compiles and the Go backend translates, so every operation
// on a pointer is written in a form both can read:
//
//	Ptr[T](n)      ((T*)(uintptr_t)(n))     an address held as a number
//	Ptr[T](0)      ((T*)NULL)               the null pointer
//	Ptr[T](p)      ((T*)(p))                another pointer, retyped
//	u64(p)         ((uint64_t)(uintptr_t)(p))
//	&x             ((T*)(&(x)))
//	p[i]           ((T*)(p))[i]
//	*p             ((T*)(p))[0]
//	p + n          ((T*)(&((T*)(p))[n]))    the address n elements on
//
// In C the casts are what the value already is, so `((T*)(p))[i]` is `p[i]`. The
// Go backend has no element type to read off a name — it spells every pointer as
// `unsafe.Pointer` — so the cast carries it: `((T*)(p))[i]` is the one form it
// rewrites into a typed access of the memory `p` refers to.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"rho/src/frontend/ast"
	"rho/src/ir"
)

// pointerCType is the C spelling of a pointer type, `T*`.
func (tc *TypeChecker) pointerCType(pointer ir.Type) string {
	return tc.cTypeFor(pointer)
}

// pointerElementAt is the element `index` places after the one `object` points
// at, as an lvalue: `((T*)(p))[i]`.
func (tc *TypeChecker) pointerElementAt(pointer ir.Type, object, index string) string {
	return "((" + tc.pointerCType(pointer) + ")(" + object + "))[" + index + "]"
}

// lowerPointerConversion lowers a conversion whose target or operand is a
// pointer, and reports whether it was one.
//
// A pointer converts to and from the integer an address is. The integer is
// `uintptr_t` in between, because that is the integer both C and Go convert a
// pointer to: a `uint64_t` cast straight to a pointer is a warning on a 32-bit
// target, and Go converts only `uintptr` to `unsafe.Pointer`.
func (tc *TypeChecker) lowerPointerConversion(target ir.Type, operand ast.Expr, scope map[string]ir.Type) (string, bool) {
	source, err := tc.exprType(operand, scope)
	if err != nil {
		source = ir.Type{}
	}
	value := tc.lowerExpr(operand, scope)
	if target.Kind == ir.TypePtr {
		cType := tc.pointerCType(target)
		if source.Kind == ir.TypePtr {
			// The same pointer needs no cast: `Ptr[u8](&buf[0])` is the
			// address as `&` already wrote it.
			if tc.pointerCType(source) == cType {
				return value, true
			}
			return "((" + cType + ")(" + value + "))", true
		}
		// A written zero is the null pointer, which both backends have a word
		// for: C's NULL and Go's nil.
		if number, ok := operand.(ast.NumberExpr); ok && isZeroLiteral(number.Value) {
			return "((" + cType + ")NULL)", true
		}
		return "((" + cType + ")(uintptr_t)(" + value + "))", true
	}
	if source.Kind == ir.TypePtr && numeric(target) {
		return "((" + tc.cTypeFor(target) + ")(uintptr_t)(" + value + "))", true
	}
	return "", false
}

// lowerPointerUnary lowers `&x` and `*p`, and reports whether the expression
// was one of them.
//
// `&x` is cast to the pointer it is, so the Go backend — which spells every
// pointer as `unsafe.Pointer` — converts the address once, where it is taken.
// `*p` is the element at index zero, the one access form both backends share.
func (tc *TypeChecker) lowerPointerUnary(expr ast.UnaryExpr, scope map[string]ir.Type) (string, bool) {
	switch expr.Op {
	case "&":
		inner, err := tc.exprType(expr.Value, scope)
		if err != nil || isFixedArray(inner) {
			// An array's address is a pointer to the array, which C cannot
			// spell as `T[N]*`; it is left as the address it is.
			return "", false
		}
		pointer := ir.Type{Kind: ir.TypePtr, Name: "Ptr[" + typeName(inner) + "]", Args: []ir.Type{inner}}
		return "((" + tc.pointerCType(pointer) + ")(&(" + tc.lowerExpr(expr.Value, scope) + ")))", true
	case "*":
		pointer, err := tc.exprType(expr.Value, scope)
		if err != nil || pointer.Kind != ir.TypePtr || len(pointer.Args) != 1 {
			return "", false
		}
		return tc.pointerElementAt(pointer, tc.lowerExpr(expr.Value, scope), "0"), true
	}
	return "", false
}

// lowerPointerOffset lowers `p + n` and `p - n` for a pointer p, and reports
// whether the expression was one: the address of the element n places on, which
// is what C's pointer arithmetic means and what Go has to compute in bytes.
func (tc *TypeChecker) lowerPointerOffset(expr ast.BinaryExpr, scope map[string]ir.Type) (string, bool) {
	if expr.Op != "+" && expr.Op != "-" {
		return "", false
	}
	pointer, err := tc.exprType(expr.Left, scope)
	if err != nil || pointer.Kind != ir.TypePtr || len(pointer.Args) != 1 {
		return "", false
	}
	offset := tc.lowerExpr(expr.Right, scope)
	if expr.Op == "-" {
		offset = "-(" + offset + ")"
	}
	element := tc.pointerElementAt(pointer, tc.lowerExpr(expr.Left, scope), offset)
	return "((" + tc.pointerCType(pointer) + ")(&" + element + "))", true
}

// isFixedArray reports whether a type is a fixed-size array.
func isFixedArray(typ ir.Type) bool {
	if typ.ArrayLen > 0 {
		return true
	}
	_, _, ok := nameArrayDims(typ.Name)
	return ok
}

// isZeroLiteral reports whether a written number is zero.
func isZeroLiteral(text string) bool {
	switch text {
	case "0", "0x0", "0X0", "0b0", "0o0":
		return true
	}
	return false
}
