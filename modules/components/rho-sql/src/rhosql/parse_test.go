//
// @file parse_test.go
// @brief Tests for reading a service's SQL.
//
// The fixtures are written the way a service writes SQL, including the things
// that break a pattern-based reader: a string containing a semicolon, a
// defaulted column, a multi-word type, a predicate, and a colon inside a
// literal. What is checked is that the reading says what the file says, because
// a shape that misreads a schema produces generated code that is wrong in a way
// nobody can see from the SQL.
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

// The schema a service would write, with the shapes that matter: a composite
// key, a defaulted column, a numeric type with a scale, and a string that holds
// a semicolon.
const schemaText = `-- Customers of the catalogue.
CREATE TABLE customers (
    id        integer PRIMARY KEY,          -- the customer's identity
    address   text NOT NULL,               -- where the order goes
    name      text NOT NULL,
    note      text,                       -- anything worth remembering
    limit     integer NOT NULL DEFAULT 20,
    balance   numeric(10, 2) NOT NULL
);

-- One row per order, keyed by the customer it belongs to.
CREATE TABLE orders (
    customer   integer NOT NULL,
    placed_at  integer NOT NULL,
    total      numeric(10, 2) NOT NULL,
    reference  text NOT NULL DEFAULT 'a;b;c',
    PRIMARY KEY (customer, placed_at)
);
`

func parseSchemaFixture(t *testing.T) Schema {
	t.Helper()
	schema, diagnostics := ParseSchema("schema.sql", schemaText)
	if HasErrors(diagnostics) {
		t.Fatalf("the fixture did not read: %+v", diagnostics)
	}
	return schema
}

func TestASchemaReadsItsTablesInOrder(t *testing.T) {
	schema := parseSchemaFixture(t)
	if len(schema.Tables) != 2 {
		t.Fatalf("read %d tables, want 2", len(schema.Tables))
	}
	if schema.Tables[0].Name != "customers" || schema.Tables[1].Name != "orders" {
		t.Errorf("the tables read as %q and %q; the order they were written in is the order they are in",
			schema.Tables[0].Name, schema.Tables[1].Name)
	}
}

func TestAColumnKeepsItsTypeAsWritten(t *testing.T) {
	// The spelling of a type is the engine's to interpret, so the shape carries
	// what the service wrote rather than a mapping this package would have to
	// keep right for every engine.
	schema := parseSchemaFixture(t)
	balance, found := schema.Tables[0].Column("balance")
	if !found {
		t.Fatal("the balance column is missing")
	}
	if balance.Type != "numeric(10, 2)" {
		t.Errorf("the balance type is %q, want the type as written, scale and all", balance.Type)
	}
}

func TestACompositeKeyIsRead(t *testing.T) {
	// A key spread over a table constraint is the case a line-based reader
	// misses, and a service that generates a struct from the key needs it.
	schema := parseSchemaFixture(t)
	orders, _ := schema.Table("orders")
	if len(orders.PrimaryKey) != 2 {
		t.Fatalf("the orders key read as %v, want the two columns it was written with", orders.PrimaryKey)
	}
	if orders.PrimaryKey[0] != "customer" || orders.PrimaryKey[1] != "placed_at" {
		t.Errorf("the orders key read as %v, in the order it was written", orders.PrimaryKey)
	}
}

func TestAColumnConstraintIsNotPartOfItsName(t *testing.T) {
	schema := parseSchemaFixture(t)
	address, _ := schema.Tables[0].Column("address")
	if address.Name != "address" {
		t.Errorf("the column read as %q; NOT NULL is a constraint, not a name", address.Name)
	}
	if !address.NotNull {
		t.Error("the column is marked NOT NULL and that was not read")
	}
}

func TestADefaultIsKeptAsWritten(t *testing.T) {
	// A default is the service's decision, and an engine that applies the schema
	// has to write exactly this rather than a value guessed from the type.
	schema := parseSchemaFixture(t)
	limit, _ := schema.Tables[0].Column("limit")
	if limit.Default != "20" {
		t.Errorf("the default read as %q, want 20", limit.Default)
	}
	reference, _ := schema.Tables[1].Column("reference")
	if reference.Default != "'a;b;c'" {
		t.Errorf("the default read as %q; a semicolon inside a string is not a statement end", reference.Default)
	}
}

func TestACommentIsTheColumnDescription(t *testing.T) {
	schema := parseSchemaFixture(t)
	id, _ := schema.Tables[0].Column("id")
	if !strings.Contains(id.Comment, "identity") {
		t.Errorf("the column comment read as %q", id.Comment)
	}
}

func TestIfNotExistsIsRead(t *testing.T) {
	schema, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE IF NOT EXISTS t (id integer PRIMARY KEY);\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	if !schema.Tables[0].IfNotExists {
		t.Error("IF NOT EXISTS was not read")
	}
}

func TestAColonInsideAStringIsNotAStatementEnd(t *testing.T) {
	// A default of 'a;b' ends a statement if the reader does not know strings
	// end at their closing quote, and a schema cut in half is a schema that
	// describes a different database.
	schema, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE t (\n  id integer PRIMARY KEY,\n  note text DEFAULT 'a;b'\n);\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	if len(schema.Tables) != 1 {
		t.Fatalf("read %d tables, want 1: %+v", len(schema.Tables), schema.Tables)
	}
	if len(schema.Tables[0].Columns) != 2 {
		t.Errorf("read %d columns, want 2", len(schema.Tables[0].Columns))
	}
}

func TestAStatementThatIsNotATableIsReported(t *testing.T) {
	// A schema that quietly dropped what it could not read would describe a
	// different database from the one the service creates.
	_, diagnostics := ParseSchema("schema.sql",
		"CREATE INDEX idx ON customers (name);\nCREATE TABLE t (id integer);\n")
	if !HasErrors(diagnostics) {
		t.Fatal("a statement that is not a table was accepted")
	}
}

func TestTheDiagnosticNamesTheLine(t *testing.T) {
	// A line is what a person can go and look at.
	_, diagnostics := ParseSchema("schema.sql",
		"CREATE TABLE good (id integer PRIMARY KEY);\nDELETE FROM customers;\n")
	if len(diagnostics) == 0 {
		t.Fatal("nothing was reported")
	}
	if diagnostics[0].Line != 2 {
		t.Errorf("the problem was reported on line %d, want 2", diagnostics[0].Line)
	}
	if diagnostics[0].File != "schema.sql" {
		t.Errorf("the problem was reported in %q, want the file it was found in", diagnostics[0].File)
	}
}

/* -------------------------------------------------------------------------- */
/*  Queries                                                                     */
/* -------------------------------------------------------------------------- */

// The queries a service would write, with the shapes that matter: three marker
// kinds, a predicate, a comment, a write, and a colon inside a literal.
const queriesText = `-- name: find_customer
-- Returns the customer at this address, or no rows.
-- The address is compared as written: a collation is an engine's decision.
SELECT id, name FROM customers WHERE address = :address;

-- name: customers_limited
-- Lists customers up to a limit, the way the index does.
SELECT id, name FROM customers LIMIT :limit;

-- name: orders_for
-- Lists the orders of one customer, newest first.
SELECT customer, total FROM orders WHERE customer = ? ORDER BY placed_at DESC;

-- name: add_customer
-- Adds a customer, returning the one that was added.
INSERT INTO customers (address, name) VALUES (:address, :name) RETURNING id;

-- name: total_for
-- Adds up what a customer has spent.
SELECT sum(total) AS spent FROM orders WHERE customer = $1;
`

func parseQueriesFixture(t *testing.T) Schema {
	t.Helper()
	schema, diagnostics := ParseQueries("queries.sql", queriesText)
	if HasErrors(diagnostics) {
		t.Fatalf("the fixture did not read: %+v", diagnostics)
	}
	return schema
}

func TestAQueryIsReadByItsName(t *testing.T) {
	schema := parseQueriesFixture(t)
	if len(schema.Queries) != 5 {
		t.Fatalf("read %d queries, want 5: %+v", len(schema.Queries), schema.Queries)
	}
	if schema.Queries[0].Name != "find_customer" {
		t.Errorf("the first query is %q", schema.Queries[0].Name)
	}
	query, found := schema.Query("find_customer")
	if !found {
		t.Fatal("a named query could not be found again")
	}
	if query.Verb != VerbSelect {
		t.Errorf("the verb is %q", query.Verb)
	}
	if query.Table != "customers" {
		t.Errorf("the table is %q", query.Table)
	}
}

func TestTheProseAboveANameIsItsDescription(t *testing.T) {
	// The comment above a `name:` is what a caller reading a generated
	// signature is looking for, and it is the reason the convention puts the name
	// last in the block.
	schema := parseQueriesFixture(t)
	query, _ := schema.Query("find_customer")
	if !strings.Contains(query.Comment, "collation") {
		t.Errorf("the description read as %q", query.Comment)
	}
}

func TestAStatementKeepsItsText(t *testing.T) {
	// The text is what an engine runs when it can, so it has to be the
	// statement as written and not a reconstruction of it.
	schema := parseQueriesFixture(t)
	query, _ := schema.Query("orders_for")
	if !strings.Contains(query.SQL, "ORDER BY placed_at DESC") {
		t.Errorf("the statement read as %q; the ordering was lost", query.SQL)
	}
}

func TestEachMarkerKindIsRead(t *testing.T) {
	// The three ways a value is marked is most of what differs between engines,
	// so which one a statement uses is part of the contract.
	schema := parseQueriesFixture(t)
	cases := map[string]Marker{
		"find_customer":     MarkerNamed,
		"customers_limited": MarkerNamed,
		"orders_for":        MarkerPositional,
		"total_for":         MarkerNumbered,
	}
	for name, want := range cases {
		query, _ := schema.Query(name)
		if len(query.Parameters) == 0 {
			t.Errorf("%s takes no parameters", name)
			continue
		}
		if got := query.Parameters[0].Marker; got != want {
			t.Errorf("%s's first parameter is a %q, want %q", name, got, want)
		}
	}
}

func TestANumberedParameterKeepsItsNumber(t *testing.T) {
	// `$1` is the value's position, not a name, and a caller passing one value
	// has to know it is the first.
	schema := parseQueriesFixture(t)
	query, _ := schema.Query("total_for")
	parameter, found := query.Parameter("$1")
	if !found {
		t.Fatalf("the parameter read as %+v", query.Parameters)
	}
	if parameter.Ordinal != 1 {
		t.Errorf("the ordinal is %d, want 1", parameter.Ordinal)
	}
}

func TestParametersAreInTheOrderTheirMarkersAppear(t *testing.T) {
	// The order a caller passes values in is the order the markers appear, so
	// two values in one statement are not interchangeable.
	schema, diagnostics := ParseQueries("queries.sql",
		"-- name: between\nSELECT id FROM t WHERE a <= :low AND b <= :high;\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	query, _ := schema.Query("between")
	if len(query.Parameters) != 2 {
		t.Fatalf("read %d parameters, want 2: %+v", len(query.Parameters), query.Parameters)
	}
	if query.Parameters[0].Name != "low" || query.Parameters[1].Name != "high" {
		t.Errorf("the parameters read as %q then %q; the order is the order they were written in",
			query.Parameters[0].Name, query.Parameters[1].Name)
	}
}

func TestAMarkerInsideAStringIsNotAParameter(t *testing.T) {
	// An address written as 'a:b' is text, and reading it as a parameter would
	// make a caller pass a value for something that is already known.
	schema, diagnostics := ParseQueries("queries.sql",
		"-- name: by_note\nSELECT id FROM t WHERE note = 'see :chapter' AND id = ?;\n")
	if HasErrors(diagnostics) {
		t.Fatalf("did not read: %+v", diagnostics)
	}
	query, _ := schema.Query("by_note")
	if len(query.Parameters) != 1 {
		t.Fatalf("read %d parameters, want 1: %+v", len(query.Parameters), query.Parameters)
	}
	if query.Parameters[0].Name != "?" {
		t.Errorf("the parameter is %q, want the positional one", query.Parameters[0].Name)
	}
}

func TestAWriteIsReadAsAWrite(t *testing.T) {
	// `INSERT INTO` is two words and the shape has one verb, so this is the case
	// a first word reader gets wrong.
	schema := parseQueriesFixture(t)
	query, _ := schema.Query("add_customer")
	if query.Verb != VerbInsert {
		t.Errorf("the verb is %q, want insert", query.Verb)
	}
	if query.Table != "customers" {
		t.Errorf("the table is %q", query.Table)
	}
}

func TestReturningIsRead(t *testing.T) {
	// A write that returns its rows is what a caller needs to know it happened,
	// and an engine that cannot do it has to be told which queries want it.
	schema := parseQueriesFixture(t)
	query, _ := schema.Query("add_customer")
	if len(query.Returning) != 1 || query.Returning[0] != "id" {
		t.Errorf("returning read as %v, want [id]", query.Returning)
	}
}

func TestAStatementWithNoNameIsReported(t *testing.T) {
	// A query nobody can call is a query nobody meant to write.
	_, diagnostics := ParseQueries("queries.sql", "SELECT id FROM customers;\n")
	if !HasErrors(diagnostics) {
		t.Fatal("a statement with no name was accepted")
	}
}

func TestAnEmptyQueryFileSaysSo(t *testing.T) {
	_, diagnostics := ParseQueries("queries.sql", "-- nothing here yet\n")
	if len(diagnostics) == 0 {
		t.Fatal("an empty file was read as having no queries, silently")
	}
	// It is a warning rather than an error: a file being written is not a file
	// that is wrong.
	if HasErrors(diagnostics) {
		t.Errorf("an empty file is an error rather than a note: %+v", diagnostics)
	}
}
