//
// @file npo.go
// @brief An optional pointer, which costs nothing.
//
// A value that may be absent is normally a tag and a payload: a flag saying
// whether there is a value, and the value itself. For a pointer that is a waste.
// A null pointer already says "there is no value", and every C library that
// returns a pointer already uses null for it — `robohub_get_gpio` answers NULL
// for a name it does not know, and the program has to be able to see that.
//
// So `Option[Ptr[T]]` is stored as the pointer. `None` is `NULL` and `Some(p)` is
// `p`, and the two are the same size and the same representation. Nothing is
// tagged, nothing is allocated, and the value crosses into C and back as itself.
//
// This is not a trick to save a word. It is what makes the boundary honest: a
// nullable C pointer becomes an optional Rho pointer without a translation, and
// the representation on the far side of that boundary is the one C already
// agreed to.
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

	"rho/src/frontend/ast"
	"rho/src/ir"
)

// npoPrefix is how a specialisation of `Option[Ptr[...]]` is mangled.
//
// The mangled name is what a back end is handed, so this is read off the name
// rather than re-deriving the specialisation — the same string the rest of the
// pipeline already agrees on.
const npoPrefix = "Option_Ptr_"

// IsOptionalPointer reports whether a mangled type name is `Option[Ptr[T]]`.
//
// It answers for a name rather than a resolved type on purpose. By the time a
// backend holds a type, `Option[Ptr[T]]` has been mangled and the pointer inside
// it is a name as well; re-resolving would be a second reading of the same
// decision, and the two would eventually disagree.
func IsOptionalPointer(name string) bool {
	return strings.HasPrefix(name, npoPrefix) && len(name) > len(npoPrefix)
}

// npoPayload is the element an `Option[Ptr[T]]` carries: the `T` in the name
// after `Option_Ptr_`.
func npoPayload(name string) string {
	return name[len(npoPrefix):]
}

// optionalPointerPayload returns the C type an optional pointer is represented
// as, or "" when the name is not one.
//
// `Option[RoboHubGPIO_Ptr]` carries `RoboHubGPIO`, so the representation is
// `RoboHubGPIO*`. A nested `Ptr` keeps its brackets, which is what a C type
// wants and what a mangled name no longer spells.
func optionalPointerPayload(name string) string {
	if !IsOptionalPointer(name) {
		return ""
	}
	return npoPayload(name)
}

// lowerOptionalPointerLiteral emits the pointer an `Option[Ptr[T]]` literal
// stands for.
//
// The literal is `{ is_some, value }` — the two fields the general Option has —
// so the presence is the first field and the value is the second. A present
// value is the pointer itself, which is what makes the whole construction free.
func (tc *TypeChecker) lowerOptionalPointerLiteral(expr ast.StructLiteralExpr, name string, scope map[string]ir.Type) string {
	if len(expr.Values) == 0 {
		return "NULL"
	}
	present := tc.lowerExpr(expr.Values[0], scope)
	if len(expr.Values) < 2 {
		return "NULL"
	}
	value := tc.lowerExpr(expr.Values[1], scope)
	// `false` is the only thing that is absent, and it is the only thing that
	// could be: the field is a bool written by `None` or by `Some`.
	if present == "false" || present == "0" {
		return "NULL"
	}
	return value
}
