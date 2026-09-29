//
// @file emit_rho.go
// @brief Writing the Rho a service calls its statements through.
//
// This is the file a project ends up with and never writes. The service's SQL is
// turned into one struct per shape of row and one function per statement, and a
// handler calls a function by the name the statement was declared with. There is
// no SQL in the generated file and no type the service has to spell: the
// columns are typed by the engine that holds them, and the markers are the
// function's parameters.
//
// The shape is chosen so the same generated code runs on a microcontroller and in
// a service. A function takes a handle, the values, and a slice the caller owns,
// and answers how many rows it wrote:
//
//	fn find_customer(
//	    db: SqlDb,
//	    address: string,
//	    rows: Slice[CustomerRow],
//	    capacity: u32
//	) Result[u32, SqlError]
//
// Nothing is allocated and nothing is thrown across the boundary. A device with
// 2 KB of RAM and a service with a garbage collector call the same function, and
// the difference between a failure and an answer is a count.
//
// The statement's SQL is not in the file. It is a constant, and the port
// implements the function over the engine's own driver — so the generated Rho
// does not carry a dialect, and moving a service to another engine does not
// move a line of it.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import (
	"fmt"
	"sort"
	"strings"
)

// RhoOptions say what a project's generated Rho is called and where it goes.
type RhoOptions struct {
	// Package is the Rho module the file belongs to, which is the project's own
	// name unless it says otherwise.
	Package string
	// Imports are the Rho modules the generated file needs, beyond the one the
	// statements are written in.
	Imports []string
}

// ModelsRho is the Rho a project's row shapes are written in.
func (r Result) ModelsRho(options RhoOptions) string {
	var b strings.Builder
	r.writeRhoHeader(&b, options,
		"Generated result types. One struct per shape of row a statement returns,\n"+
			"which is the table's own shape when a statement returns a whole table\n"+
			"and is named for the statement otherwise.")
	for _, model := range r.Models {
		writeRhoComment(&b, model.Comment, "")
		fmt.Fprintf(&b, "struct %s {\n", model.Name)
		for _, field := range model.Fields {
			writeRhoComment(&b, field.Comment, "    ")
			fmt.Fprintf(&b, "    %s: %s\n", rhoFieldName(field.Name), field.Type.RhoType())
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}

// QueriesRho is the Rho a project's statements are written in.
func (r Result) QueriesRho(options RhoOptions) string {
	var b strings.Builder
	r.writeRhoHeader(&b, options,
		"Generated statements. One function per named statement, taking the values\n"+
			"the statement takes and answering how many rows it read or changed.")

	// The per-statement functions are externs: the port implements each one over
	// the engine's own driver, and a function written here would be a function
	// the compiler could not type, because what a statement returns is a fact
	// about the database rather than about the text of a query.
	for _, query := range r.Queries {
		writeRhoComment(&b, query.Comment, "")
		b.WriteString(externSignature(query, true))
		b.WriteString("\n")
	}
	for _, query := range r.Queries {
		writeRhoComment(&b, query.Comment, "")
		for _, warning := range query.Warnings {
			fmt.Fprintf(&b, "// Note: %s.\n", warning)
		}
		writeRhoQuery(&b, query)
	}
	return b.String()
}

// externSignature is the declaration the port implements.
//
// The name is the generator's own `sql_`-prefixed spelling, used verbatim: the
// port implements the name the generator recorded rather than one rebuilt here,
// so a name that is derived twice cannot come out two different ways. Prefixing
// it again is what produced `sql_sql_find_customer`, which nothing implements.
// A read and a write take different things. A read answers rows, so it takes
// the slice the caller owns and a capacity for them. A write answers a count of
// what it changed and has no rows to place anywhere, so it takes neither —
// emitting `Slice[]` for a statement with no result would name a slice of
// nothing, which is not a type.
func externSignature(query GeneratedQuery, keyword bool) string {
	var b strings.Builder
	if keyword {
		b.WriteString("extern fn ")
	}
	fmt.Fprintf(&b, "%s(", query.Extern)
	for index, parameter := range query.Parameters {
		if index > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s: %s", parameter.Name, parameter.Type.RhoType())
	}
	// The handle comes after the values, and a statement that takes any needs the
	// comma between them: a signature whose last value runs into the handle is
	// not a signature, it is two declarations the parser reads as one.
	if len(query.Parameters) > 0 {
		b.WriteString(", ")
	}
	if query.Result == "" {
		b.WriteString("db: SqlDb) u32")
	} else {
		fmt.Fprintf(&b, "db: SqlDb, out: Slice[%s], capacity: u32) u32", query.Result)
	}
	if keyword {
		b.WriteString(";")
	}
	return b.String()
}

// writeRhoQuery is one generated function.
//
// It is a wrapper rather than the query itself, and it is a wrapper that has
// something to do: a read refuses when there is no room for a row, because a
// read that silently answers nothing is a read that looks like a table that is
// empty. A write has no such ambiguity — it answered a count — so what it
// reports is the change, and a zero is a change of nothing rather than a
// failure to try.
func writeRhoQuery(b *strings.Builder, query GeneratedQuery) {
	fmt.Fprintf(b, "fn %s(", query.Name)
	for index, parameter := range query.Parameters {
		if index > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "%s: %s", parameter.Name, parameter.Type.RhoType())
	}
	// The handle comes after the values, and the comma between them is what
	// keeps the last value from running into it.
	if len(query.Parameters) > 0 {
		b.WriteString(", ")
	}
	if query.Result == "" {
		b.WriteString("db: SqlDb) Result[u32, SqlError] {\n")
		fmt.Fprintf(b, "    let changed: u32 = %s(", query.Extern)
		for _, name := range parameterNames(query) {
			fmt.Fprintf(b, "%s, ", name)
		}
		b.WriteString("db)\n")
		b.WriteString("    return Ok[u32, SqlError](changed)\n}\n\n")
		return
	}
	fmt.Fprintf(b, "db: SqlDb, out: Slice[%s], capacity: u32) Result[u32, SqlError] {\n", query.Result)
	b.WriteString("    if capacity == 0 {\n        return Err[u32, SqlError](SqlError{ \"no room for rows\", -1 })\n    }\n")
	fmt.Fprintf(b, "    let wrote: u32 = %s(", query.Extern)
	for _, name := range parameterNames(query) {
		fmt.Fprintf(b, "%s, ", name)
	}
	b.WriteString("db, out, capacity)\n")
	b.WriteString("    if wrote == 0 {\n")
	b.WriteString("        return Err[u32, SqlError](SqlError{ \"the statement failed\", -1 })\n    }\n")
	b.WriteString("    return Ok[u32, SqlError](wrote)\n}\n\n")
}

// parameterNames are the values a function takes, in order.
func parameterNames(query GeneratedQuery) []string {
	out := make([]string, 0, len(query.Parameters))
	for _, parameter := range query.Parameters {
		out = append(out, parameter.Name)
	}
	return out
}

// writeRhoHeader is what a generated Rho file opens with.
func (r Result) writeRhoHeader(b *strings.Builder, options RhoOptions, about string) {
	b.WriteString("// Generated by rho sql. DO NOT EDIT.\n//\n")
	for _, line := range strings.Split(about, "\n") {
		fmt.Fprintf(b, "// %s\n", line)
	}
	// How to use it, because a generated file that is included rather than
	// imported is not obvious: the reader has a file of functions and no
	// statement of what to do with them. `include` merges declarations into the
	// module that writes it, so the file is the project's own source and the
	// including module is the one that owns what is declared here.
	b.WriteString("//\n")
	b.WriteString("// Include it from the module that calls the statements:\n")
	b.WriteString("//\n")
	fmt.Fprintf(b, "//\tinclude %q\n", "models.rho")
	fmt.Fprintf(b, "//\tinclude %q\n", "queries.rho")
	b.WriteString("//\n")
	b.WriteString("// Regenerate it with: rho sql generate <project>\n\n")
	fmt.Fprintf(b, "module %s\n\n", options.Package)

	imports := append([]string{"std.sql", "std.core.result", "std.core.slice", "std.core.ptr"},
		options.Imports...)
	sort.Strings(imports)
	for _, name := range imports {
		fmt.Fprintf(b, "import %s\n", name)
	}
	b.WriteString("\n")
}

// writeRhoComment is a description above a declaration, or nothing when there is
// none.
func writeRhoComment(b *strings.Builder, text, indent string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(b, "%s// %s\n", indent, strings.TrimRight(line, " "))
	}
}

// rhoFieldName is a name the language accepts, which a column's name may not be:
// a marker-derived name like `?` is how a value is written, not what it is
// called.
func rhoFieldName(name string) string {
	if name == "" {
		return "value"
	}
	var b strings.Builder
	for i, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			if i == 0 {
				b.WriteRune(r + ('a' - 'A'))
			} else {
				b.WriteRune(r)
			}
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		}
	}
	// A name that begins with a digit cannot be an identifier, and a name that is
	// all punctuation is nothing at all.
	if b.Len() == 0 {
		return "value"
	}
	if d := b.String(); d[0] >= '0' && d[0] <= '9' {
		return "n" + d
	}
	return b.String()
}
