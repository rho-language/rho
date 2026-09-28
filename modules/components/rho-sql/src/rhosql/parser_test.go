//
// @file parser_test.go
// @brief Tests for the reader a service's SQL is read with.
//
// The reader is where a `.sql` file is turned into something a service can be
// generated from, so what matters is that it reads what was written. The cases
// here are the ones a first reading gets wrong and a second reading gets right:
// a comment that belongs to the column before it, a default that contains the
// separator, a type that contains one, a name that is a keyword.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import (
	"strings"
	"testing"
)

func TestACommentBelongsToTheColumnItWasWrittenBeside(t *testing.T) {
	// SQL writes a column's comment after its comma, so the comment sits between
	// two separators and documents the piece that was just finished. A reader that
	// attaches it forward documents the wrong column, which is worse than not
	// documenting it at all.
	schema, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE t (\n"+
			"    id    integer PRIMARY KEY,   -- the identity\n"+
			"    name  text NOT NULL,        -- what to call it\n"+
			"    note  text\n"+
			");\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	if len(schema.Tables) != 1 {
		t.Fatalf("read %d tables", len(schema.Tables))
	}
	columns := schema.Tables[0].Columns
	if len(columns) != 3 {
		t.Fatalf("read %d columns, want 3: %+v", len(columns), columns)
	}
	if columns[0].Comment != "the identity" {
		t.Errorf("the id column is described as %q; a comment written after its comma documents it", columns[0].Comment)
	}
	if columns[1].Comment != "what to call it" {
		t.Errorf("the name column is described as %q", columns[1].Comment)
	}
}

func TestACommentMayBeAboveTheColumnItDescribes(t *testing.T) {
	// A description that does not fit beside a column is written above it, and
	// a reader that only looks to the right of a column misses it.
	schema, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE t (\n"+
			"    id   integer PRIMARY KEY,\n"+
			"    -- the customer's name, in full\n"+
			"    name text NOT NULL\n"+
			");\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	columns := schema.Tables[0].Columns
	for _, column := range columns {
		if column.Name == "name" && column.Comment != "the customer's name, in full" {
			t.Errorf("the name column is described as %q", column.Comment)
		}
	}
}

func TestACommentBesideATableIsItsOwn(t *testing.T) {
	schema, diagnostics := ParseSchema("schema.sql",
		"-- Customers of the catalogue.\nCREATE TABLE customers (id integer PRIMARY KEY);\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	if schema.Tables[0].Comment != "Customers of the catalogue." {
		t.Errorf("the table is described as %q", schema.Tables[0].Comment)
	}
}

func TestATypeMayContainTheSeparator(t *testing.T) {
	// `numeric(10, 2)` is one type with a scale, and a reader that splits the
	// column list on every comma makes it two columns.
	schema, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE t (balance numeric(10, 2) NOT NULL);\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	columns := schema.Tables[0].Columns
	if len(columns) != 1 {
		t.Fatalf("a type with a scale made %d columns: %+v", len(columns), columns)
	}
	if columns[0].Type != "numeric(10, 2)" {
		t.Errorf("the type read as %q, want the whole of it", columns[0].Type)
	}
}

func TestADefaultMayContainTheSeparator(t *testing.T) {
	// A default is an expression, and an expression may be a list or a string
	// with a comma in it.
	for _, declared := range []string{
		"text DEFAULT 'a,b'",
		"integer NOT NULL DEFAULT 20",
		"text DEFAULT (1 + 2)",
	} {
		schema, diagnostics := ParseSchema("schema.sql",
			"CREATE TABLE t (id integer PRIMARY KEY, value "+declared+");\n")
		if HasErrors(diagnostics) {
			t.Errorf("%q did not read: %+v", declared, diagnostics)
			continue
		}
		columns := schema.Tables[0].Columns
		if len(columns) != 2 {
			t.Errorf("%q made %d columns", declared, len(columns))
			continue
		}
		if columns[1].Default == "" {
			t.Errorf("%q lost its default", declared)
		}
	}
}

func TestASemicolonInsideAStringIsNotTheEndOfAStatement(t *testing.T) {
	// A default of 'a;b' ends a statement if the reader does not know a string
	// ends at its closing quote, and a schema cut in half describes a different
	// database from the one the service creates.
	schema, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE t (\n"+
			"  id   integer PRIMARY KEY,\n"+
			"  note text DEFAULT 'a;b;c'\n"+
			");\n"+
			"CREATE TABLE u (id integer PRIMARY KEY);\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	if len(schema.Tables) != 2 {
		t.Fatalf("read %d tables, want 2: the semicolon ended the first one", len(schema.Tables))
	}
	if schema.Tables[1].Name != "u" {
		t.Errorf("the second table is %q", schema.Tables[1].Name)
	}
}

func TestAKeywordIsANameLikeAnyOther(t *testing.T) {
	// `limit` and `order` are words the database has meanings for, and a column
	// of that name is a thing a service writes. The reader quotes what it
	// generates, so reading the name as written is enough.
	schema, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE t (\"limit\" integer NOT NULL, \"order\" text, \"select\" integer);\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	names := schema.Tables[0].ColumnNames()
	if len(names) != 3 {
		t.Fatalf("read %v", names)
	}
	for _, want := range []string{"limit", "order", "select"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("the column %q was not read", want)
		}
	}
}

func TestAQuotedNameIsUnquoted(t *testing.T) {
	// A service that quotes a name quotes it to keep it, and a reader that
	// leaves the quotes on makes a column whose name is `"name"`.
	schema, diagnostics := ParseSchema("schema.sql",
		`CREATE TABLE "order" ("id" integer PRIMARY KEY);`)
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	if schema.Tables[0].Name != "order" {
		t.Errorf("the table is named %q, with its quotes", schema.Tables[0].Name)
	}
	if schema.Tables[0].Columns[0].Name != "id" {
		t.Errorf("the column is named %q", schema.Tables[0].Columns[0].Name)
	}
}

func TestTheKeyMayBeWrittenAsAColumnOrAsATableConstraint(t *testing.T) {
	// Both spellings are what a service writes, and they are the same key.
	inline, _ := ParseSchema("schema.sql",
		"CREATE TABLE t (id integer PRIMARY KEY, name text);\n")
	trailing, _ := ParseSchema("schema.sql",
		"CREATE TABLE t (id integer, name text, PRIMARY KEY (id));\n")
	if len(inline.Tables) != 1 || len(trailing.Tables) != 1 {
		t.Fatal("did not read")
	}
	// The inline form says the column is the key; the trailing form says the
	// table has one. Both have to come out as a key, because a generated create
	// that lost one creates a different table.
	if !inline.Tables[0].Columns[0].PrimaryKey {
		t.Error("an inline primary key was not read")
	}
	if len(trailing.Tables[0].PrimaryKey) != 1 || trailing.Tables[0].PrimaryKey[0] != "id" {
		t.Errorf("a table key read as %v", trailing.Tables[0].PrimaryKey)
	}
}

func TestACompositeKeyIsReadInItsOwnOrder(t *testing.T) {
	schema, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE orders (customer integer, placed_at integer, PRIMARY KEY (customer, placed_at));\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	key := schema.Tables[0].PrimaryKey
	if len(key) != 2 || key[0] != "customer" || key[1] != "placed_at" {
		t.Errorf("the key read as %v; the order of a composite key is the key", key)
	}
}

func TestIfNotExistsIsReadAndAnIndexIsNotATable(t *testing.T) {
	// A table declared with one has to keep it, or applying a schema twice
	// fails the second time on a service that was meant to come up.
	schema, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE IF NOT EXISTS t (id integer PRIMARY KEY);\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	if !schema.Tables[0].IfNotExists {
		t.Error("IF NOT EXISTS was not read")
	}

	// An index is a statement every engine understands and none of them spell
	// the same way, so it is reported rather than read as a table with an index
	// for a name.
	_, diagnostics = ParseSchema("schema.sql",
		"CREATE INDEX idx ON customers (name);\nCREATE TABLE t (id integer);\n")
	if !HasErrors(diagnostics) {
		t.Error("a statement that is not a table was accepted, and a schema that drops what it cannot read describes a different database")
	}
}

func TestAConstraintIsNotAColumn(t *testing.T) {
	// A constraint written inside the column list is not a column, and a reader
	// that files it as one generates a create that a database refuses.
	schema, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE orders (customer integer, placed_at integer, PRIMARY KEY (customer, placed_at));\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	for _, column := range schema.Tables[0].Columns {
		if column.Type == "" || strings.EqualFold(column.Name, "PRIMARY") {
			t.Errorf("a constraint was read as the column %+v", column)
		}
	}
}

func TestAnEmptySchemaReadsAsNothingRatherThanFailing(t *testing.T) {
	// A project with no SQL declares no SQL, which is a fact rather than a
	// failure: most modules are libraries.
	schema, diagnostics := ParseSchema("schema.sql", "")
	if HasErrors(diagnostics) {
		t.Errorf("an empty schema is an error: %+v", diagnostics)
	}
	if len(schema.Tables) != 0 {
		t.Errorf("an empty schema read %d tables", len(schema.Tables))
	}
}

func TestAStatementSpanningLinesIsOneStatement(t *testing.T) {
	// A statement is however many lines it is written as, and a reader that
	// reads line by line sees several statements where a service wrote one.
	queries, diagnostics := ParseQueries("queries.sql",
		"-- name: everything\n"+
			"SELECT id, name\n"+
			"FROM customers\n"+
			"WHERE address = :address\n"+
			"ORDER BY name;\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	if len(queries.Queries) != 1 {
		t.Fatalf("read %d statements, want 1", len(queries.Queries))
	}
	statement := queries.Queries[0]
	if statement.Table != "customers" {
		t.Errorf("the table read as %q", statement.Table)
	}
	if len(statement.Parameters) != 1 || statement.Parameters[0].Name != "address" {
		t.Errorf("the parameters read as %+v", statement.Parameters)
	}
}
