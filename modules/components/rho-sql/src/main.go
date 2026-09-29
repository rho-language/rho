//
// @file main.go
// @brief rho sql: what a service declares, and what an engine says about itself.
//
//	rho sql read <project>       read schema.sql and queries.sql, and say what they declare
//	rho sql ast <project>        the same, as the JSON contract engines are given
//	rho sql queries <project>     list the statements, and what each takes and returns
//	rho sql generate <project>   write the statements as code, from the schema
//	rho sql engine <name>        what an installed engine says about itself
//	rho sql engines              the engines that are installed
//	rho sql run <project> <name>  run one statement against a database and print the rows
//
// The reading is the whole of what a service declares, and it is checked here
// rather than at run time: a statement this command cannot read is a statement
// no generated code could have been written from, and finding that out while
// reading is better than finding it out from a client that got no rows.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rho/rho-sql/src/rhosql"
	"rho/src/deps"
)

// SchemaFile is the file a service declares its tables in, and QueriesFile the
// one it declares its statements in. They are named rather than configured,
// because a file a service cannot name is a file a build cannot find.
const (
	SchemaFile  = "schema.sql"
	QueriesFile = "queries.sql"
)

// usage is what the command prints when asked.
const usage = `rho sql - the SQL a service declares, and the engine that runs it

usage:
  rho sql generate <project>     write the statements as code
  rho sql read <project>         read the SQL and say what it declares
  rho sql ast <project>          the same, as the JSON contract engines are given
  rho sql queries <project>      list the statements, and what each takes
  rho sql engines                the engines that are installed
  rho sql engine <name>          what an engine says about itself
  rho sql run <project> <name>   run one statement and print what came back

flags:
  -engine <name>  the engine to run against; RHO_SQL_ENGINE otherwise
  -out <path>     where generate writes, or the file the reading commands write to
  -plain          report in a form a script can read

generate writes what a project declared, in the form its manifest asks for:
a project with an sql section gets Rho and the contract the port reads, and a
project that is a directory of SQL gets the standalone Go and C backends.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(os.Stdout, usage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}

	command := args[0]
	rest := args[1:]
	switch command {
	case "read":
		return readCommand(rest, false)
	case "ast":
		return readCommand(rest, true)
	case "queries":
		return queriesCommand(rest)
	case "generate":
		return generateCommand(rest)
	case "engines":
		return enginesCommand(rest)
	case "engine":
		return engineCommand(rest)
	case "run":
		return runCommand(rest)
	default:
		fmt.Fprintf(os.Stderr, "rho sql: unknown command %q\n\n%s", command, usage)
		return 2
	}
}

/* -------------------------------------------------------------------------- */
/*  Reading                                                                     */
/* -------------------------------------------------------------------------- */

// readCommand reads a project's SQL and reports what it declares.
func readCommand(args []string, asJSON bool) int {
	project, out, _, status := readFlags(args)
	if status != 0 {
		return status
	}
	schema, diagnostics := readProject(project)
	if rhosql.HasErrors(diagnostics) {
		for _, item := range diagnostics {
			fmt.Fprintf(os.Stderr, "rho sql: %s:%d: %s\n", item.File, item.Line, item.Message)
		}
		return 1
	}
	for _, item := range diagnostics {
		fmt.Fprintf(os.Stderr, "rho sql: %s:%d: %s\n", item.File, item.Line, item.Message)
	}
	if asJSON {
		encoded, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
			return 1
		}
		return write(out, string(encoded)+"\n")
	}
	return write(out, describe(schema))
}

// generateCommand writes the Go a project gets from what it declared.
//
// The statements are typed by the engine that will run them, when one is
// configured: a column's type is a fact about the database, and asking the
// database is better than reading it off a declaration. Without an engine the
// schema is used, and the result says so at the top of the file, because a type
// that was guessed is a type a reader has a reason to check.
func generateCommand(args []string) int {
	project, out, engineName, status := readFlags(args)
	if status != 0 {
		return status
	}
	// What the project declared decides what is read and what is written, so a
	// manifest's `sql:` block is read before anything is generated rather than
	// being a record of a decision the generator ignores.
	declaration := LoadDeclaration(project)
	paths, err := declaration.Resolve(project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
		return 1
	}
	// The engine is the project's unless a flag or the environment names
	// another: which database a project generates against is a fact about the
	// project, and a person asking for a different one is a deliberate act
	// rather than the default.
	if declaration.SQL.Driver != "" {
		engineName = declaration.SQL.Driver
	}
	schema, ddl, diagnostics := readDeclared(paths)
	if rhosql.HasErrors(diagnostics) {
		for _, item := range diagnostics {
			fmt.Fprintf(os.Stderr, "rho sql: %s:%d: %s\n", item.File, item.Line, item.Message)
		}
		return 1
	}

	// The engine is asked what each statement returns, so a result is typed by
	// the database rather than by a reading of the select list.
	results := map[string][]rhosql.ResultColumn{}
	// rendered is each statement in the engine's own spelling, which is what a
	// prepared statement and a generated function both want.
	rendered := map[string]string{}
	if engineName != "" {
		client, cleanup, err := scratchEngine(engineName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
			return 1
		}
		defer cleanup()
		if _, err := client.Migrate(schema, false); err != nil {
			fmt.Fprintf(os.Stderr, "rho sql: the schema could not be applied: %v\n", err)
			return 1
		}
		for _, query := range schema.Queries {
			// Every statement is put to the engine, because every statement's
			// markers have to become the ones it takes before a generated
			// function can pass values to it. Only a select's *columns* come
			// from the engine: a write names its own, and asking a write to run
			// just to learn them would change a database nobody asked to change.
			answer, err := client.DescribeQuery(rhosql.DescribeQueryRequest{
				Name: query.Name, Query: query, Schema: &schema,
			})
			if answer.SQL != "" {
				rendered[query.Name] = answer.SQL
			}
			if query.Verb != rhosql.VerbSelect || err != nil || len(answer.Columns) == 0 {
				continue
			}
			results[query.Name] = answer.Columns
		}
	}

	// A statement with no engine's rendering keeps the text the service wrote,
	// which is right: the markers are the engine's business and a caller that
	// has not asked an engine has not been told which markers it takes.
	generated := rhosql.NewGenerator(schema, "queries", results).WithRenderedSQL(rendered).Generate()
	// The generated file belongs to the project, so it is named and packaged as
	// the project is, and a handler imports it the way it imports anything else.
	rhoPackage := paths.Package
	if rhoPackage == "" {
		rhoPackage = "queries"
	}
	root := paths.Out
	// `-out` still wins: a person who names a directory meant it, and the
	// manifest is the default rather than an override of an explicit request.
	if out != "" {
		root = out
	}
	if root == "" {
		root = filepath.Join(project, "queries")
	}
	// Rho first: it is what a service calls, and the two backends exist so the
	// same file builds for a device and for a service. They go to a directory
	// each, because a Go package and a C translation unit cannot share one, and
	// the Rho file sits beside neither — it is the project's own source.
	rhoOptions := rhosql.RhoOptions{Package: rhoPackage}
	// The row types go in their own file rather than beside the statements.
	//
	// A statement names the shape of the rows it returns, and the shape is
	// defined once per table or once per statement. Writing the definitions into
	// the statements file would put a struct between two functions that are
	// about the same thing, and would re-declare a shape two statements share —
	// so a file that emitted them inline would not compile the moment a project
	// wrote its second query against the same table.
	files := map[string]string{
		"models.rho":  generated.ModelsRho(rhoOptions),
		"queries.rho": generated.QueriesRho(rhoOptions),
		// The contract is what the build reads to implement the externs the
		// generated Rho declares. It is written here rather than derived again
		// from the SQL by the compiler, because the Rho that names a statement
		// and the Go that implements it have to agree — and two readings of the
		// same SQL are two chances to disagree.
		rhosql.ContractFile: contractOf(generated),
	}
	// The schema the engine is given. For a project that declared tables it is
	// written from those declarations, so the create statements and the code
	// generated from the same reading cannot come to disagree.
	if ddl != "" {
		files["schema.sql"] = ddl
	}
	// The standalone backends are for a project that is not a Rho module: a Go
	// service or a C program that wants its statements typed, and has no compiler
	// to generate the port for it. A Rho module does not need them — the port is
	// generated into the project it builds — and writing them would put a Go
	// package inside the project's own sources, where any build of a surrounding
	// Go module would find it and try to compile it.
	if !declaration.Found {
		files["go/models.go"] = rhosql.ModelsGo(generated)
		files["go/queries.go"] = rhosql.QueriesGo(generated)
		files["c/models.h"] = rhosql.ModelsC(generated)
		files["c/queries.c"] = rhosql.QueriesC(generated)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
			return 1
		}
		if err := os.WriteFile(path, []byte(files[name]), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "rho sql: wrote %s\n", path)
	}
	return 0
}

// contractOf is the machine-readable half of what was generated, for the build
// to read when it supplies the port that implements the statements.
func contractOf(generated rhosql.Result) string {
	encoded, err := generated.GoContract()
	if err != nil {
		// A contract that cannot be written leaves the statements themselves
		// intact, and the build then reports an undefined function — which says
		// more than a JSON encoding error would.
		return "{\n  \"package\": \"\",\n  \"statements\": []\n}\n"
	}
	return encoded
}

// scratchEngine opens an engine against a database of its own, so that asking it
// about a schema does not touch the database a service runs against.
//
// Generating a description needs each statement prepared, and preparing a
// statement needs the tables to exist. Creating them where the service runs would
// make `rho sql generate` a command that changes production, which is not
// something a command that writes files should ever be.
//
// The engine is told where to work through its environment, because that is how
// an engine is configured: it reads the path once, when it starts.
func scratchEngine(name string) (*rhosql.Client, func(), error) {
	directory, err := os.MkdirTemp("", "rho-sql-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { os.RemoveAll(directory) }
	// The component says what it means and the engine decides what that is: it
	// does not know whether an engine's database is a file, a connection string
	// or a cluster, and naming one engine's setting would make it a component for
	// that engine.
	client, err := rhosql.OpenAt(name,
		rhosql.DatabaseEnv+"="+filepath.Join(directory, "scratch"),
		rhosql.ScratchEnv+"=1")
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return client, func() { client.Close(); cleanup() }, nil
}

// queriesCommand lists the statements a project declares.
func queriesCommand(args []string) int {
	project, out, _, status := readFlags(args)
	if status != 0 {
		return status
	}
	schema, diagnostics := readProject(project)
	if rhosql.HasErrors(diagnostics) {
		for _, item := range diagnostics {
			fmt.Fprintf(os.Stderr, "rho sql: %s:%d: %s\n", item.File, item.Line, item.Message)
		}
		return 1
	}
	var b strings.Builder
	for _, query := range schema.Queries {
		fmt.Fprintf(&b, "%s  %s %s\n", query.Name, strings.ToUpper(string(query.Verb)), query.Table)
		if query.Comment != "" {
			for _, line := range strings.Split(query.Comment, "\n") {
				fmt.Fprintf(&b, "    %s\n", line)
			}
		}
		if len(query.Parameters) > 0 {
			names := make([]string, 0, len(query.Parameters))
			for _, parameter := range query.Parameters {
				names = append(names, fmt.Sprintf("%d:%s(%s)", parameter.Ordinal,
					parameter.Name, parameter.Marker))
			}
			fmt.Fprintf(&b, "    takes %s\n", strings.Join(names, " "))
		}
		if len(query.Returning) > 0 {
			fmt.Fprintf(&b, "    returns %s\n", strings.Join(query.Returning, ", "))
		}
	}
	return write(out, b.String())
}

// describe is what a reading says about a project in prose.
func describe(schema rhosql.Schema) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", firstLine(schema.Description))
	for _, table := range schema.Tables {
		if table.Comment != "" {
			fmt.Fprintf(&b, "%s\n", table.Comment)
		}
		fmt.Fprintf(&b, "  table %s\n", table.Name)
		for _, column := range table.Columns {
			mark := "  "
			if column.PrimaryKey {
				mark = "* "
			}
			fmt.Fprintf(&b, "    %s%-16s %s", mark, column.Name, column.Type)
			if column.NotNull {
				b.WriteString(" not null")
			}
			if column.Default != "" {
				fmt.Fprintf(&b, " default %s", column.Default)
			}
			b.WriteString("\n")
			if column.Comment != "" {
				fmt.Fprintf(&b, "        -- %s\n", column.Comment)
			}
		}
		if len(table.PrimaryKey) > 0 {
			fmt.Fprintf(&b, "    key: %s\n", strings.Join(table.PrimaryKey, ", "))
		}
	}
	if len(schema.Queries) > 0 {
		fmt.Fprintf(&b, "\n%d statements\n", len(schema.Queries))
		for _, query := range schema.Queries {
			fmt.Fprintf(&b, "  %s: %s %s, %d parameter(s)\n",
				query.Name, strings.ToUpper(string(query.Verb)), query.Table,
				len(query.Parameters))
		}
	}
	return b.String()
}

// SQLDirs are where a project's SQL is looked for, in order.
//
// A project keeps its SQL beside its sources or in a directory of its own, and
// a reading that only looked in one place would describe half of the projects it
// was pointed at. `sql/` is the convention because it keeps the tables and the
// statements together and out of the sources that use them.
var SQLDirs = []string{"", "sql", "db", "database"}

// readDeclared reads a project's tables and statements, and answers the schema
// an engine is given alongside them.
//
// A declaration file and a schema of statements are two ways to describe the same
// database, and which one a project used decides what is derived: a project that
// wrote create statements gets those statements, and a project that declared
// tables gets the create statements written from them and the common operations
// over them, so neither has to write what the other already said.
//
// A file that is either is optional in the sense that a missing one is not an
// error: a project may declare its tables and not its statements yet, and reading
// what is there is more useful than refusing. What is there is still parsed and
// still checked, so a statement that cannot be read is reported wherever it was
// found.
//
// The DDL that comes back is what an engine is handed. For declarations it is
// generated, and from the same reading the code was generated from, so the
// database a service creates and the code that runs against it cannot come to
// describe two different things.
func readDeclared(paths SQLPaths) (rhosql.Schema, string, []rhosql.Diagnostic) {
	empty := rhosql.Schema{Tables: []rhosql.Table{}, Queries: []rhosql.Query{}}
	if paths.Schema == "" {
		return empty, "", nil
	}
	declared := readIfPresent(paths.Schema)

	var schema rhosql.Schema
	var diagnostics []rhosql.Diagnostic
	ddl := declared
	if rhosql.IsEntitySchema(paths.Schema) {
		tables, tableDiagnostics := rhosql.ParseEntities(filepath.Base(paths.Schema), declared)
		diagnostics = append(diagnostics, tableDiagnostics...)
		ddl = rhosql.EntitiesDDL(tables)
		schema = rhosql.Schema{Version: rhosql.Version, Tables: tables, Queries: rhosql.CRUDQueries(tables)}
	} else {
		parsed, schemaDiagnostics := rhosql.ParseSchema(SchemaFile, declared)
		diagnostics = append(diagnostics, schemaDiagnostics...)
		schema = parsed
	}

	// A project that wrote no statements of its own declares no statements: the
	// file is optional, and reading an absent one would report that it holds
	// nothing, which is a fact about a file that is not there.
	if statements := readIfPresent(paths.Queries); statements != "" {
		queries, queryDiagnostics := rhosql.ParseQueries(filepath.Base(paths.Queries), statements)
		diagnostics = append(diagnostics, queryDiagnostics...)
		schema.Queries = append(schema.Queries, queries.Queries...)
	}
	return schema, ddl, diagnostics
}

// readProject reads a project's SQL from where it declares it.
func readProject(project string) (rhosql.Schema, []rhosql.Diagnostic) {
	paths, err := LoadDeclaration(project).Resolve(project)
	if err != nil {
		return rhosql.Schema{Tables: []rhosql.Table{}, Queries: []rhosql.Query{}}, nil
	}
	schema, _, diagnostics := readDeclared(paths)
	return schema, diagnostics
}

// readIfPresent reads a file, or nothing when it is not there.
//
// A project with no SQL declares no SQL, which is a fact rather than a failure:
// a service that talks to a database through something else, or to none at all,
// is not a project this command cannot read.
func readIfPresent(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

/* -------------------------------------------------------------------------- */
/*  Engines                                                                     */
/* -------------------------------------------------------------------------- */

// enginesCommand lists the engines that are installed.
func enginesCommand(args []string) int {
	var b strings.Builder
	home := deps.RhoHome()
	if home == "" {
		return write("", "no Rho installation was found; run 'rho install'\n")
	}
	names := rhosql.InstalledEngines()
	if len(names) == 0 {
		fmt.Fprintf(&b, "No engine is installed.\nAn engine is a plugin: publish one, as with rho-sql-sqlite.\n")
		fmt.Fprintf(&b, "(looking in %s)\n", home)
		return write("", b.String())
	}
	for _, name := range names {
		fmt.Fprintf(&b, "%s\n", name)
	}
	return write("", b.String())
}

// engineCommand says what an engine says about itself.
func engineCommand(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: rho sql engine <name>")
		return 2
	}
	client, err := rhosql.Open(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
		return 1
	}
	defer client.Close()
	engine, err := client.Describe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
		return 1
	}
	encoded, err := json.MarshalIndent(engine, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
		return 1
	}
	return write("", string(encoded)+"\n")
}

// runCommand runs one statement and prints what came back.
//
// The values are given after the statement as `name=value`, in the order the
// statement takes them. A statement whose values are not all given is refused
// rather than run with blanks: a select that matched everything because its
// filter was dropped would look like a working query and be one that answers a
// question nobody asked.
func runCommand(args []string) int {
	project, out, engineName, status := readFlags(args)
	if status != 0 {
		return status
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: rho sql run <project> <statement> [name=value ...]")
		return 2
	}
	schema, diagnostics := readProject(project)
	if rhosql.HasErrors(diagnostics) {
		for _, item := range diagnostics {
			fmt.Fprintf(os.Stderr, "rho sql: %s:%d: %s\n", item.File, item.Line, item.Message)
		}
		return 1
	}
	query, found := schema.Query(args[1])
	if !found {
		fmt.Fprintf(os.Stderr, "rho sql: no statement named %q\n", args[1])
		return 1
	}
	client, err := rhosql.Open(engineName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
		return 1
	}
	defer client.Close()
	if _, err := client.Migrate(schema, false); err != nil {
		fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
		return 1
	}
	values, status := valuesFor(query, args[2:])
	if status != 0 {
		return status
	}
	response, err := client.Query(rhosql.QueryRequest{
		Name: query.Name, Query: query, Values: values, Schema: &schema,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
		return 1
	}
	encoded, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
		return 1
	}
	return write(out, string(encoded)+"\n")
}

// valuesFor are the values a statement takes, in the order it takes them.
//
// A statement that takes values and was given none is refused, and a value that
// was given for a parameter the statement does not have is refused with it: a
// statement run with the wrong values is a statement that ran, which is the one
// outcome a caller cannot tell from success.
func valuesFor(query rhosql.Query, given []string) ([]rhosql.Value, int) {
	supplied := map[string]string{}
	positional := []string{}
	for _, item := range given {
		if name, value, found := strings.Cut(item, "="); found {
			supplied[name] = value
			continue
		}
		positional = append(positional, item)
	}
	values := make([]rhosql.Value, 0, len(query.Parameters))
	for _, parameter := range query.Parameters {
		if text, found := supplied[parameter.Name]; found {
			values = append(values, valueFor(parameter.Ordinal, text))
			continue
		}
		// A positional value fills the next parameter that was not named, which
		// is what a caller passing `Ada 1 Bridge Street` means.
		if len(positional) > 0 {
			values = append(values, valueFor(parameter.Ordinal, positional[0]))
			positional = positional[1:]
			continue
		}
		fmt.Fprintf(os.Stderr,
			"rho sql: %s takes %d value(s) and %d were given; name them as name=value\n",
			query.Name, len(query.Parameters), len(given))
		return nil, 2
	}
	if len(positional) > 0 {
		fmt.Fprintf(os.Stderr, "rho sql: %s was given more values than it takes\n", query.Name)
		return nil, 2
	}
	return values, 0
}

// valueFor is one value as the boundary carries it, guessed from what it looks
// like because a value typed on a command line is text until it is not.
func valueFor(ordinal int, text string) rhosql.Value {
	if text == "null" {
		return rhosql.Value{Ordinal: ordinal, Kind: rhosql.ValueNull, Null: true}
	}
	if _, err := strconv.ParseInt(text, 10, 64); err == nil {
		// A whole number is an integer, not a float that happens to have no
		// fraction: the boundary is where a large value is lost, so the kind is
		// decided before it is carried rather than after.
		return rhosql.Value{Ordinal: ordinal, Kind: rhosql.ValueInteger, Text: text}
	}
	if _, err := strconv.ParseFloat(text, 64); err == nil {
		return rhosql.Value{Ordinal: ordinal, Kind: rhosql.ValueFloat, Text: text}
	}
	if text == "true" || text == "false" {
		return rhosql.Value{Ordinal: ordinal, Kind: rhosql.ValueBoolean, Text: text}
	}
	return rhosql.Value{Ordinal: ordinal, Kind: rhosql.ValueText, Text: text}
}

/* -------------------------------------------------------------------------- */
/*  Plumbing                                                                    */
/* -------------------------------------------------------------------------- */

// readFlags are the flags every reading command takes. They are read by hand
// rather than with a flag set, because a command that reads a project and a
// statement has a positional argument of its own and a flag set in front of it
// is a reader nobody can predict.
func readFlags(args []string) (project, out, engine string, status int) {
	project = "."
	out = ""
	engine = os.Getenv("RHO_SQL_ENGINE")
	positional := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		value := ""
		if index := strings.Index(arg, "="); index > 0 && strings.HasPrefix(arg, "-") {
			value = arg[index+1:]
			arg = arg[:index]
		}
		next := func() string {
			if value != "" {
				return value
			}
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch arg {
		case "-out", "--out":
			out = next()
		case "-engine", "--engine":
			engine = next()
		case "-plain", "--plain":
			// Accepted and ignored: the output is already plain.
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "rho sql: %q is not a flag this command takes\n", arg)
				return "", "", "", 2
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) > 0 {
		project = positional[0]
	}
	return project, out, engine, 0
}

// write puts a report where it was asked for.
func write(out, content string) int {
	if out == "" {
		fmt.Fprint(os.Stdout, content)
		return 0
	}
	if directory := filepath.Dir(out); directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
			return 1
		}
	}
	if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "rho sql: %v\n", err)
		return 1
	}
	return 0
}

func firstLine(text string) string {
	if text == "" {
		return "SQL declared by this project"
	}
	if index := strings.Index(text, "\n"); index >= 0 {
		return text[:index]
	}
	return text
}
