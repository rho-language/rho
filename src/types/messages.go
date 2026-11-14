//
// @file messages.go
// @brief What the checker says when a program is wrong, and what to do about it.
//
// A message names the mistake and the way out of it. "cannot assign string to
// u32" is true and leaves the reader to work out which of three fixes they want;
// "cannot use a string as u32 in `let y`: parse it with int_from_str(...), or
// declare y as string" is the same fact with the fixes in it. The sentences that
// recur — a type mismatch, a name nothing declares — are built here so that they
// read the same wherever they are raised.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package types

import (
	"fmt"
	"sort"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/ir"
)

// writtenCallee is a call's target as the program wrote it.
func writtenCallee(callee ast.Expr) string {
	switch c := callee.(type) {
	case ast.IdentifierExpr:
		return c.Name
	case ast.MemberExpr:
		return writtenCallee(c.Object) + "." + c.Name
	}
	return "the function"
}

// plural is a count of something, in words that agree with it.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// signatureHint spells the parameters a call has to supply.
func signatureHint(signature functionType) string {
	if len(signature.params) == 0 {
		return "call it with no arguments"
	}
	names := make([]string, len(signature.params))
	for i, param := range signature.params {
		names[i] = typeName(param)
	}
	return "it is called with (" + strings.Join(names, ", ") + ")"
}

// substitutionText is a generic's substitution, as `T = u32, U = string`.
func substitutionText(subs map[string]ir.Type) string {
	names := make([]string, 0, len(subs))
	for name := range subs {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = name + " = " + typeName(subs[name])
	}
	return strings.Join(parts, ", ")
}

// mismatch is the error for a value of one type where another is wanted.
func mismatch(got, want ir.Type, where string) error {
	return fmt.Errorf("cannot use %s as %s %s; %s", article(typeName(got)), typeName(want), where, conversionHint(got, want))
}

// article puts "a" or "an" before a type name, as a sentence reads it.
func article(name string) string {
	if name == "" {
		return "a value"
	}
	// A primitive is read by its letter: "a u32", "an i32", "an f64".
	if len(name) > 1 && name[1] >= '0' && name[1] <= '9' {
		if name[0] == 'u' {
			return "a " + name
		}
		return "an " + name
	}
	switch name[0] {
	case 'a', 'e', 'i', 'o', 'A', 'E', 'I', 'O':
		return "an " + name
	}
	return "a " + name
}

// conversionHint says how a value of one type becomes another, when there is a
// way, and otherwise that one of the two has to change.
func conversionHint(got, want ir.Type) string {
	wantName := typeName(want)
	switch {
	case want.Enum && got.Enum:
		return fmt.Sprintf("these are two different enums; convert through the number, as in %s(u32(v)), if that is what is meant", wantName)
	case want.Enum && numeric(got):
		return fmt.Sprintf("a number is not %s; convert it with %s(n), which stops the program if n is none of its values", article(wantName), wantName)
	case got.Enum && numeric(want):
		return fmt.Sprintf("an enum is not a number; write %s(v) to use its number", wantName)
	case got.Kind == ir.TypeString && integerBits(want.Kind) > 0:
		return "parse it with int_from_str(...) or i64_from_str(...) and convert the result, or change the declared type"
	case integerBits(got.Kind) > 0 && want.Kind == ir.TypeString:
		return "format it with int_to_str(...) or i64_to_str(...), or change the declared type"
	case want.Kind == ir.TypeBool && numeric(got):
		return "a number is not a condition; compare it, for example `n != 0`"
	case numeric(got) && numeric(want):
		return fmt.Sprintf("convert it explicitly with %s(...), or change the declared type", wantName)
	case got.Kind == ir.TypeVoid:
		return "the expression produces no value; call a function that returns one"
	}
	return "change the value, or the declared type, so the two agree"
}

// undefinedNameMessage is what to say about a name nothing declares: that, and
// the nearest name that is declared, when one is close enough to be a typo.
func (tc *TypeChecker) undefinedNameMessage(name string, scope map[string]ir.Type) string {
	candidates := make([]string, 0, len(scope)+len(tc.moduleConstTypes))
	for local, typ := range scope {
		if typ.Name != poisonName {
			candidates = append(candidates, local)
		}
	}
	for constant := range tc.moduleConstTypes {
		candidates = append(candidates, constant)
	}
	message := fmt.Sprintf("undefined name %q: no local, parameter or constant of that name is in scope", name)
	if near := nearest(name, candidates); near != "" {
		return message + fmt.Sprintf("; did you mean %s?", near)
	}
	return message + "; declare it with `let`, or check the spelling"
}

// undefinedFunctionMessage is what to say about a call to a function nothing
// declares.
func (tc *TypeChecker) undefinedFunctionMessage(name string) string {
	candidates := make([]string, 0, len(tc.funcs)+len(intrinsicSignatures))
	for fn := range tc.funcs {
		candidates = append(candidates, fn)
	}
	for fn := range intrinsicSignatures {
		candidates = append(candidates, fn)
	}
	candidates = append(candidates, "print")
	message := fmt.Sprintf("undefined function %q: nothing declared or imported has that name", name)
	if near := nearest(name, candidates); near != "" {
		return message + fmt.Sprintf("; did you mean %s?", near)
	}
	return message + "; a function from another module needs an `import` of that module"
}

// nearest is the candidate closest to a name, when it is close enough to be a
// slip of the keyboard: one edit for a short name, two for a longer one.
func nearest(name string, candidates []string) string {
	// A name of one or two letters is a fresh name more often than a slip, and
	// every other short name is one edit from it.
	if len(name) < 3 {
		return ""
	}
	limit := 1
	if len(name) > 5 {
		limit = 2
	}
	sort.Strings(candidates)
	best, bestDistance := "", limit+1
	for _, candidate := range candidates {
		if candidate == name || candidate == "" {
			continue
		}
		if d := editDistance(name, candidate); d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	return best
}

// editDistance is the Levenshtein distance between two names.
func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

// fieldsHint names the field a misspelt one is closest to, or lists them.
func fieldsHint(name string, fields map[string]ir.Type) string {
	names := make([]string, 0, len(fields))
	for field := range fields {
		names = append(names, field)
	}
	if near := nearest(name, names); near != "" {
		return "did you mean " + near + "?"
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "it has no fields"
	}
	return "its fields are " + strings.Join(names, ", ")
}
