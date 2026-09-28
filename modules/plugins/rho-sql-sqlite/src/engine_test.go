//
// @file engine_test.go
// @brief Tests for the sqlite engine, against a real database.
//
// The point of an engine plugin is that the boundary works, so these are not
// tests of a mock: a file is created, a schema is applied to it, statements are
// run against it, and what comes back is checked. A test that only checked the
// shapes would pass on an engine that answered plausible nonsense.
//
// The database is a file in a temporary directory rather than one in memory,
// because a service's database is a file and the driver is asked for a path, not
// a mode.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"path/filepath"
	"testing"

	"github.com/rho/rho-sql/src/rhosql"
)

// openTest connects to a database in a temporary directory, so each test has one
// that nothing else can see.
func openTest(t *testing.T) *engine {
	t.Helper()
	t.Setenv("RHO_SQLITE_PATH", filepath.Join(t.TempDir(), "test.sqlite"))
	e, err := open()
	if err != nil {
		t.Fatalf("the engine did not open: %v", err)
	}
	t.Cleanup(e.close)
	return e
}

// The queries the tests are written against, as a service writes them.
const testQueries = `-- name: add_customer
-- Adds a customer and answers with the one that was added.
INSERT INTO customers (address, name) VALUES (:address, :name) RETURNING id;
`

func parse(t *testing.T, schemaText, queriesText string) rhosql.Schema {
	t.Helper()
	schema, diagnostics := rhosql.ParseSchema("schema.sql", schemaText)
	if rhosql.HasErrors(diagnostics) {
		t.Fatalf("the schema did not read: %+v", diagnostics)
	}
	queries, queryDiagnostics := rhosql.ParseQueries("queries.sql", queriesText)
	if rhosql.HasErrors(queryDiagnostics) {
		t.Fatalf("the queries did not read: %+v", queryDiagnostics)
	}
	schema.Queries = queries.Queries
	return schema
}

// the schema a test applies, and the query it runs.
func testSchemaAndQuery(t *testing.T) rhosql.Schema {
	t.Helper()
	return parse(t,
		"-- Customers of the catalogue.\n"+
			"CREATE TABLE customers (\n"+
			"    id        integer PRIMARY KEY,\n"+
			"    address   text NOT NULL,\n"+
			"    name      text NOT NULL,\n"+
			"    limit     integer NOT NULL DEFAULT 20,\n"+
			"    balance   numeric(10, 2) NOT NULL DEFAULT 0\n"+
			");\n",
		testQueries)
}

func TestTheEngineSaysWhatItIs(t *testing.T) {
	engine := openTest(t).Describe()
	if engine.Name != "sqlite" {
		t.Errorf("the engine is named %q", engine.Name)
	}
	if !engine.Returning {
		t.Error("the engine does not report RETURNING, which it has had since 3.35")
	}
	// A caller checks this before it sends a statement written in a marker, so
	// every one sqlite takes has to be here.
	for _, marker := range []rhosql.Marker{
		rhosql.MarkerPositional, rhosql.MarkerNumbered, rhosql.MarkerNamed,
	} {
		if !engine.Speaks(marker) {
			t.Errorf("the engine does not report that it takes %q", marker)
		}
	}
	if engine.Version == "" || engine.Version == "unknown" {
		t.Errorf("the engine's version read as %q", engine.Version)
	}
}

func TestAMigratedSchemaExistsInTheDatabase(t *testing.T) {
	e := openTest(t)
	schema := testSchemaAndQuery(t)

	response, err := e.Migrate(rhosql.MigrateRequest{Schema: schema})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(response.Applied) != 1 || response.Applied[0] != "customers" {
		t.Errorf("the migration applied %v", response.Applied)
	}

	// The table has to be there in the database, not only in the response: a
	// migration that reported success and created nothing would be the worst
	// kind of wrong.
	var name string
	if err := e.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='customers'",
	).Scan(&name); err != nil {
		t.Fatalf("the table is not in the database: %v", err)
	}
}

func TestASchemaAppliedTwiceIsSkippedRatherThanFailed(t *testing.T) {
	e := openTest(t)
	schema := testSchemaAndQuery(t)
	if _, err := e.Migrate(rhosql.MigrateRequest{Schema: schema}); err != nil {
		t.Fatalf("the first migration: %v", err)
	}
	// A service that starts against a database it has already created must come
	// up, not refuse to.
	response, err := e.Migrate(rhosql.MigrateRequest{Schema: schema})
	if err != nil {
		t.Fatalf("the second migration should skip, not fail: %v", err)
	}
	if len(response.Skipped) != 1 {
		t.Errorf("the second migration reported %v, want the table skipped", response)
	}
}

func TestADefaultIsTheDatabaseRatherThanTheReading(t *testing.T) {
	e := openTest(t)
	schema := testSchemaAndQuery(t)
	if _, err := e.Migrate(rhosql.MigrateRequest{Schema: schema}); err != nil {
		t.Fatal(err)
	}
	// `limit` was declared with a default of 20 and nothing set it, so the
	// database has to have 20 in it. A migration that dropped the default would
	// leave a column that is null where the service wrote 20.
	if _, err := e.db.Exec("INSERT INTO customers (address, name) VALUES ('a', 'n')"); err != nil {
		t.Fatal(err)
	}
	// `limit` is a keyword in this dialect, so a service that wants a column
	// of that name writes it quoted. The value is the point, not the name.
	var limit int
	if err := e.db.QueryRow(`SELECT "limit" FROM customers`).Scan(&limit); err != nil {
		t.Fatal(err)
	}
	if limit != 20 {
		t.Errorf("the default read as %d, want the 20 the schema declared", limit)
	}
}

func TestANamedParameterReachesTheRightColumn(t *testing.T) {
	e := openTest(t)
	schema := testSchemaAndQuery(t)
	if _, err := e.Migrate(rhosql.MigrateRequest{Schema: schema}); err != nil {
		t.Fatal(err)
	}
	query, _ := schema.Query("add_customer")
	response, err := e.Query(rhosql.QueryRequest{
		Name:  query.Name,
		Query: query,
		Values: []rhosql.Value{
			{Ordinal: 1, Kind: rhosql.ValueText, Text: "1 Bridge Street"},
			{Ordinal: 2, Kind: rhosql.ValueText, Text: "Ada"},
		},
	})
	if err != nil {
		t.Fatalf("the insert: %v", err)
	}
	// A write that returns its rows is one statement in this dialect, and the
	// row it returns is the one the caller needs to remember the id.
	if len(response.Rows) != 1 {
		t.Fatalf("the insert returned %d rows, want the one it changed", len(response.Rows))
	}

	var address, name string
	if err := e.db.QueryRow("SELECT address, name FROM customers WHERE id = ?", 1).Scan(&address, &name); err != nil {
		t.Fatal(err)
	}
	// Two values in one statement is where a positional render goes wrong: a
	// swapped pair is a row in the wrong column, which is a wrong answer rather
	// than a failure.
	if address != "1 Bridge Street" || name != "Ada" {
		t.Errorf("the row stored as address=%q name=%q", address, name)
	}
}

func TestALargeIntegerSurvivesTheRoundTrip(t *testing.T) {
	// The boundary's job is to hand back what was stored. A value that crossed
	// it as a float has already lost a digit, and the loss is invisible.
	e := openTest(t)
	schema := parse(t,
		"CREATE TABLE amounts (id integer PRIMARY KEY, value integer NOT NULL);\n",
		"-- name: add_amount\nINSERT INTO amounts (value) VALUES (:value) RETURNING id;\n")
	if _, err := e.Migrate(rhosql.MigrateRequest{Schema: schema}); err != nil {
		t.Fatal(err)
	}
	query, _ := schema.Query("add_amount")
	const large = "9007199254740993" // 2^53 + 1, which a float64 cannot hold.
	if _, err := e.Query(rhosql.QueryRequest{
		Name:   query.Name,
		Query:  query,
		Values: []rhosql.Value{{Ordinal: 1, Kind: rhosql.ValueInteger, Text: large}},
	}); err != nil {
		t.Fatalf("the insert: %v", err)
	}
	var stored string
	if err := e.db.QueryRow("SELECT value FROM amounts").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != large {
		t.Errorf("the value stored as %s, want %s", stored, large)
	}
}

func TestASelectReturnsRowsInOrderWithTheirColumns(t *testing.T) {
	e := openTest(t)
	schema := parse(t,
		"CREATE TABLE customers (id integer PRIMARY KEY, name text NOT NULL);\n",
		"-- name: all_customers\n-- Every customer, by name.\n"+
			"SELECT id, name FROM customers ORDER BY name;\n")
	if _, err := e.Migrate(rhosql.MigrateRequest{Schema: schema}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Zoe", "Ada", "Mary"} {
		if _, err := e.db.Exec("INSERT INTO customers (name) VALUES (?)", name); err != nil {
			t.Fatal(err)
		}
	}
	query, _ := schema.Query("all_customers")
	response, err := e.Query(rhosql.QueryRequest{Name: query.Name, Query: query})
	if err != nil {
		t.Fatalf("the select: %v", err)
	}
	if len(response.Columns) != 2 {
		t.Fatalf("the result has %d columns, want 2: %+v", len(response.Columns), response.Columns)
	}
	if response.Columns[0].Name != "id" || response.Columns[1].Name != "name" {
		t.Errorf("the columns read as %q and %q; the order is the order they were written in",
			response.Columns[0].Name, response.Columns[1].Name)
	}
	// The order of the rows is the order the database returned, which is the
	// order the statement asked for.
	want := []string{"Ada", "Mary", "Zoe"}
	if len(response.Rows) != len(want) {
		t.Fatalf("the select returned %d rows, want %d", len(response.Rows), len(want))
	}
	for i, name := range want {
		got := response.Rows[i][1].Text
		if got != name {
			t.Errorf("row %d is %q, want %q; the order is the order the statement asked for",
				i, got, name)
		}
	}
}

func TestWhatAStatementReturnsCanBeAskedForWithoutRunningIt(t *testing.T) {
	// This is what makes a generated result possible: the columns come from the
	// engine reading the statement, not from a guess about the select list.
	e := openTest(t)
	schema := parse(t,
		"CREATE TABLE customers (id integer PRIMARY KEY, name text NOT NULL);\n",
		"-- name: all_customers\nSELECT id, name FROM customers;\n")
	if _, err := e.Migrate(rhosql.MigrateRequest{Schema: schema}); err != nil {
		t.Fatal(err)
	}
	query, _ := schema.Query("all_customers")
	response, err := e.DescribeQuery(rhosql.DescribeQueryRequest{
		Name: query.Name, Query: query, Schema: &schema,
	})
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if len(response.Columns) != 2 {
		t.Fatalf("the statement was described as returning %d columns, want 2: %+v",
			len(response.Columns), response.Columns)
	}
	if response.Columns[1].Name != "name" {
		t.Errorf("the second column is %q", response.Columns[1].Name)
	}
	// A type is what a generated Go type follows, so it is worth having: a column
	// with no type is a column whose Go type has to be a guess.
	if response.Columns[0].Type == "" {
		t.Errorf("the first column has no type: %+v", response.Columns[0])
	}
}

func TestAStatementTheDatabaseDoesNotHaveIsRefusedByName(t *testing.T) {
	e := openTest(t)
	schema := parse(t,
		"CREATE TABLE customers (id integer PRIMARY KEY);\n",
		"-- name: ghost\nSELECT * FROM nowhere;\n")
	if _, err := e.Migrate(rhosql.MigrateRequest{Schema: schema}); err != nil {
		t.Fatal(err)
	}
	query, _ := schema.Query("ghost")
	_, err := e.Query(rhosql.QueryRequest{Name: query.Name, Query: query})
	if err == nil {
		t.Fatal("a statement against a table that is not there was run")
	}
	// The kind is what a caller acts on: a missing table is not a fault in the
	// engine, and a caller that retries will fail the same way.
	failure, ok := err.(rhosql.EngineError)
	if !ok {
		t.Fatalf("the failure is a %T, so a caller cannot tell what went wrong: %v", err, err)
	}
	if failure.Response.Kind != rhosql.ErrorNotFound {
		t.Errorf("the failure is a %q, want not_found", failure.Response.Kind)
	}
}

func TestABlobSurvivesAsBytes(t *testing.T) {
	e := openTest(t)
	schema := parse(t,
		"CREATE TABLE files (id integer PRIMARY KEY, body blob NOT NULL);\n",
		"-- name: add_file\nINSERT INTO files (body) VALUES (:body) RETURNING id;\n"+
			"-- name: get_file\nSELECT body FROM files WHERE id = :id;\n")
	if _, err := e.Migrate(rhosql.MigrateRequest{Schema: schema}); err != nil {
		t.Fatal(err)
	}
	// A blob with a zero byte in it is text by no definition, which is what
	// makes it a test rather than a round trip of a string.
	body := []byte{0x00, 0x01, 0xff, 0x00}
	add, _ := schema.Query("add_file")
	if _, err := e.Query(rhosql.QueryRequest{
		Name:   add.Name,
		Query:  add,
		Values: []rhosql.Value{{Ordinal: 1, Kind: rhosql.ValueBlob, Blob: body}},
	}); err != nil {
		t.Fatalf("the insert: %v", err)
	}
	get, _ := schema.Query("get_file")
	response, err := e.Query(rhosql.QueryRequest{
		Name:   get.Name,
		Query:  get,
		Values: []rhosql.Value{{Ordinal: 1, Kind: rhosql.ValueInteger, Text: "1"}},
	})
	if err != nil {
		t.Fatalf("the select: %v", err)
	}
	if len(response.Rows) != 1 {
		t.Fatalf("the select returned %d rows", len(response.Rows))
	}
	got := response.Rows[0][0]
	if got.Kind != rhosql.ValueBlob {
		t.Errorf("the value came back as %q, want bytes; a zero byte is what makes it a blob",
			got.Kind)
	}
	if string(got.Blob) != string(body) {
		t.Errorf("the blob came back as %v, want %v", got.Blob, body)
	}
}

func TestAMarkerSqliteDoesNotTakeIsRenderedAndReported(t *testing.T) {
	// A statement written in one engine's markers is rendered into this one's
	// and the caller is told, because a silent rewrite is a rewrite nobody can
	// find when a value lands in the wrong place.
	e := openTest(t)
	schema := parse(t,
		"CREATE TABLE customers (id integer PRIMARY KEY, name text NOT NULL);\n",
		"-- name: find\nSELECT id FROM customers WHERE name = :name;\n")
	if _, err := e.Migrate(rhosql.MigrateRequest{Schema: schema}); err != nil {
		t.Fatal(err)
	}
	query, _ := schema.Query("find")
	if _, err := e.db.Exec("INSERT INTO customers (name) VALUES ('Ada')"); err != nil {
		t.Fatal(err)
	}
	response, err := e.Query(rhosql.QueryRequest{
		Name:   query.Name,
		Query:  query,
		Values: []rhosql.Value{{Ordinal: 1, Kind: rhosql.ValueText, Text: "Ada"}},
	})
	if err != nil {
		t.Fatalf("the select: %v", err)
	}
	if len(response.Rows) != 1 {
		t.Errorf("the select returned %d rows, want the one named", len(response.Rows))
	}
	if len(response.Warnings) == 0 {
		t.Error("the rewrite was not reported; a caller cannot see what was changed")
	}
}
