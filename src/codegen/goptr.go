//
// @file goptr.go
// @brief A Rho pointer in Go: one type, `unsafe.Pointer`, for every pointer.
//
// `Ptr[T]` is one IR type, and the Go backend spells it one way whatever it
// points at and wherever it was declared — a field, a parameter, the address of
// a variable, a foreign handle. `unsafe.Pointer` is that spelling because it is
// the one Go type that is both an address and a reference: it converts to and
// from `uintptr`, so `Ptr[T](n)` and `u64(p)` mean what they mean in C, and the
// collector follows it, so the memory a pointer refers to stays alive and moves
// with it — a variable whose address is taken is kept where Go can find it, and
// a stack that grows takes the pointer along.
//
// The lowered text the type checker writes is C, and C names the element in the
// cast it reads through: `((T*)(p))[i]`. That is the one access form, `*p` being
// index zero, and it is rewritten here into a typed access of the memory:
//
//	((uint32_t*)(p))[i]   ->   (*rhoPtrAt[uint32](p, int(i)))
//
// which is an lvalue in Go as `p[i]` is in C.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import "strings"

// isCPointerSpelling reports whether a C type spelling is a pointer that Go
// carries as `unsafe.Pointer`.
//
// A C string is spelled as a pointer too, `const char*`, and is a Go string; it
// is the one pointer spelling that is not a Rho pointer.
func isCPointerSpelling(cType string) bool {
	trimmed := strings.TrimSpace(cType)
	if !strings.HasSuffix(trimmed, "*") {
		return false
	}
	if goCasts[trimmed] == "string" {
		return false
	}
	return true
}

// isRhoPointerName reports whether a type as Rho writes it is a pointer.
func isRhoPointerName(name string) bool {
	trimmed := strings.TrimSpace(name)
	return strings.HasPrefix(trimmed, "Ptr[") && strings.HasSuffix(trimmed, "]")
}

// readCType reads a C type spelling between a '(' at open and its ')', as a
// cast writes one: words and then stars. It returns the spelling, normalised to
// `word word *`, and the index just past the ')'.
func readCType(src []byte, open int) (string, int, bool) {
	if open >= len(src) || src[open] != '(' {
		return "", 0, false
	}
	pos := skipSpaces(src, open+1)
	words := []string{}
	for pos < len(src) && isIdentStart(src[pos]) {
		end := scanIdent(src, pos)
		words = append(words, string(src[pos:end]))
		pos = skipSpaces(src, end)
	}
	if len(words) == 0 {
		return "", 0, false
	}
	stars := 0
	for pos < len(src) && src[pos] == '*' {
		stars++
		pos = skipSpaces(src, pos+1)
	}
	if pos >= len(src) || src[pos] != ')' {
		return "", 0, false
	}
	spelling := strings.Join(words, " ")
	if stars > 0 {
		spelling += " " + strings.Repeat("*", stars)
	}
	return spelling, pos + 1, true
}

// translatePointerAccess rewrites the one form a pointer is read and written
// through, `((T*)(p))[i]`, starting at its outer '('. It reports false for
// anything else, which the ordinary translation then handles.
func translatePointerAccess(src []byte, start int) (int, string, bool) {
	inner := skipSpaces(src, start+1)
	cType, afterCast, ok := readCType(src, inner)
	if !ok || !isCPointerSpelling(cType) {
		return 0, "", false
	}
	operandStart := skipSpaces(src, afterCast)
	operandEnd, ok := scanPrimaryExpr(src, operandStart)
	if !ok {
		return 0, "", false
	}
	closeOuter := skipSpaces(src, operandEnd)
	if closeOuter >= len(src) || src[closeOuter] != ')' || matchParen(src, start) != closeOuter {
		return 0, "", false
	}
	open := skipSpaces(src, closeOuter+1)
	if open >= len(src) || src[open] != '[' {
		return 0, "", false
	}
	close := matchBracket(src, open)
	if close < 0 {
		return 0, "", false
	}
	element := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(cType), "*"))
	goElement := goTypeForCName(element)
	if goElement == "" {
		// A pointer to nothing has no element to read.
		return 0, "", false
	}
	pointer := ToGoExpr(string(src[operandStart:operandEnd]))
	index := ToGoExpr(string(src[open+1 : close]))
	return close + 1, "(*rhoPtrAt[" + goElement + "](" + pointer + ", int(" + index + ")))", true
}
