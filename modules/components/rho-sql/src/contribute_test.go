//
// @file contribute_test.go
// @brief A module's own tables and statements, generated into the application that depends on it.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rho/rho-sql/src/rhosql"
)

func lockOf(t *testing.T, project string, packages map[string]string) {
	t.Helper()
	type pkg struct{ Name, Type, Version, Path string }
	list := []pkg{}
	for name, path := range packages {
		list = append(list, pkg{name, "rho", "0.2.0", path})
	}
	data, _ := json.Marshal(map[string]any{"Packages": list})
	if err := os.WriteFile(filepath.Join(project, LockFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// An application that depends on a module gets the module's tables and
// statements, and an old copy of one in the application's own files is passed
// over in favour of the module's.
func TestAModulesTablesAndStatementsComeFirstAndWin(t *testing.T) {
	module := writeProject(t, map[string]string{
		"module.yml": "module:\n  name: inventory\n  type: module\n  version: 0.2.0\nsql:\n  schema: db/schema.sql\n  queries: db/queries.sql\n",
		"db/schema.sql": "CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, name TEXT NOT NULL, kind TEXT NOT NULL DEFAULT 'consumable');\n" +
			"ALTER TABLE items ADD COLUMN kind TEXT NOT NULL DEFAULT 'consumable';\n",
		"db/queries.sql": "-- name: list_items\nSELECT id, name, kind FROM items ORDER BY name;\n",
	})
	// A module that is a project of its own (it names a driver) brings nothing.
	other := writeProject(t, map[string]string{
		"module.yml":    "module:\n  name: telemetry\n  type: rho\n  version: 0.1.0\nsql:\n  driver: rho-sql-sqlite\n  schema: db/schema.sql\n",
		"db/schema.sql": "CREATE TABLE readings (id INTEGER PRIMARY KEY);\n",
	})
	app := writeProject(t, map[string]string{
		"module.yml": "module:\n  name: aquarium\n  type: rho\n  version: 0.4.1\nsql:\n  driver: rho-sql-sqlite\n  schema: db/schema.sql\n  queries: db/queries.sql\n  out: src/db\n",
		"db/schema.sql": "CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, name TEXT NOT NULL);\n" +
			"CREATE TABLE IF NOT EXISTS water_tests (id INTEGER PRIMARY KEY, value REAL);\n",
		"db/queries.sql": "-- name: list_items\nSELECT id, name FROM items;\n\n-- name: list_tests\nSELECT id, value FROM water_tests;\n",
	})
	lockOf(t, app, map[string]string{"inventory": module, "telemetry": other})

	paths, err := LoadDeclaration(app).Resolve(app)
	if err != nil {
		t.Fatal(err)
	}
	var warnings []string
	own, ownDDL, diagnostics := readOwn(t, paths)
	schema, ddl, found := mergeContributions(own, ownDDL, Contributions(paths.Root), func(m string) { warnings = append(warnings, m) })
	if len(diagnostics)+len(found) != 0 {
		t.Fatalf("diagnostics: %v %v", diagnostics, found)
	}
	names := []string{}
	for _, table := range schema.Tables {
		names = append(names, table.Name)
	}
	if strings.Join(names, ",") != "items,water_tests" {
		t.Fatalf("tables %v", names)
	}
	if columns := len(schema.Tables[0].Columns); columns != 3 {
		t.Fatalf("the items table is the application's copy: %d columns", columns)
	}
	queries := []string{}
	for _, query := range schema.Queries {
		queries = append(queries, query.Name)
	}
	if strings.Join(queries, ",") != "list_items,list_tests" {
		t.Fatalf("statements %v", queries)
	}
	if !strings.Contains(schema.Queries[0].SQL, "kind") {
		t.Fatalf("list_items is the application's copy: %q", schema.Queries[0].SQL)
	}
	if !strings.HasPrefix(ddl, "-- The inventory module's tables.") || !strings.Contains(ddl, "water_tests") || strings.Contains(ddl, "readings") {
		t.Fatalf("schema text: %q", ddl)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "inventory") || !strings.Contains(warnings[0], "table items") || !strings.Contains(warnings[0], "list_items") {
		t.Fatalf("warnings %v", warnings)
	}
}

// Two modules declaring one statement is a mistake the build has to name.
func TestTwoModulesWithOneStatementAreRefused(t *testing.T) {
	a := writeProject(t, map[string]string{
		"module.yml": "module:\n  name: a\n  type: module\n  version: 0.1.0\nsql:\n  queries: q.sql\n",
		"q.sql":      "-- name: count_rows\nSELECT id FROM things;\n",
	})
	b := writeProject(t, map[string]string{
		"module.yml": "module:\n  name: b\n  type: module\n  version: 0.1.0\nsql:\n  queries: q.sql\n",
		"q.sql":      "-- name: count_rows\nSELECT id FROM things;\n",
	})
	app := writeProject(t, map[string]string{"module.yml": "module:\n  name: app\n  type: rho\n  version: 0.1.0\nsql:\n  driver: rho-sql-sqlite\n  schema: db/schema.sql\n"})
	lockOf(t, app, map[string]string{"a": a, "b": b})
	_, _, found := mergeContributions(emptySchema(), "", Contributions(app), func(string) {})
	if len(found) != 1 || !strings.Contains(found[0].Message, "count_rows is declared by both a and b") {
		t.Fatalf("diagnostics %v", found)
	}
}

func emptySchema() rhosql.Schema {
	return rhosql.Schema{Tables: []rhosql.Table{}, Queries: []rhosql.Query{}}
}

// readOwn is the project's own files, read without its dependencies.
func readOwn(t *testing.T, paths SQLPaths) (rhosql.Schema, string, []rhosql.Diagnostic) {
	t.Helper()
	paths.Root = ""
	return readDeclared(paths)
}

// Two modules declaring one table would write each other's rows: the build names
// them both.
func TestTwoModulesWithOneTableAreRefused(t *testing.T) {
	a := writeProject(t, map[string]string{
		"module.yml": "module:\n  name: a\n  type: module\n  version: 0.1.0\nsql:\n  schema: s.sql\n",
		"s.sql":      "CREATE TABLE IF NOT EXISTS things (id INTEGER PRIMARY KEY);\n",
	})
	b := writeProject(t, map[string]string{
		"module.yml": "module:\n  name: b\n  type: module\n  version: 0.1.0\nsql:\n  schema: s.sql\n",
		"s.sql":      "CREATE TABLE IF NOT EXISTS things (id INTEGER PRIMARY KEY);\n",
	})
	app := writeProject(t, map[string]string{"module.yml": "module:\n  name: app\n  type: rho\n  version: 0.1.0\nsql:\n  driver: rho-sql-sqlite\n  schema: db/schema.sql\n"})
	lockOf(t, app, map[string]string{"a": a, "b": b})
	_, _, found := mergeContributions(emptySchema(), "", Contributions(app), func(string) {})
	refused := false
	for _, d := range found {
		if d.Level == "error" && strings.Contains(d.Message, "table things") && strings.Contains(d.Message, " a ") && strings.Contains(d.Message, " b") {
			refused = true
		}
	}
	if !refused {
		t.Fatalf("two modules with one table were not refused: %v", found)
	}
}
