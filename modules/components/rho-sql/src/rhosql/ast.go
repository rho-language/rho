//
// @file ast.go
// @brief The SQL a service declares, as something other than SQL.
//
// A `.sql` file is written in one engine's dialect, and the difference between
// two engines is small but not zero: how a parameter is written, whether a
// statement can return the rows it changed, how a type is spelled. Those
// differences are the reason a service that works against sqlite and the same
// service against duckdb has to be written twice, and they are exactly the
// differences this shape does not carry.
//
// So what a service declares is parsed once into the statements below: the
// tables it has, the named queries it runs, the parameters each takes, and
// what each returns. That is the whole contract between a service and an
// engine. An engine is given this and decides what to write; a service never
// names an engine.
//
// The shape is deliberately narrow. It covers the statements a service actually
// writes — tables, indexes, and queries — and says so rather than pretending to
// be a SQL grammar. A construct this shape cannot represent is reported with the
// file and the line it was found on, rather than passed through as text a
// downstream tool has to guess at.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

// Package rhosql is the SQL a service declares, and the client that runs it.
//
// The two halves are separate on purpose. The parsing and the shape know
// nothing about any engine, and an engine knows nothing about Rho: it is handed
// this shape and answers with rows. That is what makes an engine a plugin rather
// than a dependency, and it is why a second engine costs a program rather than
// a rewrite.
package rhosql

// Version is the version of this shape. It rises when a field changes meaning
// or is removed, and not when one is added: an engine that does not recognise
// the version says so rather than reading a statement it half understands.
const Version = 1

// Schema is what a service declares about its data.
type Schema struct {
	// Version is this shape's version.
	Version int `json:"version"`
	// Module is the Rho module the schema belongs to.
	Module string `json:"module,omitempty"`
	// Description is what the project says about the data, which a reader of the
	// generated code is looking for.
	Description string `json:"description,omitempty"`
	// Tables are the tables the service has, in the order they were declared.
	Tables []Table `json:"tables"`
	// Queries are the named statements the service runs, in declaration order.
	Queries []Query `json:"queries"`
}

// Table is one table and the columns it holds.
type Table struct {
	// Name is the table's name as the service wrote it.
	Name string `json:"name"`
	// Comment is the comment written above the table, which is what a
	// developer reading the generated code is looking for.
	Comment string `json:"comment,omitempty"`
	// Columns are the columns, in declaration order.
	Columns []Column `json:"columns"`
	// PrimaryKey is the columns that form the key, in key order. It is empty for
	// a table without one, which is a fact about the table rather than an
	// absence of one.
	PrimaryKey []string `json:"primaryKey,omitempty"`
	// IfNotExists is whether the table was declared with one, which an engine
	// that applies a schema more than once needs to know.
	IfNotExists bool `json:"ifNotExists,omitempty"`
}

// Column is one column of a table.
type Column struct {
	Name string `json:"name"`
	// Type is the type as the service wrote it, kept as written rather than
	// mapped: the mapping is the engine's, because the same spelling means
	// different things in different engines and only the engine knows.
	Type    string `json:"type"`
	Comment string `json:"comment,omitempty"`
	// NotNull is whether the column refuses a null.
	NotNull bool `json:"notNull,omitempty"`
	// PrimaryKey is whether the column alone is the key.
	PrimaryKey bool `json:"primaryKey,omitempty"`
	// Default is the default expression as written, or "" when there is none.
	Default string `json:"default,omitempty"`
	// Unique is whether the column's values are unique.
	Unique bool `json:"unique,omitempty"`
}

// Query is one named statement in `queries.sql`.
type Query struct {
	// Name is the `-- name: find_customer` the statement is declared with. It
	// is the name the generated code and a caller use, so it is required: a
	// statement with no name is one nothing can call.
	Name string `json:"name"`
	// Comment is the prose written above the name, which is what a caller
	// reading the generated signature is looking for.
	Comment string `json:"comment,omitempty"`
	// Verb is what the statement does: select, insert, update or delete.
	Verb Verb `json:"verb"`
	// SQL is the statement's text, with the dialect-specific parts left as the
	// service wrote them. An engine that can use it as written will; one that
	// cannot renders it from the parts below.
	SQL string `json:"sql"`
	// Table is the table the statement reads or writes, when it is one table.
	Table string `json:"table,omitempty"`
	// Returning are the columns the statement returns, when it was written to
	// return them.
	Returning []string `json:"returning,omitempty"`
	// Parameters are what the statement takes, in the order the markers appear.
	// The order is the order a caller has to pass them in, so it is part of the
	// contract rather than an implementation detail.
	Parameters []Parameter `json:"parameters"`
	// ResultColumns are the columns the statement returns, which an engine
	// reports rather than this shape: what a query returns is a fact about the
	// database, not about the text.
	ResultColumns []Column `json:"resultColumns,omitempty"`
}

// Verb is what a statement does.
type Verb string

// The statements a query file may hold. Anything else is reported rather than
// passed through: a construct this shape does not model is a construct whose
// result columns a caller would be guessing at.
const (
	VerbSelect Verb = "select"
	VerbInsert Verb = "insert"
	VerbUpdate Verb = "update"
	VerbDelete Verb = "delete"
)

// Parameter is one value a statement takes.
type Parameter struct {
	// Name is the marker's name in a named-parameter dialect, or its position
	// spelled out where the dialect has no names.
	Name string `json:"name"`
	// Ordinal is the position the value is passed in, counted from one.
	Ordinal int `json:"ordinal"`
	// Type is the type the service wrote next to the marker, or "" where it
	// wrote none. A type here is a claim, not an inference: an engine reports
	// what a value actually was.
	Type string `json:"type,omitempty"`
	// Column is the column a value is compared against or written to, when the
	// statement names one, which is what lets an engine plan the query.
	Column string `json:"column,omitempty"`
	// In is which side of a comparison the value is on, for a query that is not
	// simply listing rows.
	In string `json:"in,omitempty"`
	// Marker is how the service wrote the parameter, so an engine that accepts
	// the text as written can.
	Marker Marker `json:"marker,omitempty"`
}

// Marker is how a parameter appears in the statement's text.
type Marker string

// The three ways a value is marked, which is most of what differs between
// engines.
const (
	// MarkerPositional is `?`, which sqlite and duckdb both take.
	MarkerPositional Marker = "positional"
	// MarkerNumbered is `$1`, which is what postgres takes.
	MarkerNumbered Marker = "numbered"
	// MarkerNamed is `:name` or `@name`, which sqlite takes and others do not.
	MarkerNamed Marker = "named"
)

// Diagnostic is a problem found while reading a service's SQL, reported
// against the file and line it was found on.
type Diagnostic struct {
	// Level is "error" for a statement that cannot be used and "warning" for
	// one that can.
	Level string `json:"level"`
	// File is the file the problem is in, as the service named it.
	File string `json:"file,omitempty"`
	// Line is the line it is on, counted from one, or 0 when it is not known.
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

// HasErrors reports whether any of the diagnostics is an error rather than a
// warning, which is what decides whether a schema is usable.
func HasErrors(diagnostics []Diagnostic) bool {
	for _, item := range diagnostics {
		if item.Level == "error" {
			return true
		}
	}
	return false
}

// Table returns one table by name, which is how a query's `Table` is resolved
// against what the service declared.
func (s Schema) Table(name string) (Table, bool) {
	for _, table := range s.Tables {
		if equalFold(table.Name, name) {
			return table, true
		}
	}
	return Table{}, false
}

// Query returns one named query.
func (s Schema) Query(name string) (Query, bool) {
	for _, query := range s.Queries {
		if query.Name == name {
			return query, true
		}
	}
	return Query{}, false
}

// Column returns one column of a table, matched without regard to case, which is
// how SQL names them.
func (t Table) Column(name string) (Column, bool) {
	for _, column := range t.Columns {
		if equalFold(column.Name, name) {
			return column, true
		}
	}
	return Column{}, false
}

// ColumnNames are the table's columns, in order, which is the order a row is in.
func (t Table) ColumnNames() []string {
	out := make([]string, 0, len(t.Columns))
	for _, column := range t.Columns {
		out = append(out, column.Name)
	}
	return out
}

// Parameter returns one parameter of a query by name, matched without regard to
// case, which is how SQL names them.
func (q Query) Parameter(name string) (Parameter, bool) {
	for _, parameter := range q.Parameters {
		if equalFold(parameter.Name, name) {
			return parameter, true
		}
	}
	return Parameter{}, false
}

// equalFold compares two SQL names without regard to case. SQL identifiers are
// not case sensitive in every engine and are in some, so a shape that is read
// across engines compares them the forgiving way: a lookup that fails where one
// spelling would have matched is a lookup that reports a column the service
// wrote.
func equalFold(a, b string) bool {
	// An empty name is not a name. Without this a lookup for a column that was
	// not given a name would find a column that was not given one, which is a
	// column the service never wrote.
	if a == "" || b == "" {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
