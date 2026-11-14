//
// @file checked.go
// @brief Integer operations with one meaning on every backend.
//
// C leaves signed overflow, a shift by the width of the type or more, a division
// by zero and an index past the end undefined; Go defines each of them, and not
// always as a panic. Rho chooses one meaning and both backends implement it:
//
//   - signed `+`, `-`, `*` and unary `-` wrap, two's complement;
//   - a shift by a count the type is not wide enough for gives 0, or the sign for
//     a signed right shift; a constant count that large is refused;
//   - a division or remainder by zero, and an index past the end of a fixed
//     array or of a `Slice[T]` read through its `ptr`, stop the program with
//     the same message on both backends.
//
// Each operation that needs it is lowered to a helper named for the operation
// and the type (`rho_add_i32`, `rho_shl_u8`, `rho_div_u32`, `rho_index`). The C
// runtime defines them `static inline` beside the program, so a constant operand
// in range folds to the plain operator; the Go backend rewrites them to Go's own
// operator where Go already means the same thing, and to a runtime function
// where it does not.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"fmt"
	"math/big"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

// integerSuffix is the suffix a helper carries for an integer kind, or "" for a
// kind that is not one (bool included).
func integerSuffix(kind ir.TypeKind) string {
	switch kind {
	case ir.TypeI8:
		return "i8"
	case ir.TypeI16:
		return "i16"
	case ir.TypeI32:
		return "i32"
	case ir.TypeI64:
		return "i64"
	case ir.TypeU8:
		return "u8"
	case ir.TypeU16:
		return "u16"
	case ir.TypeU32:
		return "u32"
	case ir.TypeU64:
		return "u64"
	}
	return ""
}

// integerBits is the width of an integer kind, or 0.
func integerBits(kind ir.TypeKind) int {
	switch kind {
	case ir.TypeI8, ir.TypeU8:
		return 8
	case ir.TypeI16, ir.TypeU16:
		return 16
	case ir.TypeI32, ir.TypeU32:
		return 32
	case ir.TypeI64, ir.TypeU64:
		return 64
	}
	return 0
}

func signedKind(kind ir.TypeKind) bool {
	return kind == ir.TypeI8 || kind == ir.TypeI16 || kind == ir.TypeI32 || kind == ir.TypeI64
}

// integerRange is the smallest and largest value of an integer kind.
func integerRange(kind ir.TypeKind) (*big.Int, *big.Int, bool) {
	bits := integerBits(kind)
	if bits == 0 {
		return nil, nil, false
	}
	one := big.NewInt(1)
	if signedKind(kind) {
		max := new(big.Int).Sub(new(big.Int).Lsh(one, uint(bits-1)), one)
		min := new(big.Int).Neg(new(big.Int).Lsh(one, uint(bits-1)))
		return min, max, true
	}
	return big.NewInt(0), new(big.Int).Sub(new(big.Int).Lsh(one, uint(bits)), one), true
}

// checkedHelper is the helper an integer operation of a kind is lowered to, or
// "" when the C operator already means what Rho means: unsigned arithmetic at
// 32 bits and wider wraps in C as it does in Go, and C computes a narrower
// unsigned value in `int`, which neither wraps at the type's width nor is safe
// from signed overflow in a product.
func checkedHelper(op string, kind ir.TypeKind) string {
	suffix := integerSuffix(kind)
	if suffix == "" {
		return ""
	}
	var name string
	switch op {
	case "+", "-", "*":
		if !signedKind(kind) && integerBits(kind) >= 32 {
			return ""
		}
		name = map[string]string{"+": "add", "-": "sub", "*": "mul"}[op]
	case "/":
		name = "div"
	case "%":
		name = "mod"
	case "<<":
		name = "shl"
	case ">>":
		name = "shr"
	default:
		return ""
	}
	return "rho_" + name + "_" + suffix
}

// constIntValue is the value of an untyped integer constant expression, when it
// is one: a literal, or literals combined. A literal with a fraction is not.
func constIntValue(expr ast.Expr) (*big.Int, bool) {
	switch e := expr.(type) {
	case ast.NumberExpr:
		digits := strings.ReplaceAll(e.Value, "_", "")
		lower := strings.ToLower(digits)
		if strings.Contains(digits, ".") || (!strings.HasPrefix(lower, "0x") && strings.ContainsAny(lower, "e")) {
			return nil, false
		}
		value, ok := new(big.Int).SetString(digits, 0)
		return value, ok
	case ast.UnaryExpr:
		if e.Op != "-" {
			return nil, false
		}
		value, ok := constIntValue(e.Value)
		if !ok {
			return nil, false
		}
		return new(big.Int).Neg(value), true
	case ast.BinaryExpr:
		left, ok := constIntValue(e.Left)
		if !ok {
			return nil, false
		}
		right, ok := constIntValue(e.Right)
		if !ok {
			return nil, false
		}
		switch e.Op {
		case "+":
			return new(big.Int).Add(left, right), true
		case "-":
			return new(big.Int).Sub(left, right), true
		case "*":
			return new(big.Int).Mul(left, right), true
		case "/":
			if right.Sign() == 0 {
				return nil, false
			}
			return new(big.Int).Quo(left, right), true
		case "%":
			if right.Sign() == 0 {
				return nil, false
			}
			return new(big.Int).Rem(left, right), true
		case "<<":
			if right.Sign() < 0 || right.Cmp(big.NewInt(1024)) > 0 {
				return nil, false
			}
			return new(big.Int).Lsh(left, uint(right.Int64())), true
		case ">>":
			if right.Sign() < 0 || right.Cmp(big.NewInt(1024)) > 0 {
				return nil, false
			}
			return new(big.Int).Rsh(left, uint(right.Int64())), true
		case "&":
			return new(big.Int).And(left, right), true
		case "|":
			return new(big.Int).Or(left, right), true
		case "^":
			return new(big.Int).Xor(left, right), true
		}
	}
	return nil, false
}

// checkConstantFits refuses an untyped integer constant that the integer type it
// becomes cannot hold: `u32(7 / -2)`, `let b: u8 = 300`. C would wrap it and Go
// refuses to build, so one program would be two; it is refused on both.
func checkConstantFits(expr ast.Expr, target ir.Type) error {
	if !isUntypedConstant(expr) {
		return nil
	}
	value, ok := constIntValue(expr)
	if !ok {
		return nil
	}
	min, max, ok := integerRange(target.Kind)
	if !ok {
		return nil
	}
	if value.Cmp(min) < 0 || value.Cmp(max) > 0 {
		return locate(fmt.Errorf("constant %s overflows %s", value.String(), integerSuffix(target.Kind)), expr.Position(), "")
	}
	return nil
}

// checkConstantOperands refuses what a constant operand makes certain to go
// wrong: a division or remainder by a constant zero, and a shift by a constant
// count that is negative or at least the width of the type shifted.
func checkConstantOperands(expr ast.BinaryExpr, shifted ir.Type) error {
	switch expr.Op {
	case "/", "%":
		if value, ok := constIntValue(expr.Right); ok && value.Sign() == 0 {
			return locate(fmt.Errorf("division by zero"), expr.Position(), "")
		}
	case "<<", ">>":
		if isUntypedConstant(expr.Left) {
			return nil
		}
		value, ok := constIntValue(expr.Right)
		if !ok {
			return nil
		}
		if value.Sign() < 0 {
			return locate(fmt.Errorf("negative shift count %s", value.String()), expr.Position(), "")
		}
		if bits := integerBits(shifted.Kind); bits > 0 && value.Cmp(big.NewInt(int64(bits))) >= 0 {
			return locate(fmt.Errorf("shift count %s overflows type %s", value.String(), integerSuffix(shifted.Kind)), expr.Position(), "")
		}
	}
	return nil
}

// checkConstantIndex refuses a constant index outside a fixed array.
func checkConstantIndex(expr ast.IndexExpr, object ir.Type) error {
	length, fixed := fixedArrayLength(object)
	if !fixed {
		return nil
	}
	value, ok := constIntValue(expr.Index)
	if !ok {
		return nil
	}
	if value.Sign() < 0 || value.Cmp(big.NewInt(int64(length))) >= 0 {
		return locate(fmt.Errorf("index %s out of range [0,%d)", value.String(), length), expr.Position(), "")
	}
	return nil
}

// isSliceStruct reports whether a type is `Slice[T]` from std.core.slice (or an
// alias of one, `String`), whose `len` bounds what its `ptr` may be indexed by.
func isSliceStruct(typ ir.Type) bool {
	if typ.Kind != ir.TypeStruct {
		return false
	}
	return strings.HasPrefix(typ.Name, "Slice[") || strings.HasPrefix(typ.Name, "Slice_") || typ.Name == "String"
}

// pureOperand reports whether an expression can be evaluated twice with nothing
// changing: a name, or fields of one. A bound check reads the slice a second
// time, so it is only written where that read cannot differ.
func pureOperand(expr ast.Expr) bool {
	switch e := expr.(type) {
	case ast.IdentifierExpr:
		return true
	case ast.MemberExpr:
		return pureOperand(e.Object)
	}
	return false
}

// checkedIndex wraps a lowered index in the bound check its object has, when it
// has one: the length of a fixed array, or the `len` of the `Slice[T]` whose
// `ptr` is indexed. A constant index into a fixed array was refused at compile
// time when it is out of range, and is left as written. A bare pointer — `[T]`
// or a `Ptr[T]` of the program's own — carries no length and is not checked.
func (tc *TypeChecker) checkedIndex(expr ast.IndexExpr, object ir.Type, index string, scope map[string]ir.Type) string {
	if length, fixed := fixedArrayLength(object); fixed {
		if _, constant := constIntValue(expr.Index); constant {
			return index
		}
		return fmt.Sprintf("rho_index(%s, %d)", index, length)
	}
	if object.Kind != ir.TypePtr {
		return index
	}
	member, ok := expr.Object.(ast.MemberExpr)
	if !ok || member.Name != "ptr" || !pureOperand(member.Object) {
		return index
	}
	owner, err := tc.exprType(member.Object, scope)
	if err != nil || !isSliceStruct(owner) {
		return index
	}
	return "rho_index(" + index + ", " + tc.lowerExpr(member.Object, scope) + ".len)"
}
