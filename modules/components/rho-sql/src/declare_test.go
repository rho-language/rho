//
// @file declare_test.go
// @brief What a project declared about its database, and what follows from it.
//
// The descriptor is the whole of a project's statement about its database, and
// every one of these tests is about a project that said one thing and got
// another — a file written where it did not ask, an engine it did not name, a
// package the generator could not type. A build that read the manifest is
// indistinguishable from one that guessed right, so the manifest is read in
// each of its spellings here.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeProject makes a project with a manifest and returns its root.
func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// A project's manifest is what decides where its SQL is read from and where the
// generated Rho is written. Reading the conventions instead would generate code
// for a database the project did not describe, which is a build that works on
// the machine that generated it and nowhere else.
func TestTheManifestDecidesWhereTheGeneratedRhoIsWritten(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": `module:
  name: myapp
  type: rho
  version: 0.1.0
sql:
  driver: rho-sql-sqlite
  schema: db/schema.sql
  queries: db/queries.sql
  out: src/db
`,
	})
	declaration := LoadDeclaration(root)
	if !declaration.SQL.Declared() {
		t.Fatal("a project that declared SQL was read as having none")
	}
	paths, err := declaration.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "src", "db"); paths.Out != want {
		t.Errorf("the generated Rho goes in %q, want %q", paths.Out, want)
	}
	if want := filepath.Join(root, "db", "schema.sql"); paths.Schema != want {
		t.Errorf("the schema is read from %q, want %q", paths.Schema, want)
	}
	if want := filepath.Join(root, "db", "queries.sql"); paths.Queries != want {
		t.Errorf("the statements are read from %q, want %q", paths.Queries, want)
	}
	if paths.Package != "myapp" {
		t.Errorf("the generated file belongs to %q, want the project's own name", paths.Package)
	}
}

// With no `out`, the generated file is written beside the statements it was
// generated from, because that is the one place a reader looking for it looks.
func TestWithNoOutTheGeneratedRhoLandsBesideTheStatements(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": `module:
  name: myapp
  type: rho
  version: 0.1.0
sql:
  schema: db/schema.sql
  queries: db/queries.sql
`,
	})
	paths, err := LoadDeclaration(root).Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "db"); paths.Out != want {
		t.Errorf("with no `out` the generated Rho goes in %q, want %q", paths.Out, want)
	}
}

// A driver is a name a project chose, and the generator has to read it rather
// than pick one: two projects on two engines generate different code.
func TestTheManifestNamesTheDriver(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": "module:\n  name: myapp\n  type: rho\n  version: 0.1.0\nsql:\n  driver: rho-sql-postgres\n  schema: db/schema.sql\n  queries: db/queries.sql\n",
	})
	if got := LoadDeclaration(root).SQL.Driver; got != "rho-sql-postgres" {
		t.Errorf("the driver is %q, want the one the project named", got)
	}
}

// `requires` and `sql.driver` are different names on purpose: one is the
// runtime package the built program links and the other is the engine that
// answers a statement. A project that declares both is describing two facts,
// and reading one as the other would be a project built for an engine it does
// not have.
func TestARequirementAndADriverAreDifferentNames(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": `module:
  name: myapp
  type: rho
  version: 0.1.0
requires:
  - name: rho/sqlite
    version: ^1.0.0
sql:
  driver: rho-sql-sqlite
  schema: db/schema.sql
  queries: db/queries.sql
`,
	})
	declaration := LoadDeclaration(root)
	if !declaration.Requires_("rho/sqlite") {
		t.Error("the runtime package the project declared was not read")
	}
	if declaration.Requires_("rho-sql-sqlite") {
		t.Error("the engine plugin was read as though it were a runtime package")
	}
	if declaration.SQL.Driver != "rho-sql-sqlite" {
		t.Errorf("the driver is %q, want the engine the project named", declaration.SQL.Driver)
	}
}

// A project that has not adopted a manifest still has SQL, and refusing it
// would make the conventions unreachable to exactly the projects that have not
// adopted a manifest.
func TestAProjectWithNoManifestFallsBackToTheConventions(t *testing.T) {
	root := writeProject(t, map[string]string{
		"sql/schema.sql":  "CREATE TABLE customers (id INTEGER PRIMARY KEY, name TEXT NOT NULL);\n",
		"sql/queries.sql": "-- name: all\nSELECT id, name FROM customers;\n",
	})
	declaration := LoadDeclaration(root)
	if declaration.Found {
		t.Error("a project with no manifest was read as having one")
	}
	paths, err := declaration.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "sql", "schema.sql"); paths.Schema != want {
		t.Errorf("the schema is read from %q, want %q", paths.Schema, want)
	}
	if paths.Package != "queries" {
		t.Errorf("a project with no name gets %q, want a name a generated file can carry", paths.Package)
	}
}

// A manifest that is not a manifest is not an error: it is a file that happens
// to be there, and a project whose SQL is beside its sources still generates.
func TestAManifestThatCannotBeReadIsNotAFailure(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml":     "this is not: [a manifest\n",
		"sql/schema.sql": "CREATE TABLE customers (id INTEGER PRIMARY KEY);\n",
	})
	if _, err := LoadDeclaration(root).Resolve(root); err != nil {
		t.Errorf("an unreadable manifest stopped the generator: %v", err)
	}
}

// A generated file belongs to the project, so it is named and packaged as the
// project is and a handler imports it the way it imports anything else.
func TestTheGeneratedFileBelongsToTheProject(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": `module:
  name: billing
  type: rho
  version: 0.1.0
sql:
  schema: db/schema.sql
  queries: db/queries.sql
  package: billing.db
`,
	})
	paths, err := LoadDeclaration(root).Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if paths.Package != "billing.db" {
		t.Errorf("the generated file is packaged as %q, want the one the project named", paths.Package)
	}
}
