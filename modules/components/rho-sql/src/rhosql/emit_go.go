//
// @file emit_go.go
// @brief Writing the generated Go.
//
// The output is written by hand rather than through a template, for the same
// reason the rest of the toolchain writes its own: a generated file is something
// a person reads in a review, and what it says and in what order is the whole of
// that review. The file is also deterministic — the same project generates the
// same bytes — because a generator that reorders its output on every run cannot
// be diffed, and a file that cannot be diffed is a file nobody checks.
//
// The generated code is written against `database/sql` and nothing else. The
// engine is behind that, so a project whose generated code named a driver
// would be a project that could not be moved to another engine — and moving is
// the whole reason the engine is a plugin.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhosql

import (
	"fmt"
	"strings"
)

// ModelsGo is the file a project's result structs are written to.
func ModelsGo(result Result) string {
	var b strings.Builder
	writeFileHeader(&b, result.Package, "Generated result types. One struct per shape of row\n"+
		"a statement returns, which is the table's own shape when a statement\n"+
		"returns a whole table and is named for the statement otherwise.", false, false)
	for _, model := range result.Models {
		writeComment(&b, model.Comment, "")
		fmt.Fprintf(&b, "type %s struct {\n", model.Name)
		for _, field := range model.Fields {
			writeComment(&b, field.Comment, "\t")
			fmt.Fprintf(&b, "\t%s %s\n", field.Name, field.Type.GoType())
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}

// QueriesGo is the file a project's statements are written to.
func QueriesGo(result Result) string {
	var b strings.Builder
	writeFileHeader(&b, result.Package, "Generated statements. One function per named\n"+
		"statement, taking the values the statement takes and returning what it\n"+
		"returns.", true, len(result.Queries) > 0)
	models := modelsByName(result)
	if len(result.Guessed) > 0 {
		// What was guessed is said at the top of the file, because a reader
		// deciding whether to trust these functions should not have to find the
		// note on each one.
		fmt.Fprintf(&b, "// These results were taken from the schema rather than from an engine,\n"+
			"// because none was asked: %s. Run them against an engine to have it\n"+
			"// type them, which it can do better than a schema can.\n\n",
			strings.Join(sortedNames(result.Guessed), ", "))
	}
	for _, query := range result.Queries {
		writeQuery(&b, query, models)
	}
	return b.String()
}

// modelsByName are the result structs a query's return values scan into.
func modelsByName(result Result) map[string]Model {
	out := make(map[string]Model, len(result.Models))
	for _, model := range result.Models {
		out[model.Name] = model
	}
	return out
}

// writeQuery is one generated function.
func writeQuery(b *strings.Builder, query GeneratedQuery, models map[string]Model) {
	// The values, in the order the statement takes them. The order is the order
	// the markers appear, which is the order the engine rendered the SQL in, so
	// a positional argument list and the rendered text cannot disagree.
	values := make([]string, 0, len(query.Parameters))
	for _, parameter := range query.Parameters {
		values = append(values, lowerFirst(parameter.Name))
	}

	writeComment(b, query.Comment, "")
	for _, warning := range query.Warnings {
		fmt.Fprintf(b, "// Note: %s.\n", warning)
	}
	for _, line := range strings.Split(query.SQL, "\n") {
		fmt.Fprintf(b, "//   %s\n", strings.TrimRight(line, " "))
	}
	b.WriteString("//\n")

	fmt.Fprintf(b, "const sql%s = %s\n\n", query.Name, goString(query.SQL))

	var args []string
	args = append(args, "ctx context.Context")
	for _, parameter := range query.Parameters {
		args = append(args, fmt.Sprintf("%s %s", lowerFirst(parameter.Name), parameter.Type.GoType()))
	}
	signature := strings.Join(args, ", ")
	list := strings.Join(values, ", ")

	model, hasModel := models[query.Result]
	switch {
	case hasModel && query.Verb == VerbSelect:
		// A read. The rows come back as the shape the columns describe.
		fmt.Fprintf(b, "func %s(%s) ([]%s, error) {\n", query.Name, signature, query.Result)
		fmt.Fprintf(b, "\trows, err := db.QueryContext(ctx, sql%s, %s)\n", query.Name, list)
		b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
		b.WriteString("\tdefer rows.Close()\n\n")
		fmt.Fprintf(b, "\tout := []%s{}\n", query.Result)
		b.WriteString("\tfor rows.Next() {\n")
		fmt.Fprintf(b, "\t\tvar row %s\n", query.Result)
		b.WriteString("\t\tif err := rows.Scan(")
		scan := make([]string, 0, len(model.Fields))
		for _, field := range model.Fields {
			scan = append(scan, "&row."+field.Name)
		}
		b.WriteString(strings.Join(scan, ", "))
		b.WriteString("); err != nil {\n\t\t\treturn nil, err\n\t\t}\n")
		b.WriteString("\t\tout = append(out, row)\n\t}\n")
		b.WriteString("\treturn out, rows.Err()\n}\n\n")

	case hasModel:
		// A write that returns the rows it changed. They are read the same way a
		// read's are, and a caller that inserted a row needs the id it was given.
		fmt.Fprintf(b, "func %s(%s) ([]%s, error) {\n", query.Name, signature, query.Result)
		fmt.Fprintf(b, "\trows, err := db.QueryContext(ctx, sql%s, %s)\n", query.Name, list)
		b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
		b.WriteString("\tdefer rows.Close()\n\n")
		fmt.Fprintf(b, "\tout := []%s{}\n", query.Result)
		b.WriteString("\tfor rows.Next() {\n")
		fmt.Fprintf(b, "\t\tvar row %s\n", query.Result)
		b.WriteString("\t\tif err := rows.Scan(")
		scan := make([]string, 0, len(model.Fields))
		for _, field := range model.Fields {
			scan = append(scan, "&row."+field.Name)
		}
		b.WriteString(strings.Join(scan, ", "))
		b.WriteString("); err != nil {\n\t\t\treturn nil, err\n\t\t}\n")
		b.WriteString("\t\tout = append(out, row)\n\t}\n")
		b.WriteString("\treturn out, rows.Err()\n}\n\n")

	case query.Affected:
		// A write that returns no rows. What it changed is the answer.
		fmt.Fprintf(b, "func %s(%s) (int64, error) {\n", query.Name, signature)
		fmt.Fprintf(b, "\tresult, err := db.ExecContext(ctx, sql%s, %s)\n", query.Name, list)
		b.WriteString("\tif err != nil {\n\t\treturn 0, err\n\t}\n")
		b.WriteString("\treturn result.RowsAffected()\n}\n\n")

	default:
		// A statement that returns rows and a shape nobody could type. It is
		// written as a read with no result, which is honest: the caller runs it
		// and gets the error rather than a value they cannot read.
		fmt.Fprintf(b, "func %s(%s) error {\n", query.Name, signature)
		fmt.Fprintf(b, "\t_, err := db.ExecContext(ctx, sql%s, %s)\n", query.Name, list)
		b.WriteString("\treturn err\n}\n\n")
	}
}

/* -------------------------------------------------------------------------- */
/*  Writing Go                                                                  */
/* -------------------------------------------------------------------------- */

// writeFileHeader is what a generated file opens with.
//
// The handle is written only into the file that holds the statements, because it
// is theirs: a types file that also declared it would be a second declaration
// of the same name in every package the generator was asked to write.
func writeFileHeader(b *strings.Builder, pkg, about string, withHandle, withStatements bool) {
	b.WriteString("// Code generated by rho sql. DO NOT EDIT.\n//\n")
	for _, line := range strings.Split(about, "\n") {
		fmt.Fprintf(b, "// %s\n", line)
	}
	b.WriteString("//\n")
	b.WriteString("// Regenerate it with: rho sql generate <project>\n\n")
	fmt.Fprintf(b, "package %s\n\n", pkg)
	if !withHandle {
		return
	}
	// `context` is imported only when there is a statement to pass it to. A
	// project that declares tables and no statements is a project being started,
	// and its generated file has to build like any other.
	b.WriteString("import (\n")
	if withStatements {
		b.WriteString("\t\"context\"\n")
	}
	b.WriteString("\t\"database/sql\"\n)\n\n")
	b.WriteString(handle)
}

// handle is the database the statements run against, written into the file that
// holds them.
const handle = `// db is the database the statements run against.
//
// It is a handle rather than a connection because the engine behind it is a
// plugin: a caller opens the handle with the driver it configured, and nothing
// generated here knows which engine that is.
var db *sql.DB

// DB is the database the statements run against.
func DB() *sql.DB { return db }

// UseDB sets the database the statements run against. A service that talks to
// more than one calls it around the work that uses each.
func UseDB(connection *sql.DB) { db = connection }

`

// writeComment is a description above a declaration, or nothing when there is
// none.
func writeComment(b *strings.Builder, text, indent string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(b, "%s// %s\n", indent, strings.TrimRight(line, " "))
	}
}

// goString is a Go string literal, written so that a statement with a quote or
// a backslash in it is still a statement.
func goString(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// lowerFirst is a Go parameter name, which is a field's name in lower case.
// lowerFirst is a Go parameter name, which is a field's name in lower case.
//
// A marker is not a name: `?` and `$1` are how a statement marks a value, and
// neither is something a Go function can call a parameter. So a value marked
// positionally is named by its position, because the position is the only thing
// there is to name it by, and a caller reading the function learns from the
// statement above it which value goes where.
func lowerFirst(name string) string {
	switch name {
	case "":
		return "value"
	case "?":
		return "value"
	}
	// `$1` and friends are a position rather than a name.
	if len(name) > 1 && name[0] == '$' && allDigits(name[1:]) {
		return "value" + name[1:]
	}
	first := name[0]
	switch {
	case first >= 'A' && first <= 'Z':
		return strings.ToLower(name[:1]) + name[1:]
	case first >= 'a' && first <= 'z':
		return name
	}
	return "v" + name
}

// allDigits reports whether a name is a number, which a marker's is.
func allDigits(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return false
		}
	}
	return len(name) > 0
}
