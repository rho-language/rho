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
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import "strings"

// npoPrefix is how a specialisation of `Option[Ptr[...]]` is mangled.
//
// The mangled name is what a backend is handed, so this is read off the name
// rather than re-deriving the specialisation: it is the same string the rest of
// the pipeline already agrees on, and a second derivation would be a second
// place for the two to disagree.
const npoPrefix = "Option_Ptr_"

// IsOptionalPointer reports whether a mangled type name is `Option[Ptr[T]]`.
func IsOptionalPointer(name string) bool {
	return strings.HasPrefix(name, npoPrefix) && len(name) > len(npoPrefix)
}

// optionalPointerCType returns the C type an optional pointer is represented
// as, and whether the name was one at all.
//
// `Option[RoboHubGPIO_Ptr]` carries `RoboHubGPIO`, so the representation is
// `RoboHubGPIO*`. A payload that already names a pointer keeps its star: an
// option of a pointer to a pointer is still a pointer, and writing `**` for it
// would be inventing a level of indirection the program did not ask for.
func optionalPointerCType(name string) (string, bool) {
	if !IsOptionalPointer(name) {
		return "", false
	}
	payload := name[len(npoPrefix):]
	if strings.HasSuffix(payload, "_Ptr") {
		payload = strings.TrimSuffix(payload, "_Ptr")
	}
	// The payload is a mangled type name, which for anything that is not a
	// declared struct is not what C calls it: `u8` is `uint8_t`. A pointer is
	// spelled from the element's C type, so the element is asked for its own.
	if cType, ok := cPrimitive(payload); ok {
		return cType + "*", true
	}
	return payload + "*", true
}

// cPrimitive spells a primitive the way C spells it, or reports that it is not
// one.
//
// The mangled name a payload carries is a Rho name: `u8`, not `uint8_t`. A
// pointer built from it has to be a C pointer, so the element is asked for the
// spelling the compiler knows.
func cPrimitive(name string) (string, bool) {
	switch name {
	case "u8":
		return "uint8_t", true
	case "u16":
		return "uint16_t", true
	case "u32":
		return "uint32_t", true
	case "u64":
		return "uint64_t", true
	case "i32":
		return "int32_t", true
	case "i64":
		return "int64_t", true
	case "f64":
		return "double", true
	case "bool":
		return "bool", true
	case "string":
		return "char", true
	}
	return "", false
}
