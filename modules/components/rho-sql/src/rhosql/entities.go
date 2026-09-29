//
// @file entities.go
// @brief Tables declared as data, and what follows from them.
//
// A service should not have to write SQL to have typed access to its own tables.
// Declaring a table is declaring a name, its columns and what each holds; the
// create statement and the operations over it are a consequence of that, and a
// person who writes them by hand writes the same five statements for every table
// and gets one of them subtly wrong.
//
// So a project declares its tables in YAML:
//
//	tables:
//	  - name: users
//	    description: The identities the registry knows.
//	    columns:
//	      - name: id
//	        type: integer
//	        primary_key: true
//	        auto: true
//	      - name: name
//	        type: text
//	        not_null: true
//	        unique: true
//
// and gets three things from it: the create statements an engine applies to
// create the database, one typed Rho function per common operation, and the row
// types those functions use. The declarations are validated here rather than
// discovered later, because a table whose primary key is a text column nobody
// can index is a fact about the declaration and not a surprise for whoever runs
// the create.
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
	"strings"

	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  The declarations                                                          */
/* -------------------------------------------------------------------------- */

// EntityFile is the extension a project's tables are declared with. A schema
// written as SQL is still read; this is what says the file is declarations
// rather than statements.
const EntityFile = ".yml"

// IsEntitySchema reports whether a schema file holds declarations.
//
// The distinction is the file, not the contents: a project that wrote SQL asked
// for its SQL to be read, and a project that wrote declarations asked for them to
// be read. Guessing from the first character would make a file whose first line
// is a comment ambiguous.
func IsEntitySchema(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")
}

// entityRootFields are the keys a declaration file may hold.
var entityRootFields = []string{"tables"}

// entityTableFields are the keys a table declaration may hold.
var entityTableFields = []string{"name", "description", "columns"}

// entityColumnFields are the keys a column declaration may hold.
var entityColumnFields = []string{
	"name", "type", "description", "primary_key", "auto", "not_null",
	"unique", "default", "references",
}

// entityTypes are the types a column may be declared with, and what each becomes
// in the statement an engine is handed.
//
// The vocabulary is a meaning rather than a storage class: `number` is a number
// the database keeps exactly, `text` is text of any length. What a dialect spells
// them is this table's business, not the declaration's, so a project that moves
// from one engine to another changes its driver and not its tables.
var entityTypes = map[string]string{
	"integer":   "INTEGER",
	"int":       "INTEGER",
	"text":      "TEXT",
	"string":    "TEXT",
	"float":     "REAL",
	"real":      "REAL",
	"double":    "REAL",
	"boolean":   "BOOLEAN",
	"bool":      "BOOLEAN",
	"bytes":     "BLOB",
	"blob":      "BLOB",
	"time":      "TIMESTAMP",
	"timestamp": "TIMESTAMP",
	"datetime":  "TIMESTAMP",
	"json":      "JSON",
	"number":    "NUMERIC",
	"decimal":   "NUMERIC",
	"numeric":   "NUMERIC",
}

/* -------------------------------------------------------------------------- */
/*  Reading                                                                   */
/* -------------------------------------------------------------------------- */

// ParseEntities reads a declaration file into the tables a generator works from.
//
// Every problem is reported rather than the first: a declaration file is written
// whole, and a person fixing it wants the list rather than a series of runs.
func ParseEntities(name, text string) ([]Table, []Diagnostic) {
	reader := &entityReader{name: name}
	document, err := yaml.Load(text)
	if err != nil {
		reader.report(1, fmt.Sprintf("the declarations cannot be read: %v", err))
		return nil, reader.diags
	}
	if document.Kind != yaml.KindMap {
		reader.report(1, "the declarations must be a mapping of `tables:` to a list")
		return nil, reader.diags
	}
	reader.rejectUnknown(document, entityRootFields, "the declarations")

	tablesNode, found := document.Map["tables"]
	if !found || tablesNode.Kind != yaml.KindList {
		reader.report(1, "`tables:` is required, and must be a list of tables")
		return nil, reader.diags
	}
	if len(tablesNode.List) == 0 {
		reader.report(1, "`tables:` names no table; a declaration with nothing in it declares nothing")
		return nil, reader.diags
	}

	tables := make([]Table, 0, len(tablesNode.List))
	seen := map[string]bool{}
	for index, entry := range tablesNode.List {
		table, ok := reader.table(entry, index)
		if !ok {
			continue
		}
		if seen[table.Name] {
			reader.report(1, fmt.Sprintf("the table %q is declared twice", table.Name))
			continue
		}
		seen[table.Name] = true
		tables = append(tables, table)
	}
	return tables, reader.diags
}

// entityReader reads declarations and collects what is wrong with them.
type entityReader struct {
	name  string
	diags []Diagnostic
}

func (r *entityReader) report(line int, message string) {
	r.diags = append(r.diags, Diagnostic{File: r.name, Line: line, Level: "error", Message: message})
}

// rejectUnknown reports a key the declarations do not define.
//
// A key that is silently ignored is a key a person believes took effect. The
// list of keys is deliberately closed: a declaration is read by this command
// alone, so a key it does not know is a mistake rather than an extension.
func (r *entityReader) rejectUnknown(node yaml.Value, allowed []string, where string) {
	if node.Kind != yaml.KindMap {
		return
	}
	for key := range node.Map {
		known := false
		for _, candidate := range allowed {
			if key == candidate {
				known = true
				break
			}
		}
		if !known {
			r.report(1, fmt.Sprintf("%s does not take %q; it takes %s",
				where, key, strings.Join(allowed, ", ")))
		}
	}
}

// table reads one table declaration.
func (r *entityReader) table(node yaml.Value, index int) (Table, bool) {
	where := fmt.Sprintf("tables[%d]", index)
	if node.Kind != yaml.KindMap {
		r.report(1, where+" must be a mapping")
		return Table{}, false
	}
	r.rejectUnknown(node, entityTableFields, where)

	table := Table{Columns: []Column{}}
	table.Name = strings.TrimSpace(node.Map["name"].Str)
	table.Comment = strings.TrimSpace(node.Map["description"].Str)
	if table.Name == "" {
		r.report(1, where+" must name the table it declares")
		return Table{}, false
	}
	if !isIdentifier(table.Name) {
		r.report(1, fmt.Sprintf("the table name %q is not a name a database takes: letters, digits and underscores, not starting with a digit", table.Name))
		return Table{}, false
	}

	columnsNode, found := node.Map["columns"]
	if !found || columnsNode.Kind != yaml.KindList {
		r.report(1, fmt.Sprintf("the table %q must declare `columns:` as a list", table.Name))
		return Table{}, false
	}
	if len(columnsNode.List) == 0 {
		r.report(1, fmt.Sprintf("the table %q declares no column", table.Name))
		return Table{}, false
	}

	seen := map[string]bool{}
	autos := 0
	for columnIndex, entry := range columnsNode.List {
		column, ok := r.column(entry, table.Name, columnIndex)
		if !ok {
			continue
		}
		if seen[column.Name] {
			r.report(1, fmt.Sprintf("the table %q declares the column %q twice", table.Name, column.Name))
			continue
		}
		seen[column.Name] = true
		if column.Auto {
			autos++
			if !column.PrimaryKey {
				r.report(1, fmt.Sprintf("the column %s.%s is generated by the database and is not the key; only a key can be", table.Name, column.Name))
			}
		}
		table.Columns = append(table.Columns, column)
	}
	if autos > 1 {
		r.report(1, fmt.Sprintf("the table %q declares more than one generated column", table.Name))
	}

	// A key of one column is written on that column and a key of several is a
	// table constraint, which is where a reader looks for each. The two are not
	// both written: a table that stated its key twice would be refused by the
	// database it is created in.
	keys := []string{}
	for _, column := range table.Columns {
		if column.PrimaryKey {
			keys = append(keys, column.Name)
		}
	}
	if len(keys) > 1 {
		table.PrimaryKey = keys
		for index := range table.Columns {
			table.Columns[index].PrimaryKey = false
		}
	}
	return table, true
}

// column reads one column declaration.
func (r *entityReader) column(node yaml.Value, table string, index int) (Column, bool) {
	where := fmt.Sprintf("the column %s.columns[%d]", table, index)
	if node.Kind != yaml.KindMap {
		r.report(1, where+" must be a mapping")
		return Column{}, false
	}
	r.rejectUnknown(node, entityColumnFields, where)

	column := Column{}
	column.Name = strings.TrimSpace(node.Map["name"].Str)
	column.Comment = strings.TrimSpace(node.Map["description"].Str)
	if column.Name == "" {
		r.report(1, where+" must name the column it declares")
		return Column{}, false
	}
	if !isIdentifier(column.Name) {
		r.report(1, fmt.Sprintf("the column name %q is not a name a database takes", column.Name))
		return Column{}, false
	}

	declared := strings.ToLower(strings.TrimSpace(node.Map["type"].Str))
	if declared == "" {
		r.report(1, fmt.Sprintf("the column %s.%s must declare a `type:`", table, column.Name))
		return Column{}, false
	}
	sqlType, known := entityTypes[declared]
	if !known {
		r.report(1, fmt.Sprintf(
			"the column %s.%s is declared as %q, which is not a type these declarations take; they take %s",
			table, column.Name, declared, strings.Join(sortedKeys(entityTypes), ", ")))
		return Column{}, false
	}
	column.Type = sqlType
	column.PrimaryKey = yamlTrue(node, "primary_key")
	column.NotNull = yamlTrue(node, "not_null")
	column.Unique = yamlTrue(node, "unique")
	column.Auto = yamlTrue(node, "auto")
	column.Default = normaliseDefault(strings.TrimSpace(node.Map["default"].Str))
	column.References = strings.TrimSpace(node.Map["references"].Str)
	if column.Unique && column.PrimaryKey {
		// The key is unique already, and a second unique index over it is a
		// second thing to keep in step rather than a second guarantee.
		column.Unique = false
	}
	return column, true
}

/* -------------------------------------------------------------------------- */
/*  The create statements                                                     */
/* -------------------------------------------------------------------------- */

// EntitiesDDL is the schema the tables become: the statements an engine applies
// to create a database that holds them.
//
// It is written from the declarations so that the file an engine is handed and
// the code generated from it are one reading of one file. A project that kept
// the two apart would have a schema it created and a schema its code assumed,
// and the difference would be found by whoever ran the second one.
func EntitiesDDL(tables []Table) string {
	var b strings.Builder
	b.WriteString("-- Generated from the declarations in the project's schema file.\n")
	b.WriteString("-- This is what the service creates, and what its generated code assumes.\n")
	for _, table := range tables {
		b.WriteString("\n")
		if table.Comment != "" {
			for _, line := range strings.Split(table.Comment, "\n") {
				fmt.Fprintf(&b, "-- %s\n", strings.TrimRight(line, " "))
			}
		}
		fmt.Fprintf(&b, "CREATE TABLE IF NOT EXISTS %s (\n", table.Name)
		definitions := make([]string, 0, len(table.Columns)+1)
		for _, column := range table.Columns {
			definitions = append(definitions, columnDefinition(column))
		}
		// A composite key is a constraint rather than a column property, so it
		// is written after the columns. A single key is written on its column,
		// because that is where a reader looks for it.
		if len(table.PrimaryKey) > 1 {
			definitions = append(definitions, "PRIMARY KEY ("+strings.Join(table.PrimaryKey, ", ")+")")
		}
		b.WriteString("\t" + strings.Join(definitions, ",\n\t") + "\n")
		b.WriteString(");\n")
	}
	return b.String()
}

// columnDefinition is one column as the create statement writes it.
func columnDefinition(column Column) string {
	var b strings.Builder
	b.WriteString(column.Name)
	b.WriteString(" ")
	b.WriteString(column.Type)
	if column.PrimaryKey {
		// A key this database generates is the row's identity rather than a value
		// the caller supplies, and `INTEGER PRIMARY KEY AUTOINCREMENT` is how it
		// is asked for.
		if column.Auto {
			b.WriteString(" PRIMARY KEY AUTOINCREMENT")
		} else {
			b.WriteString(" PRIMARY KEY")
		}
	}
	if column.NotNull && !column.PrimaryKey {
		// A key is not null already, and saying it twice is noise.
		b.WriteString(" NOT NULL")
	}
	if column.Unique {
		b.WriteString(" UNIQUE")
	}
	if column.References != "" {
		b.WriteString(" REFERENCES ")
		b.WriteString(column.References)
	}
	if column.Default != "" {
		b.WriteString(" DEFAULT ")
		b.WriteString(column.Default)
	}
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  The operations                                                            */
/* -------------------------------------------------------------------------- */

// CRUDQueries are the common operations over a set of tables, as named
// statements the rest of the pipeline reads like any other.
//
// They are statements rather than a second code path on purpose: a project that
// wrote its own `-- name: list_users` gets the same generated function as the one
// this synthesises, so the two are the same thing and a table's operations need
// no special case anywhere.
//
// What is generated is the minimum a caller cannot do without: read a row by its
// key, read them all, add one, change one, remove one. A statement that is not
// one of those is a statement about a service's domain rather than about its
// tables, and it belongs in the queries file where the service can say what it
// means.
func CRUDQueries(tables []Table) []Query {
	out := []Query{}
	for _, table := range tables {
		out = append(out, tableQueries(table)...)
	}
	return out
}

// tableQueries are the operations over one table.
func tableQueries(table Table) []Query {
	key, hasKey := singleKey(table)
	every := table.ColumnNames()
	// A column the database generates is not a value a caller passes to an
	// insert; the database is asked for it.
	insertable := make([]string, 0, len(table.Columns))
	for _, column := range table.Columns {
		if column.Auto {
			continue
		}
		insertable = append(insertable, column.Name)
	}

	queries := []Query{}
	if len(insertable) > 0 {
		queries = append(queries, Query{
			Name:    "insert_" + table.Name,
			Comment: fmt.Sprintf("Adds one row to %s, and answers how many it added.", table.Name),
			Verb:    VerbInsert,
			Table:   table.Name,
			SQL: fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
				table.Name,
				strings.Join(insertable, ", "),
				strings.Join(markers(len(insertable)), ", ")),
			Parameters: parametersFor(table, insertable),
		})
	}

	if hasKey {
		queries = append(queries, Query{
			Name:       "get_" + table.Name,
			Comment:    fmt.Sprintf("The row of %s with this %s, or no rows.", table.Name, key),
			Verb:       VerbSelect,
			Table:      table.Name,
			Returning:  every,
			SQL:        fmt.Sprintf("SELECT %s FROM %s WHERE %s = ?", strings.Join(every, ", "), table.Name, key),
			Parameters: parametersFor(table, []string{key}),
		})
	}

	queries = append(queries, Query{
		Name:      "list_" + table.Name,
		Comment:   fmt.Sprintf("Every row of %s.", table.Name),
		Verb:      VerbSelect,
		Table:     table.Name,
		Returning: every,
		SQL:       fmt.Sprintf("SELECT %s FROM %s", strings.Join(every, ", "), table.Name),
	})

	if hasKey {
		updatable := make([]string, 0, len(table.Columns))
		for _, column := range table.Columns {
			if column.Name == key || column.Auto {
				continue
			}
			updatable = append(updatable, column.Name)
		}
		if len(updatable) > 0 {
			assignments := make([]string, 0, len(updatable))
			for _, column := range updatable {
				assignments = append(assignments, column+" = ?")
			}
			queries = append(queries, Query{
				Name:    "update_" + table.Name,
				Comment: fmt.Sprintf("Changes the row of %s with this %s, and answers how many it changed.", table.Name, key),
				Verb:    VerbUpdate,
				Table:   table.Name,
				SQL: fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?",
					table.Name, strings.Join(assignments, ", "), key),
				Parameters: parametersFor(table, append(append([]string{}, updatable...), key)),
			})
		}

		queries = append(queries, Query{
			Name:       "delete_" + table.Name,
			Comment:    fmt.Sprintf("Removes the row of %s with this %s, and answers how many it removed.", table.Name, key),
			Verb:       VerbDelete,
			Table:      table.Name,
			SQL:        fmt.Sprintf("DELETE FROM %s WHERE %s = ?", table.Name, key),
			Parameters: parametersFor(table, []string{key}),
		})
	}
	return queries
}

// singleKey is the one column that identifies a row, when there is exactly one.
//
// A key of one column is written on the column and a key of several is a table
// constraint, so both are asked. A table keyed by more than one column has no
// `get` or `delete` by key, because "the row with this value" is not a question
// a composite key can answer: two values are needed and a generated function
// that took one would silently take the first column of the key and be wrong for
// every row that shared it.
func singleKey(table Table) (string, bool) {
	if len(table.PrimaryKey) == 1 {
		return table.PrimaryKey[0], true
	}
	if len(table.PrimaryKey) > 1 {
		return "", false
	}
	found := ""
	for _, column := range table.Columns {
		if !column.PrimaryKey {
			continue
		}
		if found != "" {
			// Two columns each said they were the key, which is a table with no
			// single way to name a row.
			return "", false
		}
		found = column.Name
	}
	return found, found != ""
}

// parametersFor are the values a statement takes, in order, each typed by the
// column it is written for.
//
// The column is recorded rather than inferred from the text, because a value
// written into a `VALUES` list has no column beside it and a reader of the
// statement could only guess at one.
func parametersFor(table Table, names []string) []Parameter {
	out := make([]Parameter, 0, len(names))
	for index, name := range names {
		out = append(out, Parameter{
			Name:    name,
			Ordinal: index + 1,
			Column:  name,
			Marker:  MarkerPositional,
		})
	}
	return out
}

// markers are the placeholders a statement with this many values is written
// with. A positional marker is the one every engine this generates for takes.
func markers(count int) []string {
	out := make([]string, count)
	for index := range out {
		out[index] = "?"
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

// yamlTrue reports whether a declaration wrote `true` for a flag.
func yamlTrue(node yaml.Value, key string) bool {
	value, found := node.Map[key]
	if !found {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value.Str), "true")
}

// normaliseDefault is a declared default as a create statement takes it.
//
// A default is either a literal — a number, a quoted string — or an expression
// the database evaluates. The second has to be parenthesised, because that is
// how a create statement distinguishes it from the first: `DEFAULT datetime('now')`
// is a syntax error and `DEFAULT (datetime('now'))` is a default of the current
// time. A quoted literal is left alone, and so is anything already written with
// the parentheses.
func normaliseDefault(value string) string {
	if value == "" {
		return ""
	}
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "(") || strings.HasPrefix(trimmed, "'") ||
		strings.HasPrefix(trimmed, `"`) {
		return trimmed
	}
	// A bare number or keyword is a literal and needs nothing; an expression is
	// anything with a call or an operator in it.
	if strings.ContainsAny(trimmed, "()+-*/ ") {
		return "(" + trimmed + ")"
	}
	return trimmed
}

// isIdentifier reports whether a name is one a database and a language both take.
func isIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for index, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9':
			if index == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// sortedKeys is a map's keys in a stable order, so a message about which values
// are accepted reads the same way twice.
func sortedKeys(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
