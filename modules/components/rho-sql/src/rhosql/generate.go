//
// @file generate.go
// @brief Generating static Go from what a service declared.
//
// A service writes SQL. This reads that SQL — the schema for the tables, the
// queries for the statements — and writes Go, so that a service which calls a
// query gets a typed function rather than a string of SQL and a bag of `any`.
//
// It is the shape sqlc made its point with, and it is here because the boundary
// this component already has makes it possible: the schema and the queries are
// one reading, and the engine that will run them is a plugin asked what a
// statement returns. So the generated code is typed by the engine's own reading
// of each statement, and a column whose type this cannot place is carried as
// text and reported, rather than guessed into a number that returns zeroes.
//
// What is generated, for a project that declares a statement named
// `find_customer` reading a customer's id and name by their address, is a
// function with a parameter per marker and a result struct per row. A handler
// calls it and gets back a list of structs rather than parsing JSON. The
// generated names come from the schema, the markers and the statement's own
// shape — the writer described the statement, and the description is the
// documentation of the function.
//
// What is NOT generated is the connection, the pooling, the engine. Those are a
// port, a host's business, and a generated file that opened a database would be a
// service that could not be moved to a different engine. So the generated code is
// written against `database/sql`, and the engine is behind it.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import (
	"sort"
	"strings"
)

// Generator writes Go from a project's SQL.
//
// It is given the reading and, where an engine is available, the engine's own
// reading of each statement; where there is not, it falls back to the schema's
// declared types and says which results it had to guess about.
type Generator struct {
	schema Schema
	// results is what each statement returns, when an engine has been asked.
	// A statement that is absent from it had its columns taken from the schema.
	results map[string][]ResultColumn
	// rendered is each statement in the engine's own spelling, when an engine
	// has been asked. A statement that is not in it keeps the text the service
	// wrote, markers and all, which is right: which markers an engine takes is
	// the engine's business, and a caller that has not asked has not been told.
	rendered map[string]string
	// guessed names the statements whose columns were taken from the schema
	// rather than from an engine, so a caller can be told what to check.
	guessed []string
	// package is the Go package the generated file belongs to.
	packageName string
}

// NewGenerator prepares a generator for a project.
//
// results is what `describe_query` answered for each statement, which is what
// the columns are typed from when it is there. It may be empty, in which case
// the schema's own declarations are used and every such statement is reported
// as guessed.
func NewGenerator(schema Schema, packageName string, results map[string][]ResultColumn) *Generator {
	if packageName == "" {
		packageName = "queries"
	}
	return &Generator{
		schema:      schema,
		results:     results,
		packageName: packageName,
	}
}

// WithRenderedSQL tells the generator the text each statement was rendered into,
// as an engine described it. A statement that is absent keeps the service's own
// text, because a statement with no engine behind it is a statement no caller
// has said which markers it takes.
func (g *Generator) WithRenderedSQL(rendered map[string]string) *Generator {
	g.rendered = rendered
	return g
}

// Generate is the Go a project gets from what it declared.
//
// Two files, because a generated result is a thing a caller wants to hold and a
// generated function is a thing a caller wants to call, and one of each is
// easier to read than both in one.
func (g *Generator) Generate() Result {
	result := Result{
		Package: g.packageName,
		Models:  g.models(),
		Queries: []GeneratedQuery{},
	}
	for _, query := range g.schema.Queries {
		result.Queries = append(result.Queries, g.query(query))
	}
	result.Guessed = g.guessed
	return result
}

// Result is what a generator produced.
type Result struct {
	// Package is the Go package the generated code belongs to.
	Package string
	// Models are the result structs, one per shape of row a project returns.
	Models []Model
	// Queries are the functions, one per named statement.
	Queries []GeneratedQuery
	// Guessed names the statements whose columns were taken from the schema
	// rather than from an engine. It is reported rather than hidden, because a
	// guessed type is a type a caller has no reason to trust until it is
	// checked.
	Guessed []string
}

// Model is one result struct: the shape of a row a statement returns.
type Model struct {
	// Name is the Go name, which is a statement's own or a table's.
	Name string
	// Comment is what the service wrote about the statement or the table, and it
	// is the documentation a reader of the generated file is looking for.
	Comment string
	// Fields are the columns, in the order the database returns them.
	Fields []Field
}

// Field is one column of a row.
type Field struct {
	// Name is the Go field name, which is the column's in a form Go accepts.
	Name string
	// Column is the column as the database calls it, which is what a row is read
	// by and what a `scan` names.
	Column string
	// Type is what the column carries, and what it is carried in.
	Type Type
	// Comment is what the service wrote beside the column.
	Comment string
}

// GeneratedQuery is one function, from one named statement.
type GeneratedQuery struct {
	// Name is the Go function name, which is the statement's.
	Name string
	// Comment is the statement's description, and it becomes the function's
	// documentation.
	Comment string
	// Verb is what the statement does, which is what the generated function is
	// for: a select is a read and a write is a write.
	Verb Verb
	// SQL is the statement as the service wrote it, with the markers left for
	// the engine to render — or, when the engine was asked, the dialect's own
	// spelling, which is what a prepared statement wants.
	SQL string
	// Parameters are what the function takes, in the order the markers appear.
	Parameters []Field
	// Result is the name of the model the function returns, or "" for a write
	// that changes rows rather than returning them.
	Result string
	// Affected is whether the function reports how many rows a write changed,
	// which is what a write that returns nothing else is judged by.
	Affected bool
	// Warnings are things a caller should know about this function: a statement
	// whose columns were guessed, a marker rewritten, a type that could not be
	// placed.
	Warnings []string
}

// models are the result structs a project gets.
//
// A statement's rows are a shape, and a shape gets a name. A statement that
// returns a table's columns gets that table's name, because it is that table's
// shape; anything else is named for the statement with a Row suffix, because a
// Go package cannot hold a struct and a function of the same name, and a caller
// reading a return value wants to know which query produced it.
func (g *Generator) models() []Model {
	byName := map[string]Model{}
	order := []string{}
	for _, query := range g.schema.Queries {
		if !query.ReturnsRows() {
			continue
		}
		columns := g.columnsFor(query)
		name, comment := g.modelName(query, columns)
		if !g.statementShape(query, columns) {
			// The name belongs to the function, so the shape is told apart by
			// what it is rather than by what it is called.
			name = name + "Row"
		}
		if _, seen := byName[name]; seen {
			continue
		}
		model := Model{Name: name, Comment: comment}
		for _, column := range columns {
			model.Fields = append(model.Fields, Field{
				Name:    goFieldName(column.Name),
				Column:  column.Name,
				Type:    ReduceType(column.Type),
				Comment: column.Comment,
			})
		}
		byName[name] = model
		order = append(order, name)
	}
	// A table with no statement returning all of it is still a thing a service
	// declared, and a caller writing a statement of its own against it wants the
	// shape. So every table gets a model too, named for the table.
	for _, table := range g.schema.Tables {
		name := goTypeName(table.Name)
		if _, seen := byName[name]; seen {
			continue
		}
		model := Model{Name: name, Comment: table.Comment}
		for _, column := range table.Columns {
			model.Fields = append(model.Fields, Field{
				Name:    goFieldName(column.Name),
				Column:  column.Name,
				Type:    ReduceType(column.Type),
				Comment: column.Comment,
			})
		}
		byName[name] = model
		order = append(order, name)
	}
	out := make([]Model, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

// statementShape reports whether a statement's rows are a table's own shape, in
// which case the shape is named for the table and the function for the
// statement, and there is nothing to disambiguate.
func (g *Generator) statementShape(query Query, columns []ResultColumn) bool {
	table, found := g.schema.Table(query.Table)
	if !found {
		return false
	}
	return sameColumns(columns, table.ColumnNames())
}

// modelName is what a statement's rows are called, and what it says about them.
func (g *Generator) modelName(query Query, columns []ResultColumn) (name, comment string) {
	// A statement that returns exactly one table's columns, in the table's order,
	// is that table's rows. Nothing is safer to hand a caller than the table they
	// asked for, and nothing is more likely to be right.
	if table, found := g.schema.Table(query.Table); found {
		if sameColumns(columns, table.ColumnNames()) {
			return goTypeName(table.Name), table.Comment
		}
	}
	return goTypeName(query.Name), query.Comment
}

// columnsFor are the columns a statement returns: the engine's own reading of a
// select when it answered, and the schema's declaration otherwise.
//
// A write is never asked of the engine, because it names the columns it returns
// in its RETURNING clause and running it to learn them would be a write done for
// a fact already in hand — against a database the caller did not ask to change.
func (g *Generator) columnsFor(query Query) []ResultColumn {
	if columns, found := g.results[query.Name]; found && len(columns) > 0 {
		out := make([]ResultColumn, 0, len(columns))
		for _, column := range columns {
			// The engine names a column's type; the schema, where it declares
			// one, names what it means. A comment is a thing a service wrote
			// beside a column, and only the schema has it.
			column.Comment = ""
			if table, found := g.schema.Table(query.Table); found {
				if declared, ok := table.Column(column.Name); ok {
					column.Comment = declared.Comment
					if column.Type == "" {
						column.Type = declared.Type
					}
				}
			}
			out = append(out, column)
		}
		return out
	}
	// A write says which columns it returns, and the schema says what they are.
	if query.Verb != VerbSelect {
		table, found := g.schema.Table(query.Table)
		if !found {
			return nil
		}
		out := []ResultColumn{}
		for _, name := range query.Returning {
			if column, ok := table.Column(name); ok {
				out = append(out, ResultColumn{
					Name: column.Name, Type: column.Type, Comment: column.Comment,
				})
			}
		}
		return out
	}

	// No engine, so the columns are the table's. A statement that does not select
	// the whole table cannot be typed from it, and is left with what the schema
	// says about the columns it does name.
	g.markGuessed(query)
	table, found := g.schema.Table(query.Table)
	if !found {
		return nil
	}
	if returnsEverything(query, table) {
		return allColumns(table)
	}
	out := []ResultColumn{}
	for _, name := range selectedColumns(query) {
		if column, ok := table.Column(name); ok {
			out = append(out, ResultColumn{Name: column.Name, Type: column.Type, Comment: column.Comment})
		}
	}
	return out
}

// markGuessed notes that a statement's columns were taken from the schema.
func (g *Generator) markGuessed(query Query) {
	for _, seen := range g.guessed {
		if seen == query.Name {
			return
		}
	}
	g.guessed = append(g.guessed, query.Name)
}

// query is one function.
//
// A statement that cannot be typed still gets a function, because a caller who
// has to write the query by hand is worse off than a caller with a function that
// says what it could not work out. The note is on the function, where a reader
// of the generated file sees it.
func (g *Generator) query(query Query) GeneratedQuery {
	// The statement is written the way the engine will take it, when the engine
	// has said so; otherwise the way the service wrote it.
	statement := query.SQL
	if rendered, found := g.rendered[query.Name]; found && rendered != "" {
		statement = rendered
	}
	generated := GeneratedQuery{
		Name:     goFuncName(query.Name),
		Comment:  query.Comment,
		Verb:     query.Verb,
		SQL:      statement,
		Affected: query.Verb != VerbSelect && !query.ReturnsRows(),
	}
	if !query.ReturnsRows() {
		return generated
	}
	columns := g.columnsFor(query)
	if len(columns) == 0 {
		generated.Warnings = append(generated.Warnings,
			"the columns this returns could not be worked out, so the result is "+
				"a list of a shape nobody can read")
	}
	// The shape is looked up by the name the function will return, which is the
	// table's when the statement returns a whole table and the statement's own
	// with a Row suffix when it does not.
	result, _ := g.modelName(query, columns)
	if !g.statementShape(query, columns) {
		result += "Row"
	}
	generated.Result = result
	for _, parameter := range query.Parameters {
		generated.Parameters = append(generated.Parameters, Field{
			Name:   goFieldName(parameter.Name),
			Column: parameter.Name,
			Type:   parameterType(g, query, parameter),
		})
	}
	// A value the mapping could not place is carried as text and said here, so a
	// caller reading the generated file knows which values to check rather than
	// finding zeroes later.
	for _, column := range columns {
		if ReduceType(column.Type) == TypeUnknown {
			generated.Warnings = append(generated.Warnings,
				"the type of "+column.Name+" is "+typeOrUnknown(column.Type)+
					", which is not one this knows; it is carried as text")
		}
	}
	return generated
}

// parameterType is what a parameter is carried in, which is the column it is
// compared against where the statement names one.
//
// A parameter with no column behind it is text, because that is what the caller
// has: a value in a predicate the statement does not tie to a column is a value
// this cannot type, and text is the type that keeps it.
func parameterType(g *Generator, query Query, parameter Parameter) Type {
	if parameter.Column != "" {
		if table, found := g.schema.Table(query.Table); found {
			if column, ok := table.Column(parameter.Column); ok {
				return ReduceType(column.Type)
			}
		}
	}
	return TypeText
}

// ReturnsRows reports whether a statement returns rows, which is what decides
// whether it gets a result and a function that reads rather than writes.
func (q Query) ReturnsRows() bool {
	return q.Verb == VerbSelect || len(q.Returning) > 0
}

// allColumns are a table's columns as result columns.
func allColumns(table Table) []ResultColumn {
	out := make([]ResultColumn, 0, len(table.Columns))
	for _, column := range table.Columns {
		out = append(out, ResultColumn{
			Name: column.Name, Type: column.Type, Comment: column.Comment,
		})
	}
	return out
}

// sameColumns reports whether a statement's columns are a table's, in order.
func sameColumns(columns []ResultColumn, names []string) bool {
	if len(columns) != len(names) {
		return false
	}
	for i, column := range columns {
		if !strings.EqualFold(column.Name, names[i]) {
			return false
		}
	}
	return true
}

// returnsEverything reports whether a statement selects a whole table, which is
// what lets its rows be the table's rows.
func returnsEverything(query Query, table Table) bool {
	selected := selectedColumns(query)
	return len(selected) == len(table.Columns) && sameNames(selected, table.ColumnNames())
}

// selectedColumns are the columns a statement names, read from its select list.
//
// A `select *` is every column, which is the one case a reader is right about
// without being told.
func selectedColumns(query Query) []string {
	upper := strings.ToUpper(query.SQL)
	open := strings.Index(upper, "SELECT")
	if open < 0 {
		return nil
	}
	rest := query.SQL[open+len("SELECT"):]
	if from := indexWord(upper, "FROM"); from >= 0 {
		rest = rest[:from]
	}
	rest = strings.TrimSpace(rest)
	if rest == "*" {
		table, _ := lookupTable(query)
		return table
	}
	out := []string{}
	for _, part := range strings.Split(rest, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// An `alias` or an expression is a column with a name the service
		// chose, which is a thing a caller is given rather than looked up.
		if index := strings.Index(part, " "); index > 0 {
			part = part[:index]
		}
		out = append(out, strings.Trim(part, `"`))
	}
	return out
}

// lookupTable is the table a statement reads, and its columns.
func lookupTable(query Query) ([]string, bool) {
	if query.Table == "" {
		return nil, false
	}
	return nil, true
}

// sameNames reports whether two column lists are the same, without regard to
// order.
func sameNames(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := map[string]bool{}
	for _, name := range left {
		seen[strings.ToLower(name)] = true
	}
	for _, name := range right {
		if !seen[strings.ToLower(name)] {
			return false
		}
	}
	return true
}

// indexWord is where a word appears in a statement in upper case, at a boundary.
func indexWord(upper, word string) int {
	at := 0
	for {
		index := strings.Index(upper[at:], word)
		if index < 0 {
			return -1
		}
		start := at + index
		end := start + len(word)
		before := start == 0 || !isTypeByte(upper[start-1])
		after := end == len(upper) || !isTypeByte(upper[end])
		if before && after {
			return start
		}
		at = end
	}
}

func typeOrUnknown(declared string) string {
	if declared == "" {
		return "not one it names"
	}
	return declared
}

/* -------------------------------------------------------------------------- */
/*  Names                                                                       */
/* -------------------------------------------------------------------------- */

// goFuncName is a statement's name as a Go function.
//
// A service names a statement `find_customer`; a Go function is `FindCustomer`.
// The service's name is kept in the comment above the function, because a caller
// reading the SQL file should be able to find the function it wrote.
func goFuncName(name string) string { return goExported(name) }

// goTypeName is a table's or a statement's name as a Go type.
func goTypeName(name string) string { return goExported(name) }

// goFieldName is a column's name as a Go field.
//
// A field name is the one that has to be a valid identifier, so a column whose
// name is not one is adjusted: a name a Go programmer cannot write is a name a
// generated struct cannot have.
func goFieldName(name string) string {
	words := splitWords(name)
	for i, word := range words {
		words[i] = strings.Title(word)
	}
	out := strings.Join(words, "")
	if out == "" {
		return "Field"
	}
	// A field that starts with a digit cannot be an identifier, and a column
	// called `2fa` is a thing a service writes.
	if out[0] >= '0' && out[0] <= '9' {
		return "N" + out
	}
	return out
}

// goExported is a name in the form Go exports: capitalised, no separators.
func goExported(name string) string {
	words := splitWords(name)
	var b strings.Builder
	for _, word := range words {
		b.WriteString(strings.Title(word))
	}
	if b.Len() == 0 {
		return "Unnamed"
	}
	text := b.String()
	if text[0] >= '0' && text[0] <= '9' {
		return "N" + text
	}
	return text
}

// splitWords are the words a name is made of, whatever separator it used, so
// `find_customer`, `find-customer` and `findCustomer` are one name.
func splitWords(name string) []string {
	out := []string{}
	current := strings.Builder{}
	for _, r := range name {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			if current.Len() > 0 {
				out = append(out, current.String())
				current.Reset()
			}
		case r >= 'A' && r <= 'Z' && current.Len() > 0:
			// A capital in the middle of a name starts a word, which is how a
			// camelCase name is read.
			if last := current.String()[len(current.String())-1]; last >= 'a' && last <= 'z' {
				out = append(out, current.String())
				current.Reset()
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

// sortedNames are names in order, for a generated file that is the same on every
// run.
func sortedNames(names []string) []string {
	out := append([]string{}, names...)
	sort.Strings(out)
	return out
}
