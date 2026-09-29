//
// @file entities_test.go
// @brief Tables declared as data, and what follows from them.
//
// A person should not write SQL to have typed access to their own tables, and
// these are what stands behind that: the tables are read from declarations, the
// create statements are written from them, and the operations over them are
// generated. Each of those is a place where a mistake becomes a database that is
// not the one the code expects, so each is checked here.
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

// A declaration is read as written, and the order is kept: a column's position
// decides the order of a generated struct's fields and of a result row.
func TestDeclarationsAreReadInOrder(t *testing.T) {
	tables, diagnostics := ParseEntities("schema.yml", `tables:
  - name: customers
    description: The people who buy things.
    columns:
      - name: id
        type: integer
        primary_key: true
        auto: true
      - name: name
        type: text
        not_null: true
      - name: address
        type: text
  - name: orders
    description: What was bought.
    columns:
      - name: id
        type: integer
        primary_key: true
        auto: true
      - name: customer
        type: integer
        references: customers(id)
`)
	if HasErrors(diagnostics) {
		t.Fatalf("a well-formed declaration was refused: %+v", diagnostics)
	}
	if len(tables) != 2 {
		t.Fatalf("read %d tables, want 2", len(tables))
	}
	if tables[0].Name != "customers" || tables[1].Name != "orders" {
		t.Errorf("the tables came back as %q and %q, in the order they were written",
			tables[0].Name, tables[1].Name)
	}
	names := tables[0].ColumnNames()
	if len(names) != 3 || names[0] != "id" || names[2] != "address" {
		t.Errorf("the columns came back as %v, in the order they were written", names)
	}
	if tables[0].Comment != "The people who buy things." {
		t.Errorf("a table's own description was lost: %q", tables[0].Comment)
	}
}

// A key of one column is written on that column, and the create statement says
// it once. A key written twice is a table no database will create.
func TestASingleKeyIsTheColumnsOwn(t *testing.T) {
	tables, diagnostics := ParseEntities("schema.yml", `tables:
  - name: users
    columns:
      - name: id
        type: integer
        primary_key: true
        auto: true
`)
	if HasErrors(diagnostics) {
		t.Fatalf("declaration refused: %+v", diagnostics)
	}
	table := tables[0]
	if !table.Columns[0].PrimaryKey {
		t.Error("the key column does not say it is the key")
	}
	if len(table.PrimaryKey) != 0 {
		t.Errorf("a single key was also written as a table constraint %v, so it is stated twice", table.PrimaryKey)
	}
	ddl := EntitiesDDL(tables)
	if strings.Count(ddl, "PRIMARY KEY") != 1 {
		t.Errorf("the create statement states the key %d times:\n%s",
			strings.Count(ddl, "PRIMARY KEY"), ddl)
	}
}

// A key of several columns is a table constraint, and the columns stop saying
// they are the key — for the same reason.
func TestACompositeKeyIsATableConstraint(t *testing.T) {
	tables, _ := ParseEntities("schema.yml", `tables:
  - name: user_roles
    columns:
      - name: user_id
        type: integer
        primary_key: true
      - name: role_id
        type: integer
        primary_key: true
`)
	ddl := EntitiesDDL(tables)
	if !strings.Contains(ddl, "PRIMARY KEY (user_id, role_id)") {
		t.Errorf("a composite key is not a table constraint:\n%s", ddl)
	}
	if strings.Count(ddl, "PRIMARY KEY") != 1 {
		t.Errorf("the composite key is stated %d times:\n%s", strings.Count(ddl, "PRIMARY KEY"), ddl)
	}
}

// A default that is an expression has to be parenthesised, because that is how a
// create statement tells it from a literal. `DEFAULT datetime('now')` is a
// syntax error; `DEFAULT (datetime('now'))` is the current time.
func TestAnExpressionDefaultIsParenthesised(t *testing.T) {
	tables, _ := ParseEntities("schema.yml", `tables:
  - name: sessions
    columns:
      - name: id
        type: integer
        primary_key: true
        auto: true
      - name: created_at
        type: text
        default: "datetime('now')"
      - name: name
        type: text
        default: "'public'"
`)
	ddl := EntitiesDDL(tables)
	if !strings.Contains(ddl, "DEFAULT (datetime('now'))") {
		t.Errorf("an expression default is not parenthesised:\n%s", ddl)
	}
	if !strings.Contains(ddl, "DEFAULT 'public'") {
		t.Errorf("a literal default was parenthesised, which changes it:\n%s", ddl)
	}
}

// The common operations are generated from the table, so a person writes one
// declaration rather than five statements per table.
func TestTheCommonOperationsAreGenerated(t *testing.T) {
	tables, _ := ParseEntities("schema.yml", `tables:
  - name: customers
    columns:
      - name: id
        type: integer
        primary_key: true
        auto: true
      - name: name
        type: text
        not_null: true
      - name: address
        type: text
`)
	byName := map[string]Query{}
	for _, query := range CRUDQueries(tables) {
		byName[query.Name] = query
	}
	for _, want := range []string{
		"insert_customers", "get_customers", "list_customers",
		"update_customers", "delete_customers",
	} {
		if _, found := byName[want]; !found {
			t.Errorf("no %q was generated; got %v", want, keysOf(byName))
		}
	}

	// A column the database generates is not a value the caller passes.
	insert := byName["insert_customers"]
	if strings.Contains(insert.SQL, "id") {
		t.Errorf("an insert asks for the generated key: %s", insert.SQL)
	}
	if len(insert.Parameters) != 2 {
		t.Errorf("an insert takes %d values, want 2", len(insert.Parameters))
	}
	// Each value knows the column it is for, so a generated function takes a
	// number where the column is a number.
	for _, parameter := range insert.Parameters {
		if parameter.Column == "" {
			t.Errorf("the value %q does not say which column it is for", parameter.Name)
		}
	}

	// A read returns the table's own columns, so a caller gets the row type and
	// not a shape only that statement happens to produce.
	if len(byName["list_customers"].Returning) != 3 {
		t.Errorf("a read returns %v, want every column", byName["list_customers"].Returning)
	}
	// A write changes rows rather than returning them, which the generator
	// carries through to the function it writes.
	if byName["delete_customers"].Verb != VerbDelete {
		t.Errorf("a delete is a %q", byName["delete_customers"].Verb)
	}
	if byName["list_customers"].Verb != VerbSelect {
		t.Errorf("a read-all is a %q", byName["list_customers"].Verb)
	}
}

// A table with no key has no row to name, so there is no read-one or remove-one.
// Generating a get that took the first column would be a function that silently
// returns the wrong row.
func TestATableWithNoKeyHasNoOperationsByKey(t *testing.T) {
	tables, _ := ParseEntities("schema.yml", `tables:
  - name: logs
    columns:
      - name: at
        type: text
      - name: what
        type: text
`)
	byName := map[string]Query{}
	for _, query := range CRUDQueries(tables) {
		byName[query.Name] = query
	}
	if _, found := byName["get_logs"]; found {
		t.Error("a read-one was generated for a table with no key")
	}
	if _, found := byName["delete_logs"]; found {
		t.Error("a remove-one was generated for a table with no key")
	}
	if _, found := byName["list_logs"]; !found {
		t.Error("a read-all was not generated for a table with no key")
	}
}

// A declaration is validated rather than discovered later: a table whose type
// is not one the declarations take is a fact about the file.
func TestADeclarationIsValidated(t *testing.T) {
	cases := map[string]string{
		"no tables": "other: 1\n",
		"no columns": `tables:
  - name: users
    columns: []
`,
		"no name": `tables:
  - columns:
      - name: id
        type: integer
`,
		"no type": `tables:
  - name: users
    columns:
      - name: id
`,
		"unknown type": `tables:
  - name: users
    columns:
      - name: id
        type: wobble
`,
		"unknown key": `tables:
  - name: users
    columns:
      - name: id
        type: integer
        wibble: true
`,
		"two of a name": `tables:
  - name: users
    columns:
      - name: id
        type: integer
  - name: users
    columns:
      - name: id
        type: integer
`,
		"a name a database will not take": `tables:
  - name: "not a name"
    columns:
      - name: id
        type: integer
`,
		"a generated column that is not the key": `tables:
  - name: users
    columns:
      - name: id
        type: integer
      - name: at
        type: text
        auto: true
`,
	}
	for name, text := range cases {
		if _, diagnostics := ParseEntities("schema.yml", text); !HasErrors(diagnostics) {
			t.Errorf("%s was accepted", name)
		}
	}
}

// The declarations a person writes are YAML; the file says which they are, and
// a file of SQL is still read as SQL.
func TestWhichFileIsWhichIsTheFilesName(t *testing.T) {
	if !IsEntitySchema("db/schema.yml") || !IsEntitySchema("db/schema.YAML") {
		t.Error("a declarations file was read as SQL")
	}
	if IsEntitySchema("db/schema.sql") {
		t.Error("a schema of statements was read as declarations")
	}
}

func keysOf(values map[string]Query) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}
