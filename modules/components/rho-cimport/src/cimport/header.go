//
// @file header.go
// @brief Reading C declarations out of a header.
//
// A header is not a language this repository has a parser for, and pretending
// otherwise would produce bindings that are wrong in ways a compiler accepts.
// So the reader is small and honest about its scope: it reads the declarations
// a C interface is made of, and it reports what it met and did not read rather
// than skipping it in silence.
//
// What is read:
//
//	#include "x.h"                        what a header depends on
//	typedef unsigned int rh_u32;          an alias for a type
//	typedef enum { A = 1, B } Name;       an enumeration
//	typedef struct { rh_u32 a; } Name;     a structure
//	typedef union  { rh_u32 a; } Name;     a union
//	typedef void (*Name)(int x);          a function pointer
//	RoboHubError robohub_init(void);      a function
//	#define RH_MAX 32                      a constant
//
// What is not read, and is reported: bitfields, unions of several members (the
// member is read and its type is the union's), K&R parameter lists, and anything
// the reader cannot make sense of. A declaration it cannot read is one a
// binding must not claim to expose.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cimport

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Kind is what a declaration is.
type Kind string

const (
	// KindAlias is a typedef naming a type: `typedef unsigned int rh_u32;`
	KindAlias Kind = "alias"
	// KindEnum is an enumeration.
	KindEnum Kind = "enum"
	// KindStruct is a structure.
	KindStruct Kind = "struct"
	// KindUnion is a union.
	KindUnion Kind = "union"
	// KindFuncPtr is a function pointer typedef.
	KindFuncPtr Kind = "funcptr"
	// KindFunc is a function declaration.
	KindFunc Kind = "func"
	// KindConst is a #define constant.
	KindConst Kind = "const"
)

// Decl is one declaration read from a header.
type Decl struct {
	Kind Kind
	// Name is the name the header gives it.
	Name string
	// Type is the type of an alias, a constant's type when it has one, and a
	// struct's tag.
	Type string
	// Value is what a constant is, as it is written.
	Value string
	// EnumName is the enumeration a member belongs to, for KindEnum.
	EnumName string
	// Params are the parameters of a function or a function pointer.
	Params []Param
	// Result is what a function returns, empty for void.
	Result string
	// Fields are the members of a structure or a union.
	Fields []Field
	// Line is the line the declaration was read from, so a diagnostic can point
	// at the header rather than at the generator.
	Line int
	// Header is the file the declaration was read from.
	Header string
	// Comment is the documentation comment immediately above the declaration,
	// which is where a C interface explains itself.
	Comment string
}

// Param is one parameter of a function.
type Param struct {
	Name string
	Type string
}

// Field is one member of a structure or a union.
type Field struct {
	Name string
	Type string
	// Ptr is true when the member is a function pointer, which Rho cannot hold
	// and which the generator therefore bridges in C.
	Ptr bool
	// Result and Params are the signature of a function-pointer member: the shim
	// the generator writes is made from them, so they have to survive the
	// reading rather than being reduced to a name.
	Result  string
	Params  []Param
	Comment string
}

// Diagnostic is a declaration the reader could not read, or a header it could
// not find.
type Diagnostic struct {
	Header  string
	Line    int
	Message string
}

func (d Diagnostic) String() string {
	switch {
	case d.Header == "" && d.Line > 0:
		return fmt.Sprintf("line %d: %s", d.Line, d.Message)
	case d.Header == "":
		return d.Message
	default:
		return fmt.Sprintf("%s:%d: %s", d.Header, d.Line, d.Message)
	}
}

// Header is a set of declarations read from one or more C headers.
type Header struct {
	// Decls are the declarations, in the order they were read, with a name
	// having the last definition win.
	Decls []Decl
	// Includes are the headers the ones read depend on, which is what a reader
	// of a binding needs to know: it is not the whole interface if a header it
	// never saw declares a type it uses.
	Includes []string
	// Diagnostics is everything the reader could not read.
	Diagnostics []Diagnostic
}

// Names are the names declared, sorted, which is what a report lists.
func (h Header) Names() []string {
	names := map[string]bool{}
	for _, decl := range h.Decls {
		if decl.Name != "" {
			names[decl.Name] = true
		}
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Find returns a declaration by name.
func (h Header) Find(name string) (Decl, bool) {
	for index := len(h.Decls) - 1; index >= 0; index-- {
		if h.Decls[index].Name == name {
			return h.Decls[index], true
		}
	}
	return Decl{}, false
}

/* -------------------------------------------------------------------------- */
/*  The reader                                                                 */
/* -------------------------------------------------------------------------- */

// ReadHeader reads one header, following the `#include` lines it names when
// they are on disk, and returns what it declares.
//
// The reader works on *statements* rather than on lines, because C writes a
// declaration over as many lines as it likes and a reader that takes one line at
// a time sees half a struct. A statement ends where a declaration ends: a
// semicolon at brace and paren depth zero, or a closing brace at depth zero.
func ReadHeader(path string, includeDirs []string, followed map[string]bool, depth int) Header {
	header := Header{}
	if depth > 16 {
		// A header that includes itself would otherwise read forever. The bound
		// is a diagnostic rather than a crash, because the author of the header
		// is the one who can fix it.
		header.Diagnostics = append(header.Diagnostics, Diagnostic{Header: path, Message: "headers are included too deeply; check for a cycle"})
		return header
	}
	if followed[path] {
		return header
	}
	followed[path] = true

	text, err := ReadFile(path)
	if err != nil {
		header.Diagnostics = append(header.Diagnostics, Diagnostic{Header: path, Message: "cannot read the header: " + err.Error()})
		return header
	}

	base := filepath.Dir(path)
	scanner := &scanner{lines: strings.Split(text, "\n"), path: path}
	for scanner.hasMore() {
		line, number, comment := scanner.next()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// A preprocessor line is one line by definition, and the reader acts on
		// it at once.
		if strings.HasPrefix(trimmed, "#") {
			switch {
			case strings.HasPrefix(trimmed, "#include"):
				included := includeTarget(trimmed)
				if included == "" {
					continue
				}
				header.Includes = append(header.Includes, included)
				// A quoted include is a project header and is followed; an angled
				// one is a toolchain header (stdint.h), which is not on disk
				// here and whose types are read as the primitives they are.
				if strings.Contains(trimmed, "\"") {
					if resolved, found := findInclude(included, base, includeDirs); found {
						header = mergeHeader(header, ReadHeader(resolved, includeDirs, followed, depth+1))
					} else {
						header.Diagnostics = append(header.Diagnostics, Diagnostic{
							Header: path, Line: number,
							Message: fmt.Sprintf("included header %q was not found under the declared include paths", included),
						})
					}
				}
			case strings.HasPrefix(trimmed, "#define"):
				name, rest := defineName(strings.TrimSpace(strings.TrimPrefix(trimmed, "#define")))
				if name == "" || strings.HasPrefix(rest, "(") {
					// A macro rather than a constant: one with parameters takes
					// arguments where it is used, and a binding cannot hold
					// either form. A `#define` whose value is an expression is
					// one of those, and reading it as a value would put
					// `((err) == OK)` into a generated header.
					continue
				}
				value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "="))
				header.Decls = append(header.Decls, Decl{
					Kind: KindConst, Name: name, Value: value,
					Line: number, Header: path, Comment: comment,
				})
			default:
				// A guard, a pragma, an include path limit: something the
				// preprocessor acts on and a reader of declarations has no use
				// for. `#ifdef __cplusplus` around a header's declarations is
				// the normal shape, and reporting it would bury the real
				// problems in noise.
			}
			continue
		}

		// A linkage block says how a symbol is linked, not what it is.
		if isLinkage(trimmed) {
			continue
		}

		// Anything else is a declaration, which may be several lines.
		statement := scanner.statement(trimmed, number, comment)
		if statement == "" {
			continue
		}
		decls, diagnostics := readDeclaration(statement, path, number, comment)
		header.Decls = append(header.Decls, decls...)
		header.Diagnostics = append(header.Diagnostics, diagnostics...)
	}
	return header
}

// readDeclaration reads one declaration, whatever shape it is.
func readDeclaration(text, path string, number int, comment string) ([]Decl, []Diagnostic) {
	switch {
	case strings.HasPrefix(text, "typedef"):
		decl, diagnostic := readTypedef(text, path, number, comment)
		if diagnostic != nil {
			return nil, []Diagnostic{*diagnostic}
		}
		return []Decl{*decl}, nil

	case strings.HasPrefix(text, "struct "), strings.HasPrefix(text, "union "):
		// A struct written without a typedef is a type the interface names
		// elsewhere, and it is read so a declaration that uses it is
		// understood rather than opaque.
		tag := structTag(text)
		if tag == "" {
			return nil, []Diagnostic{{Header: path, Line: number, Message: "a struct declares no name"}}
		}
		kind := KindStruct
		if strings.HasPrefix(text, "union ") {
			kind = KindUnion
		}
		return []Decl{{Kind: kind, Name: tag, Line: number, Header: path, Comment: comment}}, nil
	}

	if decl := readFunction(text, path, number, comment); decl != nil {
		return []Decl{*decl}, nil
	}
	// Whatever is left is a declaration the reader does not understand. It is
	// reported with its line, because a binding that is missing a declaration
	// and says nothing is the most misleading thing a generator can produce.
	return nil, []Diagnostic{{
		Header: path, Line: number,
		Message: fmt.Sprintf("cannot read %q; a C interface declares typedefs, structs, functions and constants", firstWords(text, 6)),
	}}
}

/* -------------------------------------------------------------------------- */
/*  statements                                                                 */
/* -------------------------------------------------------------------------- */

// isLinkage reports whether a statement is a C++ linkage block, which says how
// a symbol is linked rather than what it is.
func isLinkage(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "extern \"C") || trimmed == "{" || trimmed == "}" ||
		strings.HasPrefix(trimmed, "static_assert")
}

// scanner reads a header as a sequence of statements, keeping the comment above
// each one and skipping the parts of a header that are not declarations.
type scanner struct {
	lines []string
	index int
	path  string
	// inComment is true between a `/*` and the `*/` that closes it. A C header
	// documents itself in block comments that run over many lines, and a reader
	// that treated the continuation as code would report every documented line
	// as unreadable.
	inComment bool
	// expectingText is set by a keyword whose text is written on the lines that
	// follow it, which is how Kconfig writes help:
	//
	//	bool
	//	  Enable the thing
	//	  and what it does
	//
	// Those lines are the help of the symbol above, and a reader that reported
	// them as unreadable would report every documented symbol in a real fragment.
	expectingText bool
}

func (s *scanner) hasMore() bool { return s.index < len(s.lines) }

// next is the next line that is not part of a comment, with the comment above
// it when there is one.
func (s *scanner) next() (line string, number int, comment string) {
	collected := []string{}
	// carriedComment is set when this line was a comment in its own right, so
	// emptying the line of it does not throw the text away.
	carriedComment := false
	// expectingText is set by a keyword that takes a block of text rather than
	// a value: `help`, `prompt` and `comment` are written with the text on the
	// following lines.
	s.expectingText = false
	for s.index < len(s.lines) {
		raw := s.lines[s.index]
		s.index++
		number = s.index

		if s.inComment {
			if end := strings.Index(raw, "*/"); end >= 0 {
				s.inComment = false
				raw = raw[end+2:]
				if strings.TrimSpace(raw) == "" {
					continue
				}
			} else {
				continue
			}
		}
		// A keyword with no value of its own is followed by its text, indented.
		indented := strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")
		if !indented {
			s.expectingText = false
		}
		if open := strings.Index(raw, "/*"); open >= 0 {
			if end := strings.Index(raw[open+2:], "*/"); end >= 0 {
				// A comment that opens and closes on one line leaves the rest of
				// the line, which may still be a declaration.
				text := commentText(raw[open+2 : open+2+end])
				if strings.TrimSpace(text) != "" {
					collected = append(collected, strings.TrimSpace(text))
					carriedComment = true
				}
				raw = raw[:open] + raw[open+2+end+2:]
			} else {
				s.inComment = true
				raw = raw[:open]
			}
		}
		raw = stripLineComment(raw)
		trimmed := strings.TrimSpace(raw)

		if trimmed == "" {
			// A blank line ends the comment above whatever comes next, unless
			// the comment is still running — and unless this line carried the
			// comment, which a line that was only a comment does.
			if len(collected) > 0 && !s.inComment && !carriedComment {
				collected = nil
			}
			continue
		}
		// Indented text that follows a keyword with no value of its own is the
		// help of what came before: Kconfig writes
		//
		//	bool
		//	  Enable the thing
		//	  and what it does
		//
		// and a reader that reported those lines as unreadable declarations
		// would report every documented symbol in a real fragment.
		if s.expectingText && indented {
			collected = append(collected, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "*/") {
			// The end of a block comment whose opening was on an earlier line: the
			// text before it was already consumed, and what is left is the end.
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			// A comment that opens and closes, with nothing declared around it.
			if end := strings.Index(trimmed[2:], "*/"); end >= 0 {
				if text := commentText(trimmed[2 : 2+end]); strings.TrimSpace(text) != "" {
					collected = append(collected, strings.TrimSpace(text))
				}
				carriedComment = true
				continue
			}
		}
		if strings.HasPrefix(trimmed, "*") {
			collected = append(collected, strings.TrimSpace(strings.TrimPrefix(trimmed, "*")))
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			collected = append(collected, strings.TrimSpace(strings.TrimPrefix(trimmed, "//")))
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			return raw, number, strings.Join(collected, " ")
		}
		return raw, number, strings.Join(collected, " ")
	}
	return "", 0, ""
}

// statement is the whole of a declaration, which may run over several lines: it
// ends at a semicolon at depth zero, or at a closing brace of an aggregate.
func (s *scanner) statement(first string, number int, comment string) string {
	trimmed := strings.TrimSpace(first)
	if strings.HasSuffix(trimmed, ";") {
		return trimmed
	}
	collected := trimmed
	for s.index < len(s.lines) {
		raw := s.index
		s.index++
		piece := strings.TrimSpace(stripLineComment(s.lines[raw]))
		if piece == "" {
			continue
		}
		if strings.HasPrefix(piece, "#") || isLinkage(piece) {
			// Whatever follows is not part of this declaration, so the
			// declaration ends here — with whatever it has, which the caller
			// reports if it is not a declaration at all.
			s.index--
			break
		}
		if strings.HasPrefix(piece, "/*") {
			if end := strings.Index(piece, "*/"); end >= 0 {
				piece = piece[end+2:]
			} else {
				s.inComment = true
				continue
			}
		}
		collected = collected + " " + strings.TrimSpace(piece)
		// A declaration ends at its semicolon at depth zero. Stopping at the
		// closing brace of an aggregate would cut `typedef struct X { … } X;`
		// in half and leave the name behind as an unread declaration.
		if strings.HasSuffix(collected, ";") && balanced(collected) {
			break
		}
	}
	return collected
}

// balanced reports whether every brace and parenthesis in text is closed, which
// is what makes a semicolon the end of a declaration rather than a pause in one.
func balanced(text string) bool {
	depth, inString := 0, false
	for index := 0; index < len(text); index++ {
		switch text[index] {
		case '"':
			inString = !inString
		case '{', '(', '[':
			if !inString {
				depth++
			}
		case '}', ')', ']':
			if !inString {
				depth--
			}
		}
	}
	return depth <= 0 && !inString
}

// stripLineComment removes the comments on a line, whichever way they are
// written, respecting a quoted string so a `//` inside a literal survives.
//
// A comment inside a declaration is the normal way a C header documents an
// enumeration member or a struct field, so a reader that keeps it would carry
// `/* Success */` into every value it read.
func stripLineComment(line string) string {
	var b strings.Builder
	inString := false
	for index := 0; index < len(line); index++ {
		char := line[index]
		if inString {
			b.WriteByte(char)
			if char == '"' {
				inString = false
			}
			continue
		}
		switch {
		case char == '"':
			inString = true
			b.WriteByte(char)
		case char == '/' && index+1 < len(line) && line[index+1] == '/':
			return b.String()
		case char == '/' && index+1 < len(line) && line[index+1] == '*':
			end := strings.Index(line[index+2:], "*/")
			if end < 0 {
				// A comment that opens here closes on a later line, which the
				// scanner handles; what is left of this line is the comment.
				return b.String()
			}
			index += 2 + end + 1
		default:
			b.WriteByte(char)
		}
	}
	return b.String()
}

// mergeHeader folds a followed header into the one that included it, keeping
// the order and reporting everything both met.
func mergeHeader(into, from Header) Header {
	into.Decls = append(into.Decls, from.Decls...)
	into.Includes = append(into.Includes, from.Includes...)
	into.Diagnostics = append(into.Diagnostics, from.Diagnostics...)
	return into
}

/* -------------------------------------------------------------------------- */
/*  typedef                                                                    */
/* -------------------------------------------------------------------------- */

// readTypedef reads the shapes a typedef takes: an alias, an enum, a struct, a
// union, or a function pointer.
func readTypedef(text, path string, number int, comment string) (*Decl, *Diagnostic) {
	rest := strings.TrimSpace(strings.TrimPrefix(text, "typedef"))
	rest = strings.TrimSuffix(strings.TrimSpace(rest), ";")

	// Where the compound body opens, if it does. A function pointer is only a
	// function pointer when there is no body: a HAL struct has function pointers
	// in it, and reading the whole declaration as one of them would declare a
	// pointer named after the struct and lose the struct itself.
	open := strings.Index(rest, "{")
	if open < 0 || open > strings.Index(rest, "(*") {
		if pointer := functionPointer(rest); pointer != nil {
			return &Decl{
				Kind: KindFuncPtr, Name: pointer.Name, Result: pointer.Result,
				Params: pointer.Params, Line: number, Header: path, Comment: comment,
			}, nil
		}
	}

	// A compound type is recognised by its braces rather than by what the
	// declaration ends with: `enum { … } Name` ends with the name, and so does
	// `struct Tag { … } Name`, which is how a HAL is written.
	closing := -1
	if open > 0 {
		closing = strings.LastIndex(rest, "}")
		if closing < open {
			open = -1
		}
	}
	if open > 0 {
		// names the type before the body, and the members refer to it by that
		// name, so the tag is part of what the binding has to know.
		keyword, tag := firstWord(strings.TrimSpace(rest[:open]))
		name := strings.TrimSpace(rest[closing+1:])
		body := rest[open+1 : closing]
		if name == "" {
			name = tag
		}
		if name == "" {
			return nil, &Diagnostic{Header: path, Line: number, Message: "a typedef of a struct or enum declares no name"}
		}
		decl := &Decl{Name: name, Type: tag, Line: number, Header: path, Comment: comment}
		switch keyword {
		case "enum":
			decl.Kind = KindEnum
			decl.Fields = enumMembers(body)
			return decl, nil
		case "struct", "union":
			if keyword == "union" {
				decl.Kind = KindUnion
			} else {
				decl.Kind = KindStruct
			}
			fields, diagnostic := structFields(body, path, number)
			if diagnostic != nil {
				return nil, diagnostic
			}
			decl.Fields = fields
			return decl, nil
		}
		return nil, &Diagnostic{Header: path, Line: number, Message: fmt.Sprintf("cannot read a typedef of %q", keyword)}
	}

	// An alias: `unsigned int rh_u32`.
	name, target := splitDeclaration(rest)
	if name == "" {
		return nil, &Diagnostic{Header: path, Line: number, Message: "a typedef declares no name"}
	}
	return &Decl{Kind: KindAlias, Name: name, Type: target, Line: number, Header: path, Comment: comment}, nil
}

// firstWord is a declaration's first word and the rest of it.
func firstWord(text string) (word, rest string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}
	return fields[0], strings.TrimSpace(strings.Join(fields[1:], " "))
}

// splitDeclaration splits a declaration into the name and what it declares.
// The name is the last word of a declarator, which is where C puts it for
// everything a header interface uses.
func splitDeclaration(text string) (name, target string) {
	trimmed := strings.TrimSpace(text)
	// `const char* rh_name` names rh_name and is a pointer to characters. The
	// const says the pointee is read-only, which the type mapping does not
	// carry, so it is dropped rather than read as part of the name.
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "const"))
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", ""
	}
	name = fields[len(fields)-1]
	name = strings.TrimLeft(name, "*")
	target = strings.TrimSpace(strings.Join(fields[:len(fields)-1], " "))
	return name, target
}

// functionPointer reads `Ret (*Name)(params)`.
//
// The result is what stands *before* the pointer, not what follows the
// parameter list: a C function pointer has no trailing return type, so reading
// the result from the end of the declaration would always find nothing.
func functionPointer(text string) *Decl {
	open := strings.Index(text, "(*")
	if open < 0 {
		return nil
	}
	close := strings.Index(text[open:], ")")
	if close < 0 {
		return nil
	}
	name := strings.TrimSpace(text[open+2 : open+close])
	result := strings.TrimSpace(text[:open])
	params, _ := splitParams(strings.TrimPrefix(strings.TrimSpace(text[open+close+1:]), "("))
	return &Decl{Name: name, Result: result, Params: params}
}

// splitParams reads a parameter list and what a function returns: the text
// after the parameter list, with void meaning nothing.
func splitParams(text string) ([]Param, string) {
	closing := strings.LastIndex(text, ")")
	if closing < 0 {
		return nil, strings.TrimSpace(text)
	}
	result := strings.TrimSpace(text[closing+1:])
	inner := text[:closing]
	if trimmed := strings.TrimSpace(inner); trimmed == "" || trimmed == "void" {
		return nil, result
	}
	params := []Param{}
	for _, part := range strings.Split(inner, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "void" {
			continue
		}
		// `void* context` names context; `const rh_u8* data` names data.
		fields := strings.Fields(part)
		if len(fields) < 2 {
			params = append(params, Param{Type: part})
			continue
		}
		name := fields[len(fields)-1]
		type_ := strings.Join(fields[:len(fields)-1], " ")
		params = append(params, Param{Name: strings.TrimLeft(name, "*"), Type: strings.TrimSpace(type_)})
	}
	return params, result
}

// structFields reads the members of a structure or a union.
func structFields(body, path string, number int) ([]Field, *Diagnostic) {
	fields := []Field{}
	// A structure's members are separated by semicolons, which is what a C
	// struct is: `rh_u8* data; rh_size size;`. Splitting on commas instead cuts
	// nothing off — and takes the commas *inside* a function pointer's
	// parameter list as separators, which halves every member that has one.
	for _, part := range splitMembers(body) {
		part = strings.TrimSpace(stripComment(part))
		if part == "" {
			continue
		}
		if strings.Contains(part, ":") {
			// A bitfield has a width the reader does not model, and a width is
			// a layout fact Rho cannot express, so it is reported rather than
			// read as a plain member.
			return nil, &Diagnostic{Header: path, Line: number,
				Message: fmt.Sprintf("cannot read the bitfield %q", part)}
		}
		// A function pointer member: `Ret (*name)(params)`. Its signature is
		// kept, because the shim that reaches it is written from it.
		if strings.Contains(part, "(*") {
			if pointer := functionPointer(strings.TrimSpace(part)); pointer != nil && pointer.Name != "" {
				fields = append(fields, Field{
					Name:   pointer.Name,
					Type:   pointer.Result,
					Ptr:    true,
					Result: pointer.Result,
					Params: pointer.Params,
				})
				continue
			}
		}
		name, target := splitDeclaration(part)
		if name == "" {
			continue
		}
		fields = append(fields, Field{Name: name, Type: target})
	}
	return fields, nil
}

// enumMembers reads the members of an enumeration, keeping the values so a
// binding can name them.
func enumMembers(body string) []Field {
	fields := []Field{}
	for _, part := range splitTopLevel(body) {
		part = strings.TrimSpace(stripComment(part))
		if part == "" {
			continue
		}
		name, value := part, ""
		if index := strings.Index(part, "="); index >= 0 {
			name = strings.TrimSpace(part[:index])
			value = strings.TrimSpace(part[index+1:])
		}
		if name == "" {
			continue
		}
		fields = append(fields, Field{Name: name, Type: "int", Comment: value})
	}
	return fields
}

/* -------------------------------------------------------------------------- */
/*  Functions                                                                  */
/* -------------------------------------------------------------------------- */

// readFunction reads a function declaration, or nothing when the line is not one.
func readFunction(text, path string, number int, comment string) *Decl {
	if !strings.HasSuffix(text, ";") || strings.HasPrefix(text, "typedef") {
		return nil
	}
	open := strings.Index(text, "(")
	if open < 0 {
		return nil
	}
	// A function pointer variable is not a function; it is read as a member
	// elsewhere, and a line that is one here would be a variable declaration.
	if strings.Contains(text[:open], "(*") {
		return nil
	}
	prefix := strings.TrimSpace(text[:open])
	fields := strings.Fields(prefix)
	if len(fields) < 2 {
		return nil
	}
	name := fields[len(fields)-1]
	result := strings.Join(fields[:len(fields)-1], " ")
	params, _ := splitParams(text[open+1:])
	return &Decl{
		Kind: KindFunc, Name: name, Result: result, Params: params,
		Line: number, Header: path, Comment: comment,
	}
}

/* -------------------------------------------------------------------------- */
/*  Text                                                                       */
/* -------------------------------------------------------------------------- */

// stripComment removes a C comment from a line, respecting a quoted string so a
// `//` inside a literal is not read as a comment.
func stripComment(line string) string {
	inString := false
	for index := 0; index < len(line); index++ {
		switch line[index] {
		case '"':
			inString = !inString
		case '/':
			if inString {
				continue
			}
			if index+1 < len(line) && line[index+1] == '/' {
				return line[:index]
			}
			if index+1 < len(line) && line[index+1] == '*' {
				// A comment opened on this line and closed on the next: the
				// reader drops what is left of the line, which is the whole
				// comment.
				if end := strings.Index(line[index+2:], "*/"); end >= 0 {
					return line[:index] + line[index+2+end+2:]
				}
				return line[:index]
			}
		}
	}
	return line
}

// commentAbove is the documentation comment above a declaration, which is where
// a C interface explains itself and a binding should carry the explanation
// across.
func commentAbove(lines []string, index int) string {
	collected := []string{}
	for back := index - 1; back >= 0; back-- {
		trimmed := strings.TrimSpace(lines[back])
		switch {
		case trimmed == "":
			// A blank line ends the comment; a declaration separated from its
			// documentation by a blank line is not documented by it.
			if len(collected) > 0 {
				return ""
			}
			continue
		case strings.HasPrefix(trimmed, "/**"), strings.HasPrefix(trimmed, "/*"):
			text := strings.TrimPrefix(strings.TrimPrefix(trimmed, "/**"), "/*")
			text = strings.TrimPrefix(strings.TrimPrefix(text, "*"), " ")
			collected = append([]string{strings.TrimSpace(text)}, collected...)
		case strings.HasPrefix(trimmed, "*"):
			text := strings.TrimSpace(strings.TrimPrefix(trimmed, "*"))
			collected = append([]string{text}, collected...)
		case strings.HasPrefix(trimmed, "//"):
			text := strings.TrimSpace(strings.TrimPrefix(trimmed, "//"))
			collected = append([]string{text}, collected...)
		default:
			return strings.Join(collected, " ")
		}
	}
	return strings.Join(collected, " ")
}

// defineName is the name a `#define` declares and whatever follows it, so a
// caller can tell a constant from a macro: a macro's name is followed by its
// parameters, a constant's by its value.
func defineName(body string) (name, rest string) {
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return "", ""
	}
	return fields[0], strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(body), fields[0]))
}

// includeTarget is the header an `#include` names.
func includeTarget(line string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "#include"))
	rest = strings.TrimPrefix(rest, "<")
	if index := strings.Index(rest, ">"); index >= 0 {
		rest = rest[:index]
	}
	rest = strings.TrimPrefix(rest, "\"")
	if index := strings.Index(rest, "\""); index >= 0 {
		rest = rest[:index]
	}
	return strings.TrimSpace(rest)
}

// findInclude looks for an included header under the declared include paths, the
// way a compiler does.
func findInclude(name, from string, includeDirs []string) (string, bool) {
	candidates := []string{}
	if strings.HasPrefix(name, "/") {
		candidates = append(candidates, name)
	} else {
		candidates = append(candidates, filepath.Join(from, name))
		for _, dir := range includeDirs {
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}
	for _, candidate := range candidates {
		if exists(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// structTag is the name of a struct or union declared without a typedef.
func structTag(text string) string {
	fields := strings.Fields(strings.TrimSuffix(strings.TrimSpace(text), ";"))
	if len(fields) < 2 {
		return ""
	}
	return fields[len(fields)-1]
}

// splitMembers splits the body of a structure at the semicolons that end its
// members, and not at a semicolon inside a parameter list or a nested aggregate.
func splitMembers(text string) []string { return splitOn(text, ';') }

// splitTopLevel splits an enumeration at the commas that separate its members.
func splitTopLevel(text string) []string { return splitOn(text, ',') }

// splitOn splits text at every separator that is not inside a parameter list, a
// nested aggregate or a string literal.
func splitOn(text string, separator byte) []string {
	parts := []string{}
	depth := 0
	inString := false
	current := strings.Builder{}
	for index := 0; index < len(text); index++ {
		char := text[index]
		switch {
		case char == '"':
			inString = !inString
		case inString:
		case char == '(' || char == '{' || char == '[':
			depth++
		case char == ')' || char == '}' || char == ']':
			depth--
		case char == separator && depth == 0:
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(char)
	}
	if strings.TrimSpace(current.String()) != "" {
		parts = append(parts, current.String())
	}
	return parts
}

// commentText is the text of a documentation comment, with the stars and the
// slashes a C comment is written with taken off.
//
// A banner is not documentation: `/* ------ Type Definitions ------ */`
// separates the parts of a header and says nothing about what follows it, so it
// is not carried onto the declaration that happens to be under it.
func commentText(text string) string {
	text = commentLines(text)
	if isBanner(text) {
		return ""
	}
	return text
}

// isBanner reports whether a comment is a separator rather than documentation:
// mostly punctuation, or a heading in capitals.
func isBanner(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	letters, punctuation := 0, 0
	for _, char := range trimmed {
		switch {
		case unicode.IsLetter(char):
			letters++
		case unicode.IsSpace(char):
		default:
			punctuation++
		}
	}
	if punctuation > letters {
		return true
	}
	// A heading is written in capitals and is one or two words.
	if letters > 0 && strings.ToUpper(trimmed) == trimmed {
		words := strings.Fields(trimmed)
		if len(words) <= 3 {
			return true
		}
	}
	return false
}

// commentLines is the text of a comment with the stars and slashes taken off.
func commentLines(text string) string {
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		lines = append(lines, strings.TrimSpace(line))
	}
	return strings.Join(lines, " ")
}

// firstWords is a line shortened for a diagnostic that quotes it.
func firstWords(text string, count int) string {
	words := strings.Fields(text)
	if len(words) > count {
		words = append(words[:count], "...")
	}
	return strings.Join(words, " ")
}
