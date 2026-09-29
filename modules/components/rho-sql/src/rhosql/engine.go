//
// @file engine.go
// @brief What an engine is asked, and what it answers.
//
// An engine is a plugin. It is given a dialect-agnostic statement and it
// answers with rows, and it is told nothing about Rho: it never sees a handler,
// a type or a port, only this shape. That is what makes a second engine cost a
// program rather than a rewrite, and it is why the boundary is drawn at the
// statement rather than at the driver.
//
// The hooks are the four a service needs and no more:
//
//	describe   what this engine is, and what it can be asked
//	migrate    create the tables a schema declares
//	query      run one named statement with values for its parameters
//	describe_query   what columns a statement returns, for generated code
//
// The fourth is the one that makes static generation possible. The result
// columns of a statement are a fact about the database rather than about the
// text of the query, so an engine reports them and a caller that wants a typed
// struct asks the engine rather than guessing from the select list.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import (
	"sort"

	"rho/src/deps"
	"rho/src/plugins"
)

// Hooks is the set an engine answers. A plugin declares these, and the
// validator checks that what it declares is what this component offers.
const (
	// HookDescribe is asked what the engine is and what it can be asked, which
	// is how a caller learns the markers it speaks before sending a statement.
	HookDescribe = "describe"
	// HookMigrate is asked to create the tables a schema declares.
	HookMigrate = "migrate"
	// HookQuery is asked to run one named statement.
	HookQuery = "query"
	// HookDescribeQuery is asked what a statement returns, so a caller can
	// generate a typed result rather than an untyped one.
	HookDescribeQuery = "describe_query"
)

// OfferedHooks is every hook this component offers, which is what a plugin is
// measured against.
var OfferedHooks = []string{HookDescribe, HookMigrate, HookQuery, HookDescribeQuery}

// ComponentName is this component's own name, which a plugin has to say in
// order to be loaded by it. A component knows its own name; that is the one
// thing it does not have to be told.
const ComponentName = "rho-sql"

// The environment an engine is told about its database through.
//
// The component does not know what an engine calls its own settings — a path to
// a file, a connection string, a cluster — so it says what it means and lets the
// engine decide. Naming one engine's variable here would make this a component
// for that engine, which is the thing it exists not to be.
const (
	// DatabaseEnv names the database an engine should work against.
	DatabaseEnv = "RHO_SQL_DATABASE"
	// ScratchEnv says the database is disposable, which is how a caller asks an
	// engine to work somewhere it may create and drop anything. Asking for a
	// description is asking a question, and a question should not change the
	// database it was asked about.
	ScratchEnv = "RHO_SQL_SCRATCH"
)

// Engine is what an engine says about itself.
//
// It is asked before anything else, and a caller that does not ask cannot know
// whether the engine speaks the markers a statement is written in. That is why
// it is a hook rather than something inferred from a query that failed.
type Engine struct {
	// Name is the engine's name, which is what a manifest names it by.
	Name string `json:"name"`
	// Version is the engine's own version, not the plugin protocol's.
	Version string `json:"version"`
	// Driver is the Go driver the engine holds the connection with, which is a
	// fact a reader wants and nothing depends on.
	Driver string `json:"driver,omitempty"`
	// Markers are the parameter markers the engine accepts, in the spellings it
	// accepts them in. A statement written in a marker the engine does not take
	// is rendered into one it does, by the engine.
	Markers []Marker `json:"markers"`
	// Returning is whether a write can return the rows it changed, which is
	// `RETURNING` in the dialects that have it and a second statement in the
	// ones that do not. A caller that does not check would write a query that
	// works on one engine and silently returns nothing on another.
	Returning bool `json:"returning"`
	// MaxParameters is how many values one statement may take, or 0 for no
	// limit. sqlite is 999, which is a limit a generated query can reach and a
	// caller needs to be told about.
	MaxParameters int `json:"maxParameters,omitempty"`
	// Notes are things a reader should know that are not a capability: a
	// version it was tested against, a limitation worth knowing before it is
	// found.
	Notes []string `json:"notes,omitempty"`
}

// Speaks reports whether an engine takes a marker, which is what a caller asks
// before sending a statement written in it.
func (e Engine) Speaks(marker Marker) bool {
	for _, spoken := range e.Markers {
		if spoken == marker {
			return true
		}
	}
	return false
}

// DescribeRequest asks an engine what it is.
type DescribeRequest struct{}

// MigrateRequest asks an engine to create a schema.
//
// The schema is the whole declaration, not a list of commands: an engine that
// can be handed the declaration can decide the order, can create a table
// together with the indexes that depend on it, and can refuse a schema it
// cannot honour before it has created half of it.
type MigrateRequest struct {
	// Schema is what the service declared.
	Schema Schema `json:"schema"`
	// Force is whether to apply a schema to a database that already has one,
	// which is how a development loop is driven.
	Force bool `json:"force,omitempty"`
}

// MigrateResponse is what an engine did about a schema.
type MigrateResponse struct {
	// Applied are the tables that now exist, in the order they were created.
	Applied []string `json:"applied"`
	// Skipped are the tables that were already there.
	Skipped []string `json:"skipped,omitempty"`
	// Warnings are things the engine did not do and why, which are not failures:
	// a constraint one engine cannot express is a warning, and the table it
	// belongs to is still there.
	Warnings []string `json:"warnings,omitempty"`
}

// QueryRequest asks an engine to run one statement.
//
// The values are positional and match `Query.Parameters` by ordinal, because
// the order is the contract and a name is a convenience the caller may have lost
// by the time it is sending.
type QueryRequest struct {
	// Name is the statement, which is how a caller names it.
	Name string `json:"name"`
	// Query is the statement as the service wrote it.
	Query Query `json:"query"`
	// Values are the statement's parameters, in the order `Query.Parameters` gives.
	Values []Value `json:"values,omitempty"`
	// Schema is what the service declared, so an engine can resolve a table or a
	// column without being asked separately.
	Schema *Schema `json:"schema,omitempty"`
}

// Value is one value, going in or coming out.
//
// It is typed rather than a bare `any`, because a driver needs to hand the value
// to a database in a form it will accept: a JSON number crossing into a driver
// as a float loses a large integer, which is a wrong answer rather than a
// failed one.
type Value struct {
	// Ordinal is the position a value going *in* is passed at, counted from one,
	// matching `Query.Parameters`. It is zero on a value coming *out*, because a
	// row's position is its column's and the column list already says which is
	// which: a reader seeing a zero there would ask what it was counting.
	Ordinal int `json:"ordinal,omitempty"`
	// Kind is what the value is, so an engine can convert it.
	Kind ValueKind `json:"kind"`
	// Text is a string, a number or a boolean, as written.
	Text string `json:"text,omitempty"`
	// Null is whether the value is absent, which is a value of its own: SQL
	// distinguishes `NULL` from the empty string and from zero.
	Null bool `json:"null,omitempty"`
	// Blob is binary data, which is a value that has no text form that survives
	// a round trip.
	Blob []byte `json:"blob,omitempty"`
}

// ValueKind is what a value is.
type ValueKind string

// The kinds of value a statement takes. They are the SQL ones, because a
// statement's parameters are typed in SQL and mapping them to something else is
// what this package is avoiding.
const (
	ValueNull    ValueKind = "null"
	ValueInteger ValueKind = "integer"
	ValueFloat   ValueKind = "float"
	ValueText    ValueKind = "text"
	ValueBoolean ValueKind = "boolean"
	ValueBlob    ValueKind = "blob"
)

// QueryResponse is what a statement returned.
type QueryResponse struct {
	// Columns are the columns of the result, in order, which is the order a row
	// is in. A statement that returns no rows still has columns, and a caller
	// that generated a struct from it needs them.
	Columns []ResultColumn `json:"columns"`
	// Rows are what came back. A row is a list of values in the order of the
	// columns rather than an object, because the order is the contract and a
	// decoder that rebuilt it into a map would lose it.
	Rows [][]Value `json:"rows,omitempty"`
	// Affected is how many rows a write changed, which is what a write returns
	// and a select does not.
	Affected int `json:"affected,omitempty"`
	// SQL is what the engine actually ran, which is the dialect's spelling of
	// the statement. It is in the answer rather than logged because it is how a
	// caller finds out what its parameter markers were rendered into, and a
	// caller that cannot see that cannot trust the row it got.
	SQL string `json:"sql,omitempty"`
	// Warnings are things the engine did not do the way the service wrote it: a
	// `RETURNING` the engine cannot honour, a marker it rewrote. The statement
	// still ran, so these are warnings rather than failures.
	Warnings []string `json:"warnings,omitempty"`
}

// ResultColumn is one column of a result.
type ResultColumn struct {
	Name string `json:"name"`
	// Type is the column's type as the engine reports it, which is the engine's
	// own spelling rather than a SQL standard one. A caller that wants a Go type
	// asks the engine to map it rather than assuming.
	Type string `json:"type,omitempty"`
	// Nullable is whether the column may be absent from a row, which is a
	// statement about the engine's view rather than about the service's
	// declaration and may differ from it.
	Nullable bool `json:"nullable,omitempty"`
	// Comment is what the service wrote beside the column, which an engine does
	// not report and which a generated struct is documented from. A caller that
	// fills it from the schema before writing the code carries it through.
	Comment string `json:"comment,omitempty"`
}

// DescribeQueryRequest asks what a statement returns.
type DescribeQueryRequest struct {
	// Name is the statement.
	Name string `json:"name"`
	// Query is the statement as the service wrote it.
	Query Query `json:"query"`
	// Schema is what the service declared, so an engine can resolve a table.
	Schema *Schema `json:"schema,omitempty"`
}

// DescribeQueryResponse is what a statement returns, without running it.
type DescribeQueryResponse struct {
	// Columns are the result columns, in order.
	Columns []ResultColumn `json:"columns"`
	// SQL is what the engine would run, which is what a caller generating a
	// prepared statement needs: the parameters have to be in the order the
	// rendered text puts them.
	SQL string `json:"sql,omitempty"`
	// Warnings are the same as a run's: things rewritten on the way to the
	// engine's dialect.
	Warnings []string `json:"warnings,omitempty"`
}

// ErrorResponse is what a plugin reports when it cannot do what it was asked.
//
// It is a shape rather than a string because a caller acts on it: a caller that
// sent a statement with a marker the engine does not speak needs to know that,
// and a caller that sent a query with no such query needs to know that, and
// neither is served by a sentence.
type ErrorResponse struct {
	// Kind is what went wrong, which is what a caller matches on.
	Kind ErrorKind `json:"kind"`
	// Message is what a person reads.
	Message string `json:"message"`
	// Details is whatever would make the problem fixable: the marker an engine
	// speaks, the query that was not found.
	Details map[string]string `json:"details,omitempty"`
}

// ErrorKind is what can go wrong at the boundary.
type ErrorKind string

// The kinds, named so a caller can tell "you asked for something I do not do"
// from "I tried and it failed".
const (
	// ErrorUnsupported is a request the engine cannot serve at all: a hook it
	// does not answer, a statement kind it does not run.
	ErrorUnsupported ErrorKind = "unsupported"
	// ErrorInvalid is a request that is malformed: a parameter that is missing,
	// a value that is not the kind it claims to be.
	ErrorInvalid ErrorKind = "invalid"
	// ErrorNotFound is a statement or a table that is not there.
	ErrorNotFound ErrorKind = "not_found"
	// ErrorEngine is a failure in the engine itself: a database that is
	// unreachable, a statement the engine rejected.
	ErrorEngine ErrorKind = "engine"
	// ErrorMismatch is a version the two sides do not agree on, which is
	// reported rather than guessed at.
	ErrorMismatch ErrorKind = "mismatch"
)

// EngineError is an error a caller can act on, which is what a plugin's answer
// becomes when it could not do the work.
type EngineError struct {
	Response ErrorResponse
}

func (e EngineError) Error() string {
	if e.Response.Message == "" {
		return string(e.Response.Kind)
	}
	return string(e.Response.Kind) + ": " + e.Response.Message
}

// InstalledEngines are the engine plugins that are installed.
//
// The name is returned rather than the plugin, because a caller listing what it
// could use wants the names it would write in a configuration, and a name that
// came from a plugin this version cannot load is worse than one it is not offered.
func InstalledEngines() []string {
	home := deps.RhoHome()
	if home == "" {
		return nil
	}
	out := []string{}
	// The composite literal needs its parentheses: a range clause over one is a
	// parse error without them, which is a small thing to know once.
	installed := plugins.Source{RhoHome: home}.All()
	for _, plugin := range installed {
		// A plugin belongs to this component if it says so. Anything else that
		// happens to be installed is not an engine, however well named.
		if plugin.Extends != ComponentName {
			continue
		}
		out = append(out, plugin.Name)
	}
	sort.Strings(out)
	return out
}
