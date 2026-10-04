//
// @file emit_c.go
// @brief Writing the generated C.
//
// The Go backend writes against `database/sql`, which every driver sits behind.
// C has no such standard: a C program talks to sqlite by including
// `sqlite3.h` and linking `-lsqlite3`, so generated C that named either would
// be a program that could only ever be that one engine — which is the thing the
// plugin boundary exists to prevent.
//
// So the C is written against a small handle the generated file owns, with the
// few calls a statement needs. The engine behind it is bound by the caller, the
// same way a Go service binds a driver by opening the handle: a caller that
// links the sqlite engine provides the functions, and a caller that links
// another provides its own.
//
// The handle is deliberately the smallest thing that can carry a row. It is a
// few functions and two values, not a layer: a C caller who reads it should
// recognise it as what it is, not find a framework where a database call was.
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

// cTypes are the C types the generator's types are carried in.
//
// A string in C is a pointer and a length, not a value, so a text column is a
// pair. A caller reads it through the two fields, which is what keeps a binary
// value from being treated as a C string that ends early.
var cTypes = map[Type]string{
	TypeInteger: "int64_t",
	TypeFloat:   "double",
	TypeText:    "rho_text",
	TypeBoolean: "int",
	TypeBytes:   "rho_blob",
	TypeTime:    "rho_text",
	TypeJSON:    "rho_text",
	TypeNull:    "rho_text",
	TypeUnknown: "rho_text",
}

// ModelsC is the file a project's result structs are written to.
func ModelsC(result Result) string {
	var b strings.Builder
	writeCHeader(&b, result.Package, "Generated result types. One struct per shape of row\n"+
		"a statement returns, which is the table's own shape when a statement\n"+
		"returns a whole table and is named for the statement otherwise.", "models", false)

	// The value types come first, because a struct that holds one needs the type
	// to have been declared, and C reads top to bottom.
	b.WriteString(cValueTypes)

	for _, model := range result.Models {
		writeCComment(&b, model.Comment, "")
		b.WriteString("typedef struct {\n")
		for _, field := range model.Fields {
			writeCComment(&b, field.Comment, "    ")
			fmt.Fprintf(&b, "    %s %s;\n", cTypeOf(field.Type), lowerFirst(field.Name))
		}
		fmt.Fprintf(&b, "} %s;\n\n", model.Name)
	}
	b.WriteString("#endif /* " + cGuard(result.Package, "models") + " */\n")
	return b.String()
}

// cValueTypes are the value and string types the structs hold. They are declared
// in the header the structs are in, because a struct that holds a string needs
// the string to be a type before it can be a field.
const cValueTypes = `// rho_text is a string carried as a pointer and a length, so a value with a
// zero byte in it is not a string that ends early.
typedef struct {
    const char *pointer;
    int length;
} rho_text;

// rho_blob is binary data, carried the way text is because a caller treats them
// the same way at the boundary.
typedef struct {
    const char *pointer;
    int length;
} rho_blob;

`

// QueriesC is the file a project's statements are written to.
func QueriesC(result Result) string {
	var b strings.Builder
	writeCHeader(&b, result.Package, "Generated statements. One function per named\n"+
		"statement, taking the values the statement takes and returning what it\n"+
		"returns.", "queries", true)
	models := modelsByName(result)

	if len(result.Guessed) > 0 {
		fmt.Fprintf(&b, "// These results were taken from the schema rather than from an engine,\n"+
			"// because none was asked: %s.\n\n",
			strings.Join(sortedNames(result.Guessed), ", "))
	}
	for _, query := range result.Queries {
		writeCQuery(&b, query, models)
	}
	b.WriteString("#endif /* " + cGuard(result.Package, "queries") + " */\n")
	return b.String()
}

// cHandle is the database a statement runs against, and the calls a service makes
// on it.
//
// A statement's handle is opaque, so nothing generated names an engine: a caller
// that links the sqlite engine provides these, and a caller that links another
// provides its own. The order is the types first and the functions after, because
// C reads a declaration top to bottom.
const cHandle = `// rho_db is the handle the statements run against.
//
// It is opaque on purpose. The engine behind it is bound by the caller: a
// service that links one engine provides the functions below, and a service that
// links another provides its own. Nothing generated here names an engine, so the
// same statements run against either.
typedef struct rho_db rho_db;

// rho_rows is the result of a statement that returns rows, walked one at a time.
typedef struct rho_rows rho_rows;

// rho_value is one value, going in or coming out. It is typed rather than a bare
// void* so a caller cannot mistake a number for a pointer.
typedef struct {
    int kind;          // one of the RHO_* kinds below
    int64_t integer;   // a whole number
    double real;       // a number with a fraction
    int boolean;       // true or false
    rho_text text;     // text, or bytes when kind says so
    int is_null;       // whether the value is absent, which is a value of its own
} rho_value;

enum {
    RHO_NULL = 0,
    RHO_INTEGER,
    RHO_FLOAT,
    RHO_BOOLEAN,
    RHO_TEXT,
    RHO_BYTES
};

// The handle a service links. Each returns 0 for success, so a caller reads the
// same shape whichever engine it bound.
extern int  rho_db_open(const char *path, rho_db **out);
extern void rho_db_close(rho_db *db);
extern int  rho_db_query(rho_db *db, const char *sql, const rho_value *values, int count, rho_rows **out);
extern int  rho_db_exec(rho_db *db, const char *sql, const rho_value *values, int count, long long *affected);
extern int  rho_rows_next(rho_rows *rows, int *ok);
extern void rho_rows_close(rho_rows *rows);
extern int  rho_rows_column_count(rho_rows *rows);
extern int  rho_rows_column(rho_rows *rows, int index, rho_value *out);

`

// writeCQuery is one generated function.
//
// C has no slice to return, so a function that reads rows fills an array the
// caller owns and answers how many it filled. That is the shape a C caller
// expects, and it is the shape that makes the values reachable: a function that
// returned a count and nothing else would be one no caller could read.
//
// The values are read column by column into the row's own fields, in the order
// the columns were declared, which is the order the database returns them. One
// accessor per column and one conversion per type, so what a caller reads out of
// the row is the type the schema says it is.
func writeCQuery(b *strings.Builder, query GeneratedQuery, models map[string]Model) {
	writeCComment(b, query.Comment, "")
	for _, warning := range query.Warnings {
		fmt.Fprintf(b, "// Note: %s.\n", warning)
	}
	for _, line := range strings.Split(query.SQL, "\n") {
		fmt.Fprintf(b, "//   %s\n", strings.TrimRight(line, " "))
	}
	b.WriteString("//\n")

	name := lowerFirst(query.Name)
	fmt.Fprintf(b, "static const char *sql_%s = %s;\n\n", name, cString(query.SQL))

	// The signature takes the handle, one value per marker, and then where the
	// rows go: C has no way to return both an error and a value.
	//
	// A parameter is taken in the type its column has — the same type the
	// generated Go takes — and wrapped into the boundary's value inside the
	// function. A caller passing a number should pass a number, not build a
	// tagged union for it.
	parameters := []string{"rho_db *db"}
	for _, parameter := range query.Parameters {
		parameters = append(parameters,
			fmt.Sprintf("%s %s", cTypeOf(parameter.Type), lowerFirst(parameter.Name)))
	}
	model, returnsRows := models[query.Result]
	if returnsRows {
		parameters = append(parameters,
			fmt.Sprintf("%s *rows", model.Name), "int capacity")
	}
	parameters = append(parameters, "int *status")

	fmt.Fprintf(b, "int %s(%s) {\n", query.Name, strings.Join(parameters, ", "))
	fmt.Fprintf(b, "    rho_value values[%d];\n", len(query.Parameters))
	fmt.Fprintf(b, "    int count = %d;\n", len(query.Parameters))
	for i, parameter := range query.Parameters {
		fmt.Fprintf(b, "    values[%d] = %s;\n", i,
			cWrapped(parameter.Type, lowerFirst(parameter.Name)))
	}

	if returnsRows {
		writeCRows(b, name, model)
		return
	}
	fmt.Fprintf(b, "    long long affected = 0;\n")
	fmt.Fprintf(b, "    int rc = rho_db_exec(db, sql_%s, values, count, &affected);\n", name)
	b.WriteString("    if (rc != 0) { *status = rc; return 0; }\n")
	b.WriteString("    *status = 0;\n    return (int)affected;\n}\n\n")
}

// writeCRows is the body of a function that reads rows into a caller's array.
func writeCRows(b *strings.Builder, name string, model Model) {
	b.WriteString("    rho_rows *result = 0;\n")
	fmt.Fprintf(b, "    int rc = rho_db_query(db, sql_%s, values, count, &result);\n", name)
	b.WriteString("    if (rc != 0) { *status = rc; return 0; }\n\n")
	b.WriteString("    int wrote = 0;\n")
	b.WriteString("    while (wrote < capacity) {\n")
	b.WriteString("        int more = 0;\n")
	b.WriteString("        if (rho_rows_next(result, &more) != 0) { rho_rows_close(result); *status = 1; return wrote; }\n")
	b.WriteString("        if (!more) { break; }\n")
	b.WriteString("        rho_value column;\n")
	for i, field := range model.Fields {
		fieldName := lowerFirst(field.Name)
		fmt.Fprintf(b, "        rho_rows_column(result, %d, &column);\n", i)
		fmt.Fprintf(b, "        rows[wrote].%s = %s;\n", fieldName, cConversion(field.Type, "column"))
	}
	b.WriteString("        wrote++;\n")
	b.WriteString("    }\n")
	b.WriteString("    // A database that returned more rows than fit is a caller who asked for\n")
	b.WriteString("    // too few, and leaving them unread would be a result nobody reports.\n")
	b.WriteString("    rho_rows_close(result);\n")
	b.WriteString("    *status = 0;\n    return wrote;\n}\n\n")
}

// cWrapped is a parameter the caller gave, as the boundary's value.
//
// The caller passes the type its column has, and the kind is what the engine
// reads to know how to hand it to the database: a number passed as a number and
// a string passed as text, rather than one type that stands in for both.
func cWrapped(what Type, name string) string {
	switch what {
	case TypeInteger:
		return "(rho_value){ .kind = RHO_INTEGER, .integer = " + name + " }"
	case TypeFloat:
		return "(rho_value){ .kind = RHO_FLOAT, .real = " + name + " }"
	case TypeBoolean:
		return "(rho_value){ .kind = RHO_BOOLEAN, .boolean = " + name + " }"
	default:
		return "(rho_value){ .kind = RHO_TEXT, .text = " + name + " }"
	}
}

// cConversion is a value read out of a column, as the field it goes in.
//
// A value is read through the same accessors whichever type it is, and what
// differs is which member of it is taken: a number from its number, text from
// its text. Nothing is checked here, because the engine that filled the value
// already reported its kind and a C caller checking again would be checking the
// engine's own answer.
func cConversion(what Type, value string) string {
	switch what {
	case TypeInteger:
		return value + ".integer"
	case TypeFloat:
		return value + ".real"
	case TypeBoolean:
		return value + ".boolean"
	default:
		return value + ".text"
	}
}

// writeCHeader is what a generated C file opens with.
//
// The types the structs hold come from the file written beside them, so a
// statements file includes that rather than a header named for the package,
// which nothing here writes.
func writeCHeader(b *strings.Builder, pkg, about, file string, withHandle bool) {
	b.WriteString("/* Code generated by rho sql. DO NOT EDIT.\n *\n")
	for _, line := range strings.Split(about, "\n") {
		fmt.Fprintf(b, " * %s\n", line)
	}
	b.WriteString(" *\n * Regenerate it with: rho sql generate <project>\n */\n\n")
	fmt.Fprintf(b, "#ifndef %s\n#define %s\n\n", cGuard(pkg, file), cGuard(pkg, file))
	b.WriteString("#include <stdint.h>\n#include <stddef.h>\n\n")
	if withHandle {
		b.WriteString("#include \"models.h\"\n\n")
		b.WriteString(cHandle)
	}
}

// cGuard is the include guard a file carries. It names the file rather than the
// package, because two files of one package sharing a guard means whichever is
// included second is skipped whole — which would leave a translation unit that
// includes the types and has none of them.
func cGuard(pkg, file string) string {
	return "RHO_SQL_" + upperFirst(pkg) + "_" + upperFirst(file) + "_H"
}

// writeCComment is a description above a declaration, or nothing when there is
// none.
func writeCComment(b *strings.Builder, text, indent string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(b, "%s/* %s */\n", indent, strings.TrimRight(line, " "))
	}
}

// cTypeOf is the C type a column of this type is carried in.
func cTypeOf(what Type) string {
	if declared, found := cTypes[what]; found {
		return declared
	}
	return cTypes[TypeUnknown]
}

// cString is a C string literal, written so a statement with a quote in it is
// still a statement.
func cString(value string) string {
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
		case '%':
			// A percent in a C string is a format, and a statement may have one
			// in it without meaning to.
			b.WriteString(`%%`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// upperFirst is a guard's spelling of a name.
func upperFirst(name string) string {
	if name == "" {
		return "SQL"
	}
	first := name[0]
	if first >= 'a' && first <= 'z' {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}
