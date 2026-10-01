//
// @file toany.go
// @brief Converting a parsed YAML document into ordinary Go values.
//
// The loader parses YAML into a tree of Value because that is what validation
// wants: it can ask a node what kind it is without having to guess. Code that
// wants the document as data — anything that decodes it into a struct with
// reflection, or hands it to something that speaks JSON — wants the opposite,
// and this is where the conversion lives.
//
// It lives here rather than in each caller because the same conversion is needed
// by every component that reads a YAML document into a Go type, and three
// hand-written copies of it is three places for a quoted number to be read
// differently.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package yaml

// ToAny converts a parsed document into plain Go values: map[string]any,
// []any, string, bool or nil.
//
// Every scalar is a string, because that is all the loader distinguishes: YAML's
// own `true` and `1` and `null` are three spellings of three different things to
// whatever decodes this, and a document that says `enabled: true` reaches a
// struct field as "true". Decoding a document into a Go type is therefore the
// point at which a value is interpreted, and keeping the interpretation in one
// place — the struct's own tags — is what stops two readers of the same file
// from disagreeing about what it says.
//
// A key with no value is nil rather than an empty string, because that is the
// difference between a key that was declared and one that was left out.
func ToAny(value Value) any {
	switch value.Kind {
	case KindMap:
		converted := make(map[string]any, len(value.Map))
		for key, child := range value.Map {
			converted[key] = ToAny(child)
		}
		return converted
	case KindList:
		converted := make([]any, 0, len(value.List))
		for _, child := range value.List {
			converted = append(converted, ToAny(child))
		}
		return converted
	case KindNull:
		return nil
	default:
		return value.Str
	}
}
