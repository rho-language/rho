//
// @file declare.go
// @brief Reading what a project declared about its database.
//
// A project that has a database says so in its manifest, in one place:
//
//	sql:
//	  driver: rho-sql-sqlite
//	  schema: db/schema.sql
//	  queries: db/queries.sql
//	  out: src/db
//
// Everything the generator does follows from that block: which engine types the
// statements, which files are read, where the generated Rho is written, and what
// module it belongs to. A build that read those from anywhere else would generate
// code for a database the project did not declare, which is a build that works on
// the machine that generated it and nowhere else.
//
// A project with no `sql:` block still works. It gets the conventions this
// component has always used, because a project that has not declared a database
// is a project this command has nothing to generate from — and a component that
// refused to read anything a project had not described in full would leave a
// project that keeps its SQL beside its sources with nothing working.
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

	"rho/src/yaml"
)

// Declaration is what a project's manifest says about itself.
//
// Only the parts this component acts on are read. A manifest also carries a
// module's version, its author and its dependencies, and none of those change
// what a generated file looks like.
type Declaration struct {
	// Name is the Rho module the project is, which is what a generated file
	// declares itself as.
	Name string
	// SQL is the `sql:` block, or zero when the project declared none.
	SQL SQLDeclaration
	// Requires are the packages the project's generated code links against.
	Requires []Requirement
	// Found is whether a manifest was read at all, which is different from a
	// manifest that was read and declared nothing.
	Found bool
}

// SQLDeclaration is a project's statement that it has a database.
//
// Everything here is a location or a name. None of it is a setting: the driver
// decides what its own settings are, so a project does not have to know that a
// database is a file on one engine and a connection string on another.
type SQLDeclaration struct {
	// Driver is the engine plugin the generated code runs against.
	Driver string
	// Schema and Queries are the files the SQL is written in, relative to the
	// project root.
	Schema  string
	Queries string
	// Out is the directory the generated Rho is written to, relative to the
	// project root. It defaults to the queries file's own directory.
	Out string
	// Package is the Rho module the generated file belongs to, which defaults
	// to the project's own name.
	Package string
}

// Declared reports whether a project said it has a database.
//
// It is not the same as a driver being set: a project that named its files but
// not its engine has still said it has a database, and the generator can work
// from the schema alone. The engine is what types a result, so a project without
// one gets a result whose types were guessed, and is told so.
func (d SQLDeclaration) Declared() bool {
	return d.Driver != "" || d.Schema != "" || d.Queries != ""
}

// Requirement is a package a built project links against at run time.
type Requirement struct {
	Name    string
	Version string
}

// Requires reports whether a project declared a package by that name.
//
// A driver and a requirement are different names: `sql.driver` names the engine
// plugin that answers a statement, and `requires:` names the runtime package the
// generated code links against. A project may declare `rho/sqlite` as what it
// needs and `rho-sql-sqlite` as what it talks to, and neither is a mistake — the
// first is a library and the second is the program that holds it.
func (d Declaration) Requires_(name string) bool {
	for _, requirement := range d.Requires {
		if requirement.Name == name {
			return true
		}
	}
	return false
}

// LoadDeclaration reads a project's manifest.
//
// A project with no manifest is not a failure. A directory of SQL with no
// manifest beside it is still a project this command can read, and refusing it
// would make the conventions unreachable to exactly the projects that have not
// adopted a manifest.
func LoadDeclaration(project string) Declaration {
	out := Declaration{}
	content, err := os.ReadFile(filepath.Join(project, ManifestFile))
	if err != nil {
		return out
	}
	document, err := yaml.Load(string(content))
	if err != nil {
		return out
	}
	out.Found = true

	// The module's name is under `module:`, which is where a manifest keeps
	// everything about the module rather than about what it builds.
	if section, ok := document.Map["module"]; ok && section.Kind == yaml.KindMap {
		out.Name = section.Map["name"].Str
	}

	if section, ok := document.Map["sql"]; ok && section.Kind == yaml.KindMap {
		out.SQL = SQLDeclaration{
			Driver:  section.Map["driver"].Str,
			Schema:  section.Map["schema"].Str,
			Queries: section.Map["queries"].Str,
			Out:     section.Map["out"].Str,
			Package: section.Map["package"].Str,
		}
	}

	for _, entry := range document.Map["requires"].List {
		if entry.Kind != yaml.KindMap {
			continue
		}
		out.Requires = append(out.Requires, Requirement{
			Name:    entry.Map["name"].Str,
			Version: entry.Map["version"].Str,
		})
	}
	return out
}

// ManifestFile is the file a project's own declaration is read from.
const ManifestFile = "module.yml"

// Resolve turns a declaration into the paths this component acts on, all
// absolute against the project root.
//
// A project that declared a block gets exactly what it declared. One that did
// not gets the conventions, so the change from "SQL beside the sources" to "SQL
// declared in the manifest" is a project's own decision rather than a version
// this component imposes.
//
// A path that leaves the project is refused rather than resolved: a manifest is
// a project's own file, and one that writes generated code into `/etc` or into
// a sibling project is a mistake worth a message rather than a file.
func (d Declaration) Resolve(project string) (SQLPaths, error) {
	root, err := filepath.Abs(project)
	if err != nil {
		return SQLPaths{}, err
	}
	out := SQLPaths{}

	if d.SQL.Declared() {
		out.Schema = within(root, d.SQL.Schema)
		out.Queries = within(root, d.SQL.Queries)
		// The generated file goes where the project said. A project that keeps
		// its SQL in `db/` and its code in `src/db/` is stating where the two
		// meet, and writing anywhere else is a file nothing imports.
		directory := d.SQL.Out
		if directory == "" {
			// With no `out`, the generated file is written beside the files it
			// was generated from, which is the one place a reader looking for it
			// will look. The statements are preferred when there are any, and
			// the schema otherwise: a project that declared tables and wrote no
			// statements of its own is the common case.
			directory = filepath.Dir(d.SQL.Queries)
			if d.SQL.Queries == "" {
				directory = filepath.Dir(d.SQL.Schema)
			}
		}
		out.Out = within(root, directory)
		out.Package = d.SQL.Package
		if out.Package == "" {
			out.Package = d.Name
		}
		return out, nil
	}

	// The conventions, which are a fallback rather than a second way to say it.
	for _, sub := range SQLDirs {
		schema := filepath.Join(root, sub, SchemaFile)
		if _, err := os.Stat(schema); err != nil {
			continue
		}
		out.Schema = schema
		out.Queries = filepath.Join(root, sub, QueriesFile)
		out.Out = filepath.Join(root, "queries")
		out.Package = d.Name
		if out.Package == "" {
			out.Package = "queries"
		}
		return out, nil
	}
	return out, nil
}

// SQLPaths are the files this component reads and writes, resolved.
type SQLPaths struct {
	// Schema and Queries are the SQL a project declared.
	Schema  string
	Queries string
	// Out is the directory the generated files go in.
	Out string
	// Package is the Rho module the generated file declares itself as.
	Package string
}

// within resolves a declared path against the project root.
func within(root, relative string) string {
	if relative == "" {
		return ""
	}
	if filepath.IsAbs(relative) {
		return filepath.Clean(relative)
	}
	return filepath.Clean(filepath.Join(root, relative))
}
