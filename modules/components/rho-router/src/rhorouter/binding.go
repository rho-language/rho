//
// @file binding.go
// @brief The accessor vocabulary, and what each one says about a request.
//
// A handler reads a request through accessors, and the accessor is where a
// value's type is decided. `path_u32(req, "id")` is a request for a 32-bit
// unsigned integer, and the generated service converts the text to one with a
// statement the compiler can see. Reading the accessors a handler calls is
// therefore how this component knows what an operation takes, with no second
// declaration to keep in step with the first.
//
// The list is read from the toolchain rather than written here, so the two
// cannot disagree about which accessors exist or what they are called. A
// binding primitive the toolchain gains is documented here on the day it lands.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhorouter

import "rho/src/frontend/ast"

// accessor is one typed accessor, as the language spells it.
type accessor struct {
	// name is the Rho spelling, as in path_u32.
	name string
	// in is where the value comes from.
	in string
	// kind is the Rho type the raw text is converted to.
	kind string
	// whole marks an accessor that reads the body as a whole rather than a
	// field of it, so a body has no name to write down.
	whole bool
}

// sources are the places a request carries a value, and how each is read.
//
// `body` and `field` are both the body, and they differ in what they read: one
// takes the body whole, which is what an upload or a package is, and the other
// takes one field of a JSON document. A whole read has no name to write down,
// which is what `whole` records.
var sources = []struct {
	prefix string
	in     string
	whole  bool
}{
	{"path", InPath, false},
	{"query", InQuery, false},
	{"header", InHeader, false},
	{"body", InBody, true},
	{"field", InBody, false},
}

// kinds are the types a value is converted to, and the suffix each accessor
// spells itself with. A string is read with `str`, because `string` is a word a
// handler should not have to type on every read.
var kinds = []struct {
	suffix string
	kind   string
	// schema is the type an OpenAPI document calls it, which is not always the
	// Rho spelling: an unsigned integer is a document's `integer` with a
	// format, not a type of its own.
	schema string
	format string
}{
	{"str", "string", "string", ""},
	{"i32", "i32", "integer", "int32"},
	{"u32", "u32", "integer", "int64"},
	{"u64", "u64", "integer", "int64"},
	{"f64", "f64", "number", "double"},
	{"bool", "bool", "boolean", ""},
}

// aliases are the accessors that predate the typed ones.
//
// A project written before the typed accessors existed reads a value as text
// and converts it itself, and it is still a service with an HTTP surface. So
// the names it reads through are in the vocabulary too, and a route that uses
// them is described with the same sources and the same confidence as one that
// does not. What is not known is the type: these read text, so the value is
// documented as a string, which is what the handler received.
var aliases = []accessor{
	// param(req, "name") reads a captured path wildcard as text.
	{name: "param", in: InPath, kind: "string"},
	// request_query_param(req, "q") reads one query parameter as text.
	{name: "request_query_param", in: InQuery, kind: "string"},
	// request_body(req) reads the whole body, which is what an upload is.
	{name: "request_body", in: InBody, kind: "string", whole: true},
}

// accessors is every typed accessor, built from the two tables so that a new
// type is one line rather than thirty, plus the aliases.
var accessors = append(buildAccessors(), aliases...)

// accessorsByName indexes the accessors by the name a handler calls.
var accessorsByName = indexAccessors(accessors)

// credentialReads are the helpers a handler calls when it reads a credential.
//
// They are recognised so that a test can assert the set, and so that a reader
// of this file knows why they are not in the vocabulary above: a credential is
// not a parameter, and whether an operation needs one is a statement about the
// service rather than about one value a handler read.
var credentialReads = []string{
	"request_auth", "auth_bearer", "auth_subject", "auth_is_authenticated", "auth_required",
}

// CredentialReads lists the helpers a handler reads a credential with.
func CredentialReads() []string {
	out := make([]string, len(credentialReads))
	copy(out, credentialReads)
	return out
}

// buildAccessors returns every source-and-type combination, plus the two
// whole-body reads. The latter are spelled apart from the table because neither
// is a conversion of the same value from a different place: one is the body
// itself and one is a field of a JSON body.
func buildAccessors() []accessor {
	out := []accessor{}
	for _, source := range sources {
		for _, kind := range kinds {
			out = append(out, accessor{
				name:  source.prefix + "_" + kind.suffix,
				in:    source.in,
				kind:  kind.kind,
				whole: source.whole,
			})
		}
	}
	return out
}

// indexAccessors indexes a list by the name a handler calls.
func indexAccessors(list []accessor) map[string]accessor {
	out := map[string]accessor{}
	for _, entry := range list {
		out[entry.name] = entry
	}
	return out
}

// AccessorNames lists every accessor a handler may call, for the command to
// print and for a document to describe.
func AccessorNames() []string {
	out := make([]string, 0, len(accessors))
	for _, entry := range accessors {
		out = append(out, entry.name)
	}
	return out
}

// parameterOf reads a call as the parameter it describes, and reports whether
// the call is a binding accessor at all. A call that is not one is some other
// function, and is left to whatever it is.
func parameterOf(call ast.CallExpr) (Parameter, bool) {
	callee, ok := call.Callee.(ast.IdentifierExpr)
	if !ok {
		return Parameter{}, false
	}
	entry, known := accessorsByName[callee.Name]
	if !known {
		return Parameter{}, false
	}
	// A value is not required because it was read. It is required because a
	// path wildcard cannot be absent, or because the handler refused the
	// request when it would not convert. Anything else a client may leave out.
	//
	// A whole-body read has no name to write down: the body is the value.
	if entry.whole {
		return Parameter{In: entry.in, Type: entry.kind}, true
	}
	name := stringArgument(call.Args, 1)
	if name == "" {
		// The name is a variable rather than a literal, so there is nothing
		// to write into a document. A value read that way is described by hand
		// rather than by this reading.
		return Parameter{}, false
	}
	return Parameter{Name: name, In: entry.in, Type: entry.kind}, true
}

// SchemaType returns the type a document calls a Rho type, and the format that
// goes with it. An unsigned integer is a document's `integer` with a format
// rather than a type of its own, so the mapping is stated here once and both
// the accessor's table and every writer that turns one into a schema read it
// from the same place.
func SchemaType(rho string) (string, string) {
	for _, kind := range kinds {
		if kind.kind == rho {
			return kind.schema, kind.format
		}
	}
	return rho, ""
}

// DescribeAccessor says what an accessor reads, in a form a person can act on.
// The generated service names a failure the same way, so what a handler is told
// at run time and what the command prints here are one set of words.
func DescribeAccessor(name string) string {
	entry, known := accessorsByName[name]
	if !known {
		return "not a binding accessor"
	}
	if entry.whole {
		return "the request body, as a Rho " + entry.kind
	}
	return "one value from the " + sourceLabel(entry.in) + ", as a Rho " + entry.kind
}

// sourceLabel names a request source the way a failure message does.
func sourceLabel(in string) string {
	switch in {
	case InPath:
		return "path parameter"
	case InQuery:
		return "query parameter"
	case InHeader:
		return "header"
	case InBody:
		return "request body"
	default:
		return in
	}
}
