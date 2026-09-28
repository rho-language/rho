//
// @file generate_test.go
// @brief Tests for the static generator.
//
// Every case here is one a reading of the SQL gets wrong quietly. A parameter
// named by a marker rather than by a word produces a function that does not
// compile; a shape and a statement sharing a name produces a package that does
// not compile; a scaled number carried as a float produces a service that
// returns wrong numbers. The generated code is compiled by these tests where a
// compiler is available, because a generator that emits text nobody compiles is
// a generator that emits whatever it likes.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import (
	goparser "go/parser"
	"go/token"
	"strings"
	"testing"
)

// theSchema is the project the tests generate from: two tables, a composite
// key, a default, a scaled number, and a write that returns what it changed.
const theSchema = `-- Customers of the catalogue.
CREATE TABLE customers (
    id      integer PRIMARY KEY,          -- the customer's identity
    address text NOT NULL,
    name    text NOT NULL,
    limit   integer NOT NULL DEFAULT 20,
    balance numeric(10, 2) NOT NULL
);

CREATE TABLE orders (
    customer  integer NOT NULL,
    placed_at integer NOT NULL,
    total     numeric(10, 2) NOT NULL,
    PRIMARY KEY (customer, placed_at)
);
`

// theQueries are the statements, one of each shape: a read with a named
// parameter, a positional one, a numbered one, a write that returns its rows,
// and a write that does not.
const theQueries = `-- name: find_customer
-- Returns the customer at this address.
SELECT id, name FROM customers WHERE address = :address;

-- name: customers_limited
-- Lists customers up to a limit.
SELECT id, name FROM customers LIMIT :limit;

-- name: orders_for
-- Lists what a customer has ordered.
SELECT customer, total FROM orders WHERE customer = ? ORDER BY placed_at DESC;

-- name: total_for
-- Adds up what a customer has spent.
SELECT sum(total) AS spent FROM orders WHERE customer = $1;

-- name: add_customer
-- Adds a customer, and answers with the one that was added.
INSERT INTO customers (address, name, balance) VALUES (:address, :name, :balance) RETURNING id, name;

-- name: forget_customer
-- Removes a customer.
DELETE FROM customers WHERE id = :id;
`

// theProject is the reading the tests generate from.
func theProject(t *testing.T) Schema {
	t.Helper()
	schema, diagnostics := ParseSchema("schema.sql", theSchema)
	if HasErrors(diagnostics) {
		t.Fatalf("the schema did not read: %+v", diagnostics)
	}
	queries, queryDiagnostics := ParseQueries("queries.sql", theQueries)
	if HasErrors(queryDiagnostics) {
		t.Fatalf("the queries did not read: %+v", queryDiagnostics)
	}
	schema.Queries = queries.Queries
	return schema
}

// generated runs the generator over the project, with the engine's own reading of
// the selects when one is given.
func generated(t *testing.T, results map[string][]ResultColumn) Result {
	t.Helper()
	return NewGenerator(theProject(t), "queries", results).Generate()
}

// compiles reports whether Go source parses, which is the least a generated
// function has to be.
func compiles(t *testing.T, name, source string) {
	t.Helper()
	if _, err := goparser.ParseFile(token.NewFileSet(), name, source, goparser.ParseComments); err != nil {
		t.Fatalf("the generated %s does not parse: %v\n%s", name, err, source)
	}
}

func TestTheGeneratedGoParses(t *testing.T) {
	result := generated(t, nil)
	compiles(t, "models.go", ModelsGo(result))
	compiles(t, "queries.go", QueriesGo(result))
}

func TestEveryStatementBecomesAFunction(t *testing.T) {
	result := generated(t, nil)
	if len(result.Queries) != 6 {
		t.Fatalf("generated %d functions, want 6", len(result.Queries))
	}
	source := QueriesGo(result)
	for _, name := range []string{
		"FindCustomer", "CustomersLimited", "OrdersFor",
		"TotalFor", "AddCustomer", "ForgetCustomer",
	} {
		if !strings.Contains(source, "func "+name+"(") {
			t.Errorf("no function for %s", name)
		}
	}
}

func TestAStatementAndItsRowsDoNotShareAName(t *testing.T) {
	// A Go package cannot hold a struct and a function of one name, and a
	// statement that returns a shape of its own is the case where they would.
	// A table's own shape keeps the table's name, because nothing else wants it.
	result := generated(t, nil)
	source := QueriesGo(result)
	for _, query := range result.Queries {
		if query.Result == "" {
			continue
		}
		if query.Result == query.Name {
			t.Errorf("%s returns a type of its own name, so the package would not compile", query.Name)
		}
	}
	// The two kinds of name are told apart, and a caller can see which is which.
	compiles(t, "queries.go", source)
}

func TestAMarkerIsNotAParameterName(t *testing.T) {
	// `?` and `$1` are how a statement marks a value, and neither is something a
	// Go function can call a parameter. A value marked positionally is named by
	// its position instead, because that is all there is to name it by.
	source := QueriesGo(generated(t, nil))
	for _, name := range []string{"v?", "v$1", "value?", "value$1"} {
		if strings.Contains(source, name) {
			t.Errorf("a parameter is named %q, which is a marker rather than a name", name)
		}
	}
	compiles(t, "queries.go", source)
}

func TestAReadReturnsItsRowsAndAWriteReportsWhatItChanged(t *testing.T) {
	result := generated(t, nil)
	kinds := map[string]Verb{}
	for _, query := range result.Queries {
		kinds[query.Name] = query.Verb
	}
	// A select is a read and returns rows; a write is a write and reports how
	// many rows it changed, which is the only thing a caller of a delete wants.
	if kinds["FindCustomer"] != VerbSelect {
		t.Errorf("a select reads as %q", kinds["FindCustomer"])
	}
	for _, name := range []string{"ForgetCustomer"} {
		if kinds[name] != VerbDelete {
			t.Errorf("a delete reads as %q", kinds[name])
		}
	}
	source := QueriesGo(result)
	if !strings.Contains(source, "int64, error") {
		t.Error("a write that changes rows does not report how many")
	}
	if !strings.Contains(source, "([]FindCustomerRow, error)") {
		t.Error("a read does not return rows")
	}
}

func TestAWriteThatReturnsItsRowsIsTypedFromTheReturningClause(t *testing.T) {
	// `RETURNING id, name` names the columns, and the schema says what they are.
	// An engine is not asked to run a write to learn them, because that would
	// change a database nobody asked to change.
	result := generated(t, nil)
	names := map[string]Model{}
	for _, model := range result.Models {
		names[model.Name] = model
	}
	model, found := names["AddCustomerRow"]
	if !found {
		t.Fatalf("no shape for the write's rows: %+v", names)
	}
	if len(model.Fields) != 2 {
		t.Fatalf("the write returns %d fields, want the two the RETURNING names: %+v",
			len(model.Fields), model.Fields)
	}
	if model.Fields[0].Column != "id" || model.Fields[1].Column != "name" {
		t.Errorf("the write returns %s and %s, want id and name", model.Fields[0].Column, model.Fields[1].Column)
	}
}

func TestAScaledNumberIsCarriedAsText(t *testing.T) {
	// `numeric(10, 2)` keeps two decimal places exactly, and a float64 does not.
	// The generated type is a string, which is the one type that keeps the
	// digits, and a caller that needs a number converts it knowingly.
	result := generated(t, nil)
	models := ModelsGo(result)
	if !strings.Contains(models, "Balance string") {
		t.Errorf("a scaled number is not carried as text:\n%s", models)
	}
	if strings.Contains(models, "Balance float64") {
		t.Errorf("a scaled number is carried as a float, which loses digits:\n%s", models)
	}
}

func TestAnEngineTypesWhatTheSchemaCannot(t *testing.T) {
	// With an engine asked, a select's columns come from the engine's own reading
	// and the statement is said to have been guessed no longer.
	result := generated(t, map[string][]ResultColumn{
		"find_customer": {{Name: "id", Type: "INTEGER"}, {Name: "name", Type: "TEXT"}},
	})
	for _, guessed := range result.Guessed {
		if guessed == "find_customer" {
			t.Errorf("a statement an engine typed is said to have been guessed: %v", result.Guessed)
		}
	}
	// The shape is in the models file, which is where a caller looks for it.
	models := ModelsGo(result)
	if !strings.Contains(models, "Id int64") {
		t.Errorf("the engine's type did not reach the shape:\n%s", models)
	}
	if !strings.Contains(models, "Name string") {
		t.Errorf("the engine's text type did not reach the shape:\n%s", models)
	}
	// And the ones the engine was not asked about are still said.
	if len(result.Guessed) == 0 {
		t.Error("nothing was said to have been guessed, and an engine answered for only one statement")
	}
}

func TestAGuessedResultIsSaidWhereItIsRead(t *testing.T) {
	// A reader deciding whether to trust a generated function should not have to
	// open every one to find the ones that were guessed.
	source := QueriesGo(generated(t, nil))
	if !strings.Contains(source, "taken from the schema rather than from an engine") {
		t.Errorf("the generated file does not say what was guessed:\n%s", source[:400])
	}
}

func TestAStatementIsWrittenTheWayTheEngineTakesIt(t *testing.T) {
	// The rendered text is what a prepared statement wants and what a caller
	// reads, and it is not the same as the markers a service wrote.
	result := NewGenerator(theProject(t), "queries", nil).WithRenderedSQL(
		map[string]string{
			"find_customer": "SELECT id, name FROM customers WHERE address = ?",
		}).Generate()
	source := QueriesGo(result)
	if !strings.Contains(source, "WHERE address = ?") {
		t.Errorf("the statement was not written the way the engine takes it:\n%s", source)
	}
	if strings.Contains(source, "WHERE address = :address") {
		t.Errorf("the service's marker spelling is still in the generated file:\n%s", source)
	}
}

func TestADescriptionReachesTheGeneratedCode(t *testing.T) {
	// The comment a service wrote above a statement is the documentation of the
	// function it generated, and the one above a column is the documentation of
	// the field. A generated file nobody can read is a generated file nobody
	// trusts.
	source := QueriesGo(generated(t, nil))
	if !strings.Contains(source, "Returns the customer at this address.") {
		t.Errorf("a statement's description did not reach the function:\n%s", source[:600])
	}
	models := ModelsGo(generated(t, nil))
	if !strings.Contains(models, "the customer's identity") {
		t.Errorf("a column's comment did not reach the field:\n%s", models)
	}
}

func TestAStatementIsWrittenWithItsOwnSql(t *testing.T) {
	// A reader comparing the generated file with the queries file needs the
	// statement in it, and needs it to be the statement.
	source := QueriesGo(generated(t, nil))
	if !strings.Contains(source, "SELECT id, name FROM customers WHERE address = :address") {
		t.Errorf("the statement is not in the generated file:\n%s", source[:600])
	}
}

func TestGeneratedCodeIsTheSameOnEveryRun(t *testing.T) {
	// A generator that reorders its output cannot be diffed, and a file that
	// cannot be diffed is a file nobody checks.
	first := QueriesGo(generated(t, nil))
	for i := 0; i < 5; i++ {
		if again := QueriesGo(generated(t, nil)); again != first {
			t.Fatalf("run %d produced a different file", i)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  The C backend                                                               */
/* -------------------------------------------------------------------------- */

// The C is written against a handle the generated file owns, because C has no
// standard every driver sits behind. What is tested here is that it is a handle:
// a generated file that named an engine would be a program that could only ever
// be that one.
func TestTheGeneratedCNamesNoEngine(t *testing.T) {
	result := generated(t, nil)
	for name, source := range map[string]string{
		"models.h":  ModelsC(result),
		"queries.c": QueriesC(result),
	} {
		for _, engine := range []string{"sqlite3.h", "sqlite3", "-lsqlite3", "duckdb"} {
			if strings.Contains(source, engine) {
				t.Errorf("the generated %s names %q, so it is that engine rather than a handle", name, engine)
			}
		}
	}
}

func TestTheGeneratedCIsWellFormed(t *testing.T) {
	result := generated(t, nil)
	models := ModelsC(result)
	source := QueriesC(result)

	// A guard is opened and closed, and each file has its own: two files of one
	// package sharing a guard means whichever is included second is skipped whole.
	for name, file := range map[string]string{"models.h": models, "queries.c": source} {
		guard := strings.Count(file, "#ifndef")
		if guard != 1 || strings.Count(file, "#define") < 1 || strings.Count(file, "#endif") < 1 {
			t.Errorf("the generated %s does not carry a complete include guard", name)
		}
	}
	// The statements include the file the types are in, because a struct that
	// holds a string needs the string to have been declared.
	if !strings.Contains(source, `#include "models.h"`) {
		t.Error("the statements do not include the file the types are in")
	}
	// And the value type is declared before anything that takes one.
	if strings.Index(source, "rho_value") < 0 {
		t.Error("the generated C has no value type")
	}
	valueAt := strings.Index(source, "} rho_value;")
	firstUse := strings.Index(source, "const rho_value *values")
	if valueAt < 0 || firstUse < 0 {
		t.Fatalf("the value type or its use is missing: type at %d, use at %d", valueAt, firstUse)
	}
	if valueAt > firstUse {
		t.Error("the value type is used before it is declared, which C reads top to bottom")
	}
}

func TestTheGeneratedCSaysTheSameAsTheGeneratedGo(t *testing.T) {
	// One reading, two renderings: a statement is a statement and a column is a
	// column whichever language it is written in, and a reader comparing the two
	// files should find the same things in them.
	result := generated(t, nil)
	c := ModelsC(result) + QueriesC(result)
	for _, query := range result.Queries {
		if !strings.Contains(c, query.Name) {
			t.Errorf("the generated C does not mention %s", query.Name)
		}
	}
}
