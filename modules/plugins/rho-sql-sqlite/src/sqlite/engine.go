//
// @file engine.go
// @brief The sqlite engine.
//
// This is what a second engine costs: a driver, four methods, and the dialect
// differences written down. Everything else — the shape, the protocol, the
// session, the values, the errors — is the component's, so an engine that gets
// them wrong is the only thing that gets them wrong.
//
// The dialect differences sqlite has, and what this engine does about them:
//
//   placeholders   sqlite takes `?`, `$1` and `:name`. A statement written in
//                  any of them is sent as written; one written in a marker it
//                  does not take is rendered into `?` here rather than failing.
//   returning      sqlite has had `RETURNING` since 3.35, so a write that asks
//                  for the rows it changed is one statement rather than two.
//   types          sqlite is dynamically typed, so a column's declared type is
//                  an affinity rather than a type. It is reported as the
//                  declaration says, because that is what a service wrote and
//                  what a generated Go type should follow.
//
// The connection is opened once and held. A statement per process would be a
// database open per statement, which is the reason the session exists.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rho/rho-sql/src/rhosql"

	// The driver is registered by its side effect under the name `sqlite`, which
	// is the name a DSN's prefix selects. It is sqlite compiled to Go rather than
	// a C library behind cgo, so an engine plugin builds anywhere a service
	// does.
	_ "modernc.org/sqlite"
)

// engine is the sqlite engine, holding its connection for as long as a caller
// keeps asking.
type Engine struct {
	mu sync.Mutex
	db *sql.DB
	// path is the database file, which is remembered so a caller can be told
	// where its rows are.
	path string
}

// open connects to the database the environment names.
//
// The name is read from the environment rather than from a request, because a
// database is a deployment decision and a caller that could change it by asking
// differently would be a caller that could reach a database it was not given.
func Open() (*Engine, error) {
	// The component says which database to work against and whether it is
	// disposable; this is the engine that decides what those mean. The path is
	// read from RHO_SQLITE_PATH as well, because that is what a deployment sets
	// and a deployment should not have to learn the component's vocabulary.
	path := os.Getenv(rhosql.DatabaseEnv)
	if path == "" {
		path = os.Getenv("RHO_SQLITE_PATH")
	}
	if path == "" {
		path = "data/app.sqlite"
	}
	if os.Getenv(rhosql.ScratchEnv) != "" {
		// A scratch database is a file to be made and thrown away, so the path
		// the component gave is a directory rather than a file.
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return nil, fmt.Errorf("cannot make a scratch directory at %s: %w", path, err)
			}
		}
		path = filepath.Join(path, "scratch.sqlite")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", path, err)
	}
	// A sqlite file is a file: a second writer to the same path is a second
	// process, and two of them writing is a database neither one intended. One
	// writer, one process, and the session that owns it.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot reach %s: %w", path, err)
	}
	return &Engine{db: db, path: path}, nil
}

// Describe says what this engine is.
func (e *Engine) Describe() rhosql.Engine {
	return rhosql.Engine{
		Name:    "sqlite",
		Version: e.sqliteVersion(),
		Driver:  "modernc.org/sqlite (pure Go)",
		// All three, because sqlite takes all three. A statement written in
		// `:name` runs as written; only a marker sqlite does not know is
		// rendered.
		Markers: []rhosql.Marker{
			rhosql.MarkerPositional,
			rhosql.MarkerNumbered,
			rhosql.MarkerNamed,
		},
		Returning:     true,
		MaxParameters: 999,
		Notes:         []string{"the database is at " + e.path},
	}
}

// sqliteVersion is the library's own version, which is what a caller wants when
// a statement behaves differently from how it used to — `RETURNING` arrived in
// 3.35, and a service on an older file would want to know that.
func (e *Engine) sqliteVersion() string {
	var version string
	if err := e.db.QueryRow("SELECT sqlite_version()").Scan(&version); err != nil {
		return "unknown"
	}
	return version
}

// close releases the connection, so a session that ends does not leave the
// database file held.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.db != nil {
		_ = e.db.Close()
		e.db = nil
	}
}

// Migrate creates the tables a schema declares.
func (e *Engine) Migrate(request rhosql.MigrateRequest) (rhosql.MigrateResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	response := rhosql.MigrateResponse{}
	for _, table := range request.Schema.Tables {
		statement := createTable(table, request.Force)
		if _, err := e.db.Exec(statement); err != nil {
			if !request.Force && tableExists(err) {
				response.Skipped = append(response.Skipped, table.Name)
				continue
			}
			return response, rhosql.EngineError{Response: rhosql.ErrorResponse{
				Kind:    rhosql.ErrorEngine,
				Message: fmt.Sprintf("cannot create %s: %v", table.Name, err),
				Details: map[string]string{"table": table.Name},
			}}
		}
		response.Applied = append(response.Applied, table.Name)
	}
	return response, nil
}

// Query runs one statement.
func (e *Engine) Query(request rhosql.QueryRequest) (rhosql.QueryResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	statement, values, warnings := render(request.Query, request.Values)
	response := rhosql.QueryResponse{SQL: statement, Warnings: warnings}

	if request.Query.Verb == rhosql.VerbSelect ||
		(request.Query.Verb != rhosql.VerbSelect && len(request.Query.Returning) > 0) {
		return e.selectRows(statement, values, response)
	}
	result, err := e.db.Exec(statement, values...)
	if err != nil {
		return response, engineError(err, request.Name)
	}
	affected, err := result.RowsAffected()
	if err == nil {
		response.Affected = int(affected)
	}
	return response, nil
}

// selectRows runs a statement that returns rows and reads them in order.
func (e *Engine) selectRows(statement string, values []any, response rhosql.QueryResponse) (rhosql.QueryResponse, error) {
	rows, err := e.db.Query(statement, values...)
	if err != nil {
		return response, engineError(err, "")
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return response, engineError(err, "")
	}
	response.Columns = columnNames(columns)

	for rows.Next() {
		holders := make([]any, len(columns))
		scan := make([]any, len(columns))
		for i := range holders {
			scan[i] = &holders[i]
		}
		if err := rows.Scan(scan...); err != nil {
			return response, engineError(err, "")
		}
		row := make([]rhosql.Value, len(columns))
		for i, held := range holders {
			row[i] = valueOf(held)
		}
		response.Rows = append(response.Rows, row)
	}
	return response, rows.Err()
}

// DescribeQuery says what a statement returns, without running it.
//
// sqlite prepares a statement to answer this, which is how a caller generates a
// typed result rather than an untyped one: the columns come from the engine's
// own reading of the statement, not from a guess about the select list.
func (e *Engine) DescribeQuery(request rhosql.DescribeQueryRequest) (rhosql.DescribeQueryResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// A describe renders a statement into the engine's own spelling and reads
	// the shape of what it returns. The rendering is the part a caller cannot
	// do for itself — it is what turns `:name` into the `?` a driver takes — so
	// it is answered even when the statement cannot be prepared, because a
	// statement that will not prepare is a statement whose shape is unknown but
	// whose text is known.
	statement, _, warnings := render(request.Query, nil)
	response := rhosql.DescribeQueryResponse{SQL: statement, Warnings: warnings}

	// A statement is prepared with a nil placeholder per parameter: sqlite
	// learns a statement's shape by preparing it, and a null binds to anything.
	// A write with a NOT NULL column will not prepare against a null, so its
	// shape is not read here — a write names the columns it returns anyway, and
	// the schema already says what they are.
	if request.Query.Verb != rhosql.VerbSelect {
		response.Warnings = append(response.Warnings,
			"the columns this returns come from the RETURNING clause rather than from a "+
				"prepare, which a write that has to satisfy its constraints cannot be asked about")
		return response, nil
	}
	placeholders := make([]any, len(request.Query.Parameters))
	rows, err := e.db.Query(statement, placeholders...)
	if err != nil {
		// A statement that cannot be prepared cannot be described, and saying so
		// is better than a guess: a generated struct that is wrong is compiled
		// into a service that fails at run time.
		response.Warnings = append(warnings,
			"the statement could not be prepared: "+err.Error())
		return response, nil
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return rhosql.DescribeQueryResponse{}, engineError(err, "")
	}
	declared, err := rows.ColumnTypes()
	if err != nil {
		return rhosql.DescribeQueryResponse{}, engineError(err, "")
	}
	for i, name := range columns {
		column := rhosql.ResultColumn{Name: name}
		if i < len(declared) {
			column.Type = declared[i].DatabaseTypeName()
			// A driver that cannot say says it does not know, and that is not
			// the same as the column refusing a null.
			nullable, known := declared[i].Nullable()
			column.Nullable = known && nullable
		}
		response.Columns = append(response.Columns, column)
	}
	return response, nil
}

/* -------------------------------------------------------------------------- */
/*  The dialect                                                                 */
/* -------------------------------------------------------------------------- */

// render is a statement in sqlite's spelling, with the values in the order the
// rendered text takes them.
//
// A statement written in a marker sqlite takes is sent as written, and one
// written in a marker it does not take is rendered into `?`, which it does. That
// is the whole job of the dialect here, and it is why a service written against
// one engine runs against another without being rewritten. Every rewrite is
// reported, because a silent rewrite is a rewrite nobody can find when a value
// lands in the wrong place.
func render(query rhosql.Query, values []rhosql.Value) (string, []any, []string) {
	warnings := []string{}
	byOrdinal := map[int]rhosql.Value{}
	for _, value := range values {
		byOrdinal[value.Ordinal] = value
	}

	positional := 0
	out := &strings.Builder{}
	runes := []rune(query.SQL)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\'', '"', '`':
			// A marker inside a string is text, and copying the string as it was
			// written is what keeps it that way.
			end := copyLiteral(out, runes, i)
			i = end
			continue
		case '?':
			out.WriteRune('?')
			positional++
			continue
		case '$':
			if i+1 < len(runes) && isDigit(runes[i+1]) {
				// `$1` is the first parameter, so it becomes the first `?`.
				i++
				for i+1 < len(runes) && isDigit(runes[i+1]) {
					i++
				}
				out.WriteRune('?')
				warnings = append(warnings,
					"a numbered parameter was written positionally, which is how sqlite takes it")
				positional++
				continue
			}
		case ':', '@':
			// A named parameter takes its value by name, which the driver's own
			// arguments do not, so it is rendered positionally and said.
			//
			// `::` is a cast and `:=` is an assignment; neither names a value.
			// They are consumed as a pair, because reading the second colon of a
			// cast on its own turns `id::text` into `id:?` — a statement the
			// database refuses and a caller cannot explain.
			if i+1 < len(runes) && (runes[i+1] == ':' || runes[i+1] == '=') {
				out.WriteRune(runes[i])
				out.WriteRune(runes[i+1])
				i++
				continue
			}
			if i+1 < len(runes) && !isNameByte(runes[i+1]) {
				out.WriteRune(runes[i])
				continue
			}
			name := &strings.Builder{}
			i++
			for i < len(runes) && isNameByte(runes[i]) {
				name.WriteRune(runes[i])
				i++
			}
			i--
			out.WriteRune('?')
			warnings = append(warnings,
				"the named parameter "+name.String()+" was written positionally")
			positional++
			continue
		}
		out.WriteRune(runes[i])
	}
	return out.String(), orderedValues(byOrdinal, positional), warnings
}

// orderedValues are the values in the order the rendered text takes them.
func orderedValues(byOrdinal map[int]rhosql.Value, count int) []any {
	out := make([]any, 0, count)
	for i := 1; i <= count; i++ {
		if value, found := byOrdinal[i]; found {
			out = append(out, driverValue(value))
			continue
		}
		out = append(out, nil)
	}
	return out
}

// driverValue is a value in the form the driver accepts.
//
// A typed value rather than a bare one, because a driver hands the value to the
// database and an int64 that arrived as a float has already lost a digit the
// database was asked to store.
func driverValue(value rhosql.Value) any {
	if value.Null {
		return nil
	}
	switch value.Kind {
	case rhosql.ValueInteger:
		var number int64
		if _, err := fmt.Sscanf(value.Text, "%d", &number); err == nil {
			return number
		}
		return value.Text
	case rhosql.ValueFloat:
		var number float64
		if _, err := fmt.Sscanf(value.Text, "%g", &number); err == nil {
			return number
		}
		return value.Text
	case rhosql.ValueBoolean:
		return value.Text == "true" || value.Text == "1"
	case rhosql.ValueBlob:
		return value.Blob
	default:
		return value.Text
	}
}

// copyLiteral copies a quoted string as it was written, and answers where it
// ended.
func copyLiteral(out *strings.Builder, runes []rune, open int) int {
	quote := runes[open]
	out.WriteRune(quote)
	for i := open + 1; i < len(runes); i++ {
		out.WriteRune(runes[i])
		if runes[i] == quote {
			return i
		}
	}
	return len(runes)
}

/* -------------------------------------------------------------------------- */
/*  Reading what came back                                                      */
/* -------------------------------------------------------------------------- */

// columnNames are the columns of a result, in order.
func columnNames(columns []string) []rhosql.ResultColumn {
	out := make([]rhosql.ResultColumn, 0, len(columns))
	for _, name := range columns {
		out = append(out, rhosql.ResultColumn{Name: name, Nullable: true})
	}
	return out
}

// valueOf is what a driver returned, as a value of the shape the boundary uses.
//
// The database's own bytes for a blob and its own text for anything else: a
// value that went through a float on the way out is a value that has already
// lost precision, and the boundary's job is to hand back what was stored.
func valueOf(held any) rhosql.Value {
	switch value := held.(type) {
	case nil:
		return rhosql.Value{Kind: rhosql.ValueNull, Null: true}
	case int64:
		return rhosql.Value{Kind: rhosql.ValueInteger, Text: fmt.Sprint(value)}
	case float64:
		return rhosql.Value{Kind: rhosql.ValueFloat,
			Text: strings.TrimRight(strings.TrimRight(fmt.Sprint(value), "0"), ".")}
	case bool:
		return rhosql.Value{Kind: rhosql.ValueBoolean, Text: fmt.Sprint(value)}
	case []byte:
		if text, ok := asText(value); ok {
			return rhosql.Value{Kind: rhosql.ValueText, Text: text}
		}
		return rhosql.Value{Kind: rhosql.ValueBlob, Blob: value}
	case string:
		return rhosql.Value{Kind: rhosql.ValueText, Text: value}
	case time.Time:
		return rhosql.Value{Kind: rhosql.ValueText, Text: value.Format(time.RFC3339)}
	default:
		return rhosql.Value{Kind: rhosql.ValueText, Text: fmt.Sprint(value)}
	}
}

// asText reports whether bytes are text, which is what separates a text column
// from a blob: a blob whose bytes happen to be printable is still a blob, but
// nothing can be done about telling them apart except look.
func asText(bytes []byte) (string, bool) {
	for _, b := range bytes {
		if b == 0 {
			return "", false
		}
	}
	return string(bytes), true
}

/* -------------------------------------------------------------------------- */
/*  What an engine refuses                                                       */
/* -------------------------------------------------------------------------- */

// engineError is a failure in the engine, said as the kind a caller can act on.
func engineError(err error, query string) error {
	message := err.Error()
	kind := rhosql.ErrorEngine
	switch {
	case strings.Contains(message, "no such table"),
		strings.Contains(message, "no such column"):
		kind = rhosql.ErrorNotFound
	case strings.Contains(message, "constraint failed"),
		strings.Contains(message, "datatype mismatch"):
		// A constraint the service declared being broken is a fact about the
		// values, not a fault in the engine, and a caller that retries will fail
		// the same way.
		kind = rhosql.ErrorInvalid
	}
	return rhosql.EngineError{Response: rhosql.ErrorResponse{
		Kind: kind, Message: message, Details: detail("query", query),
	}}
}

// detail is a map with one entry, or none when there is nothing to say.
func detail(key, value string) map[string]string {
	if value == "" {
		return nil
	}
	return map[string]string{key: value}
}

// tableExists reports whether a failure is a table that was already there, which
// is what applying a schema twice looks like.
func tableExists(err error) bool {
	return strings.Contains(err.Error(), "already exists")
}

/* -------------------------------------------------------------------------- */
/*  Creating what a schema declares                                             */
/* -------------------------------------------------------------------------- */

// createTable is a table as the service declared it, in sqlite's spelling.
//
// The declaration is written as the service wrote it, and only the parts sqlite
// spells differently are changed: a quoted identifier is kept, a `numeric(10,2)`
// is a type sqlite has no affinity for and keeps, and a default is written as it
// was given. An engine that rewrote more than this would be an engine deciding
// what a service meant.
func createTable(table rhosql.Table, force bool) string {
	out := &strings.Builder{}
	out.WriteString("CREATE TABLE ")
	if table.IfNotExists && !force {
		out.WriteString("IF NOT EXISTS ")
	}
	out.WriteString(quoteIdentifier(table.Name))
	out.WriteString(" (")
	for i, column := range table.Columns {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(quoteIdentifier(column.Name))
		if column.Type != "" {
			out.WriteString(" " + column.Type)
		}
		if column.PrimaryKey {
			out.WriteString(" PRIMARY KEY")
		} else if column.NotNull {
			out.WriteString(" NOT NULL")
		}
		if column.Unique {
			out.WriteString(" UNIQUE")
		}
		if column.Default != "" {
			out.WriteString(" DEFAULT " + column.Default)
		}
	}
	if len(table.PrimaryKey) > 0 {
		// A composite key is a table constraint in every dialect, including
		// this one, and it is written as one so the declaration reads the way a
		// service wrote it.
		keys := make([]string, 0, len(table.PrimaryKey))
		for _, key := range table.PrimaryKey {
			keys = append(keys, quoteIdentifier(key))
		}
		out.WriteString(", PRIMARY KEY (" + strings.Join(keys, ", ") + ")")
	}
	out.WriteString(")")
	return out.String()
}

// quoteIdentifier is a name the database will accept whatever the service wrote
// it as, so a mixed-case or reserved word survives.
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isNameByte(r rune) bool {
	return r == '_' || isDigit(r) ||
		('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}
