//
// @file emit_go_bindport.go
// @brief Zero-reflection parameter binding for the routing port.
//
// A Rho handler reads a request through typed accessors, and every value it
// reads is converted by a statement the generator wrote. There is no
// reflection anywhere on the path: no `reflect` import, no `interface{}` to
// decode into, no tag to read. `path_i32(req, "id")` is a call to a function
// whose body ends in `strconv.ParseInt`, and the compiler can see all of it.
//
// The conversions a binding can fail at are the interesting part. A path
// segment that is not a number, a query parameter that is not a float, a
// header that is not a boolean: a REST API has to answer those with a status,
// not a panic. Each accessor therefore reports through the request it was
// given, and the handler asks once whether the binding held.
//
//	let id: u32 = path_u32(req, "id")
//	if !bind_ok(req) {
//	    return error_json(400, bind_error(req))
//	}
//
// The state travels on a pointer inside the request, because a Rho handler
// receives the request by value and a value it keeps is a copy. A copy shares
// the pointer, so a failure recorded by an accessor is still there to be read.
//
// The JSON reader is written out rather than delegated, for the same reason:
// `encoding/json` decodes into `any` by way of reflection, and a binding layer
// that claims to need none should not reach for it on the way to a field.
// Scanning the bytes for one top-level key costs a hundred lines and keeps the
// claim true.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"fmt"
	"strings"
)

// bindingSources are the places a request carries a value, and the Go
// expression each one reads it with.
type bindingSource struct {
	// rho is the prefix a Rho accessor spells, as in path_i32.
	rho string
	// read yields the raw text and whether it was present at all.
	read string
	// label is what a failure message calls this source.
	label string
}

// bindingSources are the request sources a typed accessor reads from. A
// project reaches all four from one handler: a path wildcard, a query
// parameter, a header, and a field of a JSON body.
var bindingSources = []bindingSource{
	{"path", "rhoBindPath", "path parameter"},
	{"query", "rhoBindQuery", "query parameter"},
	{"header", "rhoBindHeader", "header"},
	{"body", "rhoBindBody", "body"},
	{"field", "rhoBindField", "body field"},
}

// bindingType is one type a bound value is converted to.
type bindingType struct {
	// suffix is how an accessor spells this type, as in path_i32. It is the
	// Rho spelling, shortened where the language shortens it: a string is
	// read with `str`, because `string` is a word a caller should not have to
	// spell on every read.
	suffix string
	// gotype is what the value becomes in Go.
	gotype string
	// parse is the expression converting the raw text, empty for a string.
	parse string
	// cast narrows a parse wider than the type, empty when it already matches.
	cast string
	// zero is the value a failed conversion leaves behind.
	zero string
	// complaint is what a failure says, past the source and the name.
	complaint string
}

// bindingTypes are the types a bound value takes. Every conversion is a call
// the standard library spells out, so a generated project downloads nothing to
// bind a parameter.
var bindingTypes = []bindingType{
	{"str", "string", "", "", `""`, "is not a string"},
	{"i32", "int32", "strconv.ParseInt(raw, 10, 32)", "int32", "0", "is not a 32-bit integer"},
	{"u32", "uint32", "strconv.ParseUint(raw, 10, 32)", "uint32", "0", "is not a 32-bit unsigned integer"},
	{"u64", "uint64", "strconv.ParseUint(raw, 10, 64)", "", "0", "is not a 64-bit unsigned integer"},
	{"f64", "float64", "strconv.ParseFloat(raw, 64)", "", "0", "is not a number"},
	{"bool", "bool", "strconv.ParseBool(raw)", "", "false", "is not a boolean"},
}

// emitGoBinding renders the typed accessors and the reflection-free JSON
// reader they are built on.
func emitGoBinding() string {
	var b strings.Builder

	b.WriteString(`// bindState is what a failed conversion leaves on the request. It is
// reached through a pointer so that a Rho handler, which receives the request
// by value, still sees what an accessor recorded.
type bindState struct {
	err string
}

// bindFail records the first failure, so the message names the accessor the
// handler wrote rather than the last one to run.
func (req *Request) bindFail(message string) {
	if req.bind == nil || req.bind.err != "" {
		return
	}
	req.bind.err = message
}

`)

	// The readers. Each is the raw text for one source, so the accessors below
	// differ only in the conversion they perform.
	b.WriteString(goBindReaders())

	// The accessors: one per source per type, generated from the two tables so
	// that the spellings cannot drift between them.
	for _, source := range bindingSources {
		b.WriteString("\n")
		for _, kind := range bindingTypes {
			// The comment names the accessor as Rho spells it and the function
			// as Go spells it, so a reader searching for either finds it.
			accessor := source.rho + "_" + kind.suffix
			fmt.Fprintf(&b, "// %s reads the %s as a Rho %s.\n",
				accessor, source.label, kind.gotype)
			fmt.Fprintf(&b, "func %s(req Request, name string) %s {\n",
				lowerCamel(accessor), kind.gotype)
			fmt.Fprintf(&b, "\traw, ok := %s(req, name)\n", source.read)
			b.WriteString("\tif !ok {\n")
			fmt.Fprintf(&b, "\t\treq.bindFail(bindMessage(%q, name, %q))\n",
				source.label, "is missing")
			fmt.Fprintf(&b, "\t\treturn %s\n\t}\n", kind.zero)
			if kind.parse == "" {
				// A string is its own conversion. The empty value is a value,
				// not a failure, so a parameter present but blank binds blank.
				b.WriteString("\treturn raw\n}\n")
				continue
			}
			fmt.Fprintf(&b, "\tvalue, err := %s\n", kind.parse)
			b.WriteString("\tif err != nil {\n")
			fmt.Fprintf(&b, "\t\treq.bindFail(bindMessage(%q, name, %q))\n",
				source.label, kind.complaint)
			fmt.Fprintf(&b, "\t\treturn %s\n\t}\n", kind.zero)
			if kind.cast == "" {
				b.WriteString("\treturn value\n}\n")
				continue
			}
			// A parse wider than the type is narrowed here rather than at the
			// call site, so the conversion is stated once.
			fmt.Fprintf(&b, "\treturn %s(value)\n}\n", kind.cast)
		}
	}

	b.WriteString(`
// bindMessage names the value that did not bind, so the message a handler
// passes on identifies the parameter a client sent rather than the accessor
// the handler called.
func bindMessage(kind, name, complaint string) string {
	return "the " + kind + " " + strconv.Quote(name) + " " + complaint
}

// The presence reads and the defaulting read.
//
// A required value is read through a typed accessor, which converts it and
// records whether it converted. An optional one cannot be: the accessor would
// record a failure for a request that is perfectly correct, and the failure
// would sit on the request for the rest of the handler to trip over. So these
// four read without recording, and a handler that wants a value it may not get
// says so with them.
//
// This matters most for a catch-all wildcard, which is present on one request
// and absent on the next, and for a query parameter that filters a listing
// rather than identifying a thing.
`)
	b.WriteString(goPresenceReads())
	b.WriteString(`
// bind_ok reports whether every value the handler read converted.
func bindOk(req Request) bool {
	return req.bind == nil || req.bind.err == ""
}

// bind_error returns why a value did not convert, or "" when they all did.
func bindError(req Request) string {
	if req.bind == nil {
		return ""
	}
	return req.bind.err
}
`)

	b.WriteString(goJSONScanner())
	return b.String()
}

// goBindReaders renders the five readers the accessors share.
func goBindReaders() string {
	return `// rhoBindPath returns a captured path wildcard.
//
// The router has already matched the pattern and put every value in the
// request, so this is a map lookup rather than a scan of the path.
func rhoBindPath(req Request, name string) (string, bool) {
	value, ok := req.Params[name]
	return value, ok
}

// rhoBindQuery returns one query parameter. A parameter written more than once
// takes its first value, which is the one a client that means a single value
// sent first.
func rhoBindQuery(req Request, name string) (string, bool) {
	value, found := rhoQueryValue(req.Query, name)
	return value, found
}

// rhoBindHeader returns one header, matched without regard to case as HTTP
// requires.
func rhoBindHeader(req Request, name string) (string, bool) {
	value, found := rhoHeaderValue(req, name)
	return value, found
}

// rhoBindBody returns the whole request body as the value being bound. A
// handler that posts a number or a flag rather than a document reads it here;
// one that posts an object reads its fields instead.
func rhoBindBody(req Request, name string) (string, bool) {
	return req.Body, true
}

// rhoBindField returns one top-level field of a JSON request body.
func rhoBindField(req Request, name string) (string, bool) {
	value, ok := rhoJSONField(req.Body, name)
	return value, ok
}

// rhoQueryValue scans a raw query string for one key. The values arrive
// percent-encoded, so a space written as "+" comes back as a space rather than
// as a literal plus.
func rhoQueryValue(query, name string) (string, bool) {
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		key, value, _ := strings.Cut(part, "=")
		if key != name {
			continue
		}
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return value, true
		}
		return decoded, true
	}
	return "", false
}

// rhoHeaderValue returns one request header. The headers a Rho request carries
// are kept as they arrived, already canonicalised by net/http, so a lookup has
// to canonicalise the name it is given: a header written in another case names
// the same header.
func rhoHeaderValue(req Request, name string) (string, bool) {
	if req.head == nil {
		return "", false
	}
	if value, ok := req.head[name]; ok {
		return value, true
	}
	if canonical := http.CanonicalHeaderKey(name); canonical != name {
		value, ok := req.head[canonical]
		return value, ok
	}
	return "", false
}
`
}

// goJSONScanner renders the reader that finds one field of a JSON object
// without decoding the document.
//
// The scan is a single pass over the bytes. A key is read as a string, the
// value that follows is either copied out or stepped over, and a value that
// contains brackets is measured with a counter that ignores the brackets
// inside strings. Nothing is allocated but the answer.
func goJSONScanner() string {
	return `
// rhoJSONField returns the raw text of a top-level field of a JSON object.
//
// A string comes back without its quotes, so it converts exactly as a path
// wildcard or a query parameter would. A number, a boolean and null come back
// as they were written, which is the text the conversions expect. An object or
// an array comes back whole, and reading a field out of it is the caller's
// business rather than this reader's.
func rhoJSONField(body, name string) (string, bool) {
	for key, value := range rhoJSONObject(body) {
		if key == name {
			return value, true
		}
	}
	return "", false
}

// rhoJSONObject reads the top-level fields of a JSON object.
//
// A document that is not an object of scalars has no flat reading, so one is
// refused rather than half-understood. The caller asked for a value it can
// convert, and a value it cannot convert is not one to hand back.
func rhoJSONObject(body string) map[string]string {
	out := map[string]string{}
	i := rhoJSONSkipSpace(body, 0)
	if i >= len(body) || body[i] != '{' {
		return out
	}
	i++
	for {
		i = rhoJSONSkipSpace(body, i)
		if i >= len(body) || body[i] == '}' {
			return out
		}
		if body[i] != '"' {
			return out
		}
		key, next, ok := rhoJSONString(body, i)
		if !ok {
			return out
		}
		i = rhoJSONSkipSpace(body, next)
		if i >= len(body) || body[i] != ':' {
			return out
		}
		i = rhoJSONSkipSpace(body, i+1)
		end, ok := rhoJSONValueEnd(body, i)
		if !ok {
			return out
		}
		raw := body[i:end]
		if len(raw) > 0 && (raw[0] == '{' || raw[0] == '[') {
			// A nested value is not a scalar, and reading a field out of one is
			// the caller's business rather than this reader's.
			return out
		}
		out[key] = rhoJSONUnquote(raw)
		i = rhoJSONSkipSpace(body, end)
		if i < len(body) && body[i] == ',' {
			i++
			continue
		}
		return out
	}
}

// rhoJSONValueEnd returns the offset just past the value starting at i.
func rhoJSONValueEnd(body string, i int) (int, bool) {
	if i >= len(body) {
		return 0, false
	}
	switch body[i] {
	case '"':
		_, next, ok := rhoJSONString(body, i)
		return next, ok
	case '{', '[':
		return rhoJSONNested(body, i)
	default:
		// A number, a boolean or null runs until the byte that ends it.
		start := i
		for i < len(body) && body[i] != ',' && body[i] != '}' && body[i] != ']' &&
			body[i] != ' ' && body[i] != '\t' && body[i] != '\n' && body[i] != '\r' {
			i++
		}
		if i == start {
			return 0, false
		}
		return i, true
	}
}

// rhoJSONNested returns the offset just past a bracketed value, counting only
// the brackets that are not inside a string.
func rhoJSONNested(body string, i int) (int, bool) {
	depth := 0
	for i < len(body) {
		switch body[i] {
		case '"':
			_, next, ok := rhoJSONString(body, i)
			if !ok {
				return 0, false
			}
			i = next
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return i + 1, true
			}
			if depth < 0 {
				return 0, false
			}
		}
		i++
	}
	return 0, false
}

// rhoJSONString reads the quoted string starting at i, returning the unquoted
// key and the offset just past the closing quote.
func rhoJSONString(body string, i int) (string, int, bool) {
	var out strings.Builder
	i++
	for i < len(body) {
		switch body[i] {
		case '\\':
			if i+1 >= len(body) {
				return "", 0, false
			}
			i += 2
			continue
		case '"':
			return out.String(), i + 1, true
		default:
			out.WriteByte(body[i])
			i++
		}
	}
	return "", 0, false
}

// rhoJSONUnquote removes the quotes from a raw JSON string value, decoding the
// escapes a document may carry.
func rhoJSONUnquote(raw string) string {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return raw
	}
	var out strings.Builder
	body := raw[1 : len(raw)-1]
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' || i+1 >= len(body) {
			out.WriteByte(body[i])
			continue
		}
		i++
		switch body[i] {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'u':
			if i+4 < len(body) {
				if code, err := strconv.ParseUint(body[i+1:i+5], 16, 32); err == nil {
					out.WriteRune(rune(code))
					i += 4
					continue
				}
			}
			out.WriteByte(body[i])
		default:
			out.WriteByte(body[i])
		}
	}
	return out.String()
}

// rhoJSONSkipSpace returns the offset of the next byte that is not
// whitespace, which is where a document's structure resumes.
func rhoJSONSkipSpace(body string, i int) int {
	for i < len(body) {
		switch body[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}
`
}

// goPresenceReads renders the reads that do not record a failure, generated
// from the same source table as the accessors so that a source added to one is
// added to the other.
func goPresenceReads() string {
	var b strings.Builder
	for _, source := range bindingSources {
		// The body read whole is a value rather than a name, so there is no
		// presence to ask about: a body is either there or the request is not
		// one this handler serves. A field of one is a name, and does have a
		// presence.
		if source.rho == "body" {
			continue
		}
		fmt.Fprintf(&b, `
// %s reports whether the %s was sent, without recording anything. A handler
// uses it to tell a request that omitted an optional value from one that sent
// an empty one.
func %s(req Request, name string) bool {
	_, present := %s(req, name)
	return present
}
`, "has_"+source.rho, source.label, lowerCamel("has_"+source.rho), source.read)
	}
	b.WriteString(`
// query_default returns one query parameter, or a default when it was not sent.
// Unlike the typed accessors it records nothing, because a client that left a
// filter out has asked a valid question.
func queryDefault(req Request, name, fallback string) string {
	if value, present := rhoQueryValue(req.Query, name); present {
		return value
	}
	return fallback
}
`)
	return b.String()
}
