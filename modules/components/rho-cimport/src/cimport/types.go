//
// @file types.go
// @brief Mapping C types onto Rho's.
//
// A binding is only useful when it is *typed*: a caller wants to know whether a
// value is a length or a pointer, and a generated declaration that blurred them
// would move the blur into every program that used it. So the mapping is a
// function of the C type, and a type the mapping does not know is reported
// rather than guessed at.
//
// The mapping is where the honest limits of a foreign interface show:
//
//   - A C function pointer has no Rho type, so a vtable member is carried as an
//     opaque pointer and bridged by generated C. Without that bridge the HAL is
//     unreachable, which is why the generator writes one.
//   - A pointer to a structure the interface declares is a typed pointer, so a
//     caller cannot pass the wrong thing.
//   - A bare `void*` is an opaque pointer: the interface says nothing about what
//     is in it, and inventing a type would be a lie.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package cimport

import (
	"fmt"
	"strings"
)

// TypeMapper maps a C type onto a Rho one, given the declarations the headers
// made. A struct known to the interface maps to a typed pointer; one that is not
// known maps to an opaque pointer, and is reported.
type TypeMapper struct {
	// Known is the set of structure and union names the interface declares, which
	// map to a typed pointer of that name.
	Known map[string]bool
	// Enums are the enumerations the interface declares. A C enumeration is a
	// whole number, and a Rho enum cannot be written in a foreign signature at
	// all, so one maps to the integer it is: the name is carried in the
	// declaration's documentation and is what the idiomatic layer compares a
	// result against.
	Enums map[string]bool
	// Aliases are the type aliases the interface declares, so a name that stands
	// for another name resolves through to what that one is.
	//
	// A portable interface is written in its own vocabulary: `rh_size` is a
	// `rh_u32`, which is a `uint32_t`, which is a Rho `u32`. A mapper that
	// stopped at the first name would type every field of every HAL as an opaque
	// pointer, which is the one answer that is always wrong and never said so.
	Aliases map[string]string
	// Provided are mappings a plugin supplied, which win over the ones here: a
	// plugin is a project saying what its own library's types mean, and it knows
	// better than a mapping that has never seen that library.
	Provided map[string]string
	// Unreadable is a type the mapping does not know, in the order met.
	Unreadable []string
}

// NewTypeMapper returns a mapper for the declarations a header set holds.
func NewTypeMapper(header Header) *TypeMapper {
	mapper := &TypeMapper{Known: map[string]bool{}, Enums: map[string]bool{}, Aliases: map[string]string{}}
	for _, decl := range header.Decls {
		switch decl.Kind {
		case KindStruct, KindUnion:
			mapper.Known[decl.Name] = true
		case KindEnum:
			mapper.Enums[decl.Name] = true
		case KindAlias:
			mapper.Aliases[decl.Name] = decl.Type
		}
	}
	return mapper
}

// primitives are the C types that map straight onto a Rho type.
var primitives = map[string]string{
	"void":               "Unit",
	"char":               "u8",
	"signed char":        "i8",
	"unsigned char":      "u8",
	"short":              "i16",
	"unsigned short":     "u16",
	"int":                "i32",
	"unsigned int":       "u32",
	"long":               "i64",
	"unsigned long":      "u64",
	"long long":          "i64",
	"unsigned long long": "u64",
	"float":              "f32",
	"double":             "f64",
	"bool":               "bool",
	// The fixed-width types and the size types, which is how a portable
	// interface writes them.
	"int8_t":    "i8",
	"uint8_t":   "u8",
	"int16_t":   "i16",
	"uint16_t":  "u16",
	"int32_t":   "i32",
	"uint32_t":  "u32",
	"int64_t":   "i64",
	"uint64_t":  "u64",
	"size_t":    "u64",
	"ptrdiff_t": "i64",
	"ssize_t":   "i64",
}

// Rho maps a C type onto a Rho one, and reports a type it cannot map.
func (m *TypeMapper) Rho(cType string) string {
	mapped, known := m.TryRho(cType)
	if !known {
		m.Unreadable = append(m.Unreadable, cType)
	}
	return mapped
}

// TryRho maps a C type onto a Rho one, saying whether it could.
func (m *TypeMapper) TryRho(cType string) (string, bool) {
	return m.resolve(cType, 0)
}

// resolve maps a C type, following the names a header builds up: an alias
// resolves to what it stands for, and a pointer decides what it points at before
// anything else does.
func (m *TypeMapper) resolve(cType string, depth int) (string, bool) {
	// A mapping a plugin supplied is consulted first, and only for the type it
	// names: a plugin adds what the generator does not know, it does not replace
	// what it does.
	if mapped, ok := m.Provided[normalise(strings.TrimSpace(cType))]; ok && mapped != "" {
		return mapped, true
	}
	// The chain is bounded because a header can declare `typedef A B;` and
	// `typedef B A;`, and a mapper that followed one forever would hang a build
	// over a header a compiler would accept.
	const maxAliasDepth = 16
	if depth > maxAliasDepth {
		return "Ptr[Unit]", false
	}

	// A const says the pointee is read-only, which a Rho type does not carry,
	// and a const that qualifies a whole aggregate is not a thing C has. It is
	// dropped before the type is decided, wherever it was written: `const char*`
	// is a pointer to characters, and deciding it as a constant would type it
	// wrongly.
	original := normalise(strings.ReplaceAll(normalise(strings.TrimSpace(cType)), "const", " "))
	if original == "" {
		return "Unit", true
	}

	if strings.Contains(original, "*") {
		return m.pointer(original, depth)
	}

	// An array, which Rho carries as a pointer to the element.
	if open := strings.Index(original, "["); open >= 0 {
		return m.resolve(strings.TrimSpace(original[:open])+"*", depth+1)
	}

	// A struct or union the interface declares is the *library's* type, and a
	// Rho program must not redeclare it: a struct written in Rho is a different
	// type that happens to have the same name, so a C library handing one over
	// would be read as the generated one and every member would be read from
	// the wrong offset. A foreign type is opaque here, and the layer above this
	// one is where it becomes a typed handle.
	//
	// The price is a parameter that is not checked by the compiler, and the
	// reason is worth the price: the alternative is a binding that is silently
	// wrong about memory.
	if strings.HasPrefix(original, "struct ") || strings.HasPrefix(original, "union ") {
		name := strings.TrimSpace(original[len("struct "):])
		if m.Known[name] {
			return "Ptr[" + name + "]", true
		}
		return "Ptr[Unit]", true
	}

	// An enumeration is a whole number in C, and the C backend cannot write a
	// Rho enum in a foreign signature at all, so it maps to the integer it is.
	if m.Enums[original] {
		return "i32", true
	}

	// A primitive, however it is spaced.
	if mapped, ok := primitives[original]; ok {
		return mapped, true
	}

	// An alias the interface declares: the name stands for another name, and the
	// answer is what that one finally is.
	if target, ok := m.Aliases[original]; ok {
		return m.resolve(target, depth+1)
	}

	// A struct or union named on its own is the library's type, so it is named:
	// the C the backend writes resolves against the header the interface includes.
	if m.Known[original] {
		return "Ptr[" + original + "]", true
	}
	return "Ptr[Unit]", false
}

// pointer maps a C pointer, deciding what it points at.
func (m *TypeMapper) pointer(cType string, depth int) (string, bool) {
	// A pointer to a function, or one to void, is opaque: there is nothing to
	// say about what it holds.
	base := strings.ReplaceAll(cType, "*", " ")
	fields := strings.Fields(base)
	if len(fields) == 0 {
		return "Ptr[Unit]", true
	}
	if len(fields) == 1 && (fields[0] == "void" || fields[0] == "struct") {
		return "Ptr[Unit]", true
	}
	if len(fields) == 2 && fields[0] == "struct" {
		// A pointer to a struct written inline: `struct DeviceI2C*`, which names
		// the same type as `DeviceI2C*`, so both are mapped the same way.
		if m.Known[fields[1]] {
			return "Ptr[" + fields[1] + "]", true
		}
		return "Ptr[Unit]", true
	}

	// A pointer to a primitive, to an alias, or to something the interface
	// declared as a type of its own.
	target := normalise(strings.Join(fields, " "))
	if mapped, ok := primitives[target]; ok {
		return "Ptr[" + mapped + "]", true
	}
	if alias, ok := m.Aliases[target]; ok {
		if mapped, ok := m.resolve(alias, depth+1); ok {
			return "Ptr[" + mapped + "]", true
		}
	}
	if m.Known[target] {
		return "Ptr[" + target + "]", true
	}
	// A pointer to a type the interface did not declare is opaque, and saying so
	// is better than inventing a type for it.
	return "Ptr[Unit]", false
}

// normalise is a C type as the mapping spells it: one space between words, and
// the spacing a header uses made uniform.
func normalise(cType string) string {
	return strings.Join(strings.Fields(cType), " ")
}

/* -------------------------------------------------------------------------- */
/*  Naming                                                                     */
/* -------------------------------------------------------------------------- */

/* -------------------------------------------------------------------------- */
/*  Enumeration members                                                        */
/* -------------------------------------------------------------------------- */

// EnumMembers are the names a C enumeration's members are given in Rho.
//
// C spells its members with the enumeration's own prefix — `DEVICE_ERR_IO` in
// `DeviceError` — and that prefix says which enumeration a member belongs to,
// which the Rho type already says. So what is dropped is the prefix the members
// *share*, which is what makes a set of related members read as a set.
//
//	DEVICE_OK, DEVICE_ERR_IO  ->  OK, IO
//	RH_FALSE, RH_TRUE            ->  FALSE, TRUE
func EnumMembers(members []Field) []string {
	prefix := sharedPrefix(members)
	out := make([]string, 0, len(members))
	for _, member := range members {
		name := strings.TrimPrefix(member.Name, prefix)
		if name == "" {
			name = member.Name
		}
		out = append(out, name)
	}
	return out
}

// sharedPrefix is the longest prefix every member of an enumeration has in
// common, cut at a separator so what remains is a member name and not a fragment
// of a word.
func sharedPrefix(members []Field) string {
	if len(members) == 0 {
		return ""
	}
	shared := members[0].Name
	for _, member := range members[1:] {
		shared = commonPrefixOf(shared, member.Name)
		if shared == "" {
			return ""
		}
	}
	// A prefix that ends mid-word is a fragment: `DEVICEERR` and
	// `DEVICE_ERR_IO` share `DEVICEERR`, and the underscore is what makes the
	// rest a member name.
	if cut := strings.LastIndex(shared, "_"); cut >= 0 {
		shared = shared[:cut+1]
	}
	return shared
}

// commonPrefixOf is the longest prefix two names share, comparing them the way C
// spells them: the case does not matter, the letters do.
func commonPrefixOf(left, right string) string {
	shared := 0
	for shared < len(left) && shared < len(right) {
		a, b := left[shared], right[shared]
		if a >= 'A' && a <= 'Z' {
			a += 'a' - 'A'
		}
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a != b {
			break
		}
		shared++
	}
	return left[:shared]
}

/* -------------------------------------------------------------------------- */
/*  Reporting                                                                  */
/* -------------------------------------------------------------------------- */

// Diagnostics turns what the mapper could not read into diagnostics, so a
// binding that is incomplete says why instead of looking complete.
func (m *TypeMapper) Diagnostics(header string) []Diagnostic {
	out := []Diagnostic{}
	seen := map[string]bool{}
	for _, cType := range m.Unreadable {
		if seen[cType] {
			continue
		}
		seen[cType] = true
		out = append(out, Diagnostic{
			Header:  header,
			Message: fmt.Sprintf("no Rho type for the C type %q; it is carried as an opaque pointer", cType),
		})
	}
	return out
}
