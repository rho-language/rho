//
// @file dialect_test.go
// @brief Tests for the dialect: what this engine does to a statement and a
// value on the way to the database.
//
// A dialect is where a service that works against one engine quietly does not
// work against another, so every one of these is a translation: a marker the
// engine does not take, a value a driver would take at the wrong width, a
// binary value a C string would end early on, an error a caller has to be able
// to tell apart from a fact about the data.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/rho/rho-sql/src/rhosql"
)

// errorOf is a failure a database reported, without dragging in a real one for
// every case: what is under test is how a message is read, not that sqlite
// produces it.
func errorOf(message string) error { return errors.New(message) }

/* -------------------------------------------------------------------------- */
/*  The markers                                                                */
/* -------------------------------------------------------------------------- */

// rendered is a statement put through the dialect, and the values that go with
// it.
func rendered(t *testing.T, sql string, values ...rhosql.Value) (string, []any, []string) {
	t.Helper()
	return render(rhosql.Query{Name: "q", Verb: rhosql.VerbSelect, SQL: sql}, values)
}

func TestAPositionalParameterIsSentAsWritten(t *testing.T) {
	// A statement written in the marker this engine emits is sent as written. A
	// rewrite of it would be a change nobody asked for and could not explain.
	statement := "SELECT id FROM t WHERE a = ?"
	sql, _, warnings := rendered(t, statement)
	if sql != statement {
		t.Errorf("a positional statement was rewritten:\n  %q\nto\n  %q", statement, sql)
	}
	if len(warnings) != 0 {
		t.Errorf("a statement in the engine's own marker was reported as changed: %v", warnings)
	}
}

func TestANamedParameterBecomesPositional(t *testing.T) {
	// A named parameter takes its value by name, and a driver's arguments do
	// not, so it is rendered positionally. The rewrite is reported, because a
	// silent rewrite is a rewrite nobody can find when a value lands in the
	// wrong place.
	sql, values, warnings := rendered(t,
		"SELECT id FROM t WHERE a = :first AND b = :second",
		rhosql.Value{Ordinal: 1, Kind: rhosql.ValueText, Text: "one"},
		rhosql.Value{Ordinal: 2, Kind: rhosql.ValueText, Text: "two"},
	)
	if strings.Contains(sql, ":first") || strings.Contains(sql, ":second") {
		t.Errorf("a named parameter survived the rendering: %q", sql)
	}
	if !strings.HasSuffix(sql, "a = ? AND b = ?") {
		t.Errorf("the statement rendered as %q", sql)
	}
	if len(values) != 2 {
		t.Fatalf("the statement takes %d values, want 2", len(values))
	}
	if values[0] != "one" || values[1] != "two" {
		t.Errorf("the values came out as %v, in the order the markers appear", values)
	}
	if len(warnings) == 0 {
		t.Error("the rewrite was not reported")
	}
}

func TestANumberedParameterBecomesPositional(t *testing.T) {
	sql, values, warnings := rendered(t,
		"SELECT id FROM t WHERE a = $2 AND b = $1",
		rhosql.Value{Ordinal: 2, Kind: rhosql.ValueText, Text: "second"},
		rhosql.Value{Ordinal: 1, Kind: rhosql.ValueText, Text: "first"},
	)
	if strings.Contains(sql, "$") {
		t.Errorf("a numbered parameter survived the rendering: %q", sql)
	}
	// The order is what matters, and it is the order the numbers say, not the
	// order they were written in — which is exactly why a numbered parameter is
	// worth reporting.
	if len(values) != 2 || values[0] != "first" || values[1] != "second" {
		t.Errorf("the values came out as %v; $2 and $1 are positions, not names", values)
	}
	if len(warnings) == 0 {
		t.Error("the rewrite was not reported")
	}
}

func TestAMarkerInsideAStringIsText(t *testing.T) {
	// An address written as 'a:b' is text, and reading the colon as a marker
	// would make a caller pass a value for something already known.
	sql, values, _ := rendered(t, "SELECT id FROM t WHERE note = 'see :chapter' AND a = ?",
		rhosql.Value{Ordinal: 1, Kind: rhosql.ValueText, Text: "x"})
	if !strings.Contains(sql, "'see :chapter'") {
		t.Errorf("the string was not copied as written: %q", sql)
	}
	if len(values) != 1 {
		t.Errorf("the statement takes %d values, want the one real marker", len(values))
	}
}

func TestACastIsNotANamedParameter(t *testing.T) {
	// `x::text` is a cast, and a reader that took the `::` for a marker would
	// read a cast as a parameter.
	sql, values, _ := rendered(t, "SELECT id::text FROM t WHERE a = ?",
		rhosql.Value{Ordinal: 1, Kind: rhosql.ValueText, Text: "x"})
	if !strings.Contains(sql, "id::text") {
		t.Errorf("a cast was read as a marker: %q", sql)
	}
	if len(values) != 1 {
		t.Errorf("the statement takes %d values, want the one real marker", len(values))
	}
}

/* -------------------------------------------------------------------------- */
/*  The values                                                                 */
/* -------------------------------------------------------------------------- */

func TestAValueIsHandedToTheDriverInItsOwnType(t *testing.T) {
	// A value that crossed into a driver as a float has already lost a digit
	// the database was asked to store, so the kind decides what is handed over.
	_, values, _ := rendered(t, "INSERT INTO t VALUES (?, ?, ?, ?)",
		rhosql.Value{Ordinal: 1, Kind: rhosql.ValueInteger, Text: "9007199254740993"},
		rhosql.Value{Ordinal: 2, Kind: rhosql.ValueFloat, Text: "1.5"},
		rhosql.Value{Ordinal: 3, Kind: rhosql.ValueText, Text: "hello"},
		rhosql.Value{Ordinal: 4, Kind: rhosql.ValueBoolean, Text: "true"},
	)
	whole, ok := values[0].(int64)
	if !ok {
		t.Fatalf("a whole number was handed over as %T, which is how a digit is lost", values[0])
	}
	if whole != 9007199254740993 {
		t.Errorf("the whole number was handed over as %d", whole)
	}
	if _, ok := values[1].(float64); !ok {
		t.Errorf("a number with a fraction was handed over as %T", values[1])
	}
	if values[2] != "hello" {
		t.Errorf("text was handed over as %v", values[2])
	}
	if values[3] != true {
		t.Errorf("a boolean was handed over as %v", values[3])
	}
}

func TestAnAbsentValueIsAbsent(t *testing.T) {
	// NULL is a value of its own: a statement distinguishes it from the empty
	// string and from zero, and a driver has to be told so rather than given
	// something that stands in for it.
	_, values, _ := rendered(t, "INSERT INTO t VALUES (?)",
		rhosql.Value{Ordinal: 1, Kind: rhosql.ValueNull, Null: true})
	if values[0] != nil {
		t.Errorf("an absent value was handed over as %v", values[0])
	}
}

func TestABlobIsHandedOverAsBytes(t *testing.T) {
	body := []byte{0x00, 0x01, 0xff, 0x00}
	_, values, _ := rendered(t, "INSERT INTO t VALUES (?)",
		rhosql.Value{Ordinal: 1, Kind: rhosql.ValueBlob, Blob: body})
	handed, ok := values[0].([]byte)
	if !ok {
		t.Fatalf("a blob was handed over as %T", values[0])
	}
	if string(handed) != string(body) {
		t.Errorf("the blob came out as %v", handed)
	}
}

func TestAMissingValueIsAbsentRatherThanShifted(t *testing.T) {
	// A statement with fewer values than markers is one the caller got wrong,
	// and a nil in the middle would send a later value to the wrong column.
	_, values, _ := rendered(t, "SELECT a, b, c FROM t WHERE x = ? AND y = ? AND z = ?",
		rhosql.Value{Ordinal: 1, Kind: rhosql.ValueText, Text: "one"},
		rhosql.Value{Ordinal: 3, Kind: rhosql.ValueText, Text: "three"},
	)
	if len(values) != 3 {
		t.Fatalf("the statement takes %d values, want 3", len(values))
	}
	if values[0] != "one" || values[2] != "three" {
		t.Errorf("the values came out as %v; a gap must stay a gap", values)
	}
	if values[1] != nil {
		t.Errorf("the value that was not given is %v, which would fill the wrong column", values[1])
	}
}

func TestAValueThatIsNotWhatItClaimsIsHandedOverAsText(t *testing.T) {
	// A value that claims to be a whole number and is not one is a caller that
	// got its argument wrong; handing it to the database as text means the
	// database refuses it, which is better than a silent zero.
	_, values, _ := rendered(t, "SELECT a FROM t WHERE x = ?",
		rhosql.Value{Ordinal: 1, Kind: rhosql.ValueInteger, Text: "not a number"})
	if values[0] != "not a number" {
		t.Errorf("a value that is not a number was handed over as %v", values[0])
	}
}

/* -------------------------------------------------------------------------- */
/*  Reading what came back                                                      */
/* -------------------------------------------------------------------------- */

func TestAValueThatCameBackKeepsItsKind(t *testing.T) {
	// A large whole number that came back as a float has already lost its last
	// digits, and a caller that never sees them cannot know.
	back := valueOf(int64(9007199254740993))
	if back.Kind != rhosql.ValueInteger {
		t.Errorf("a whole number came back as %q", back.Kind)
	}
	if back.Text != "9007199254740993" {
		t.Errorf("the whole number came back as %q", back.Text)
	}
	if valueOf(float64(1.5)).Kind != rhosql.ValueFloat {
		t.Error("a number with a fraction did not come back as one")
	}
	if valueOf(true).Kind != rhosql.ValueBoolean {
		t.Error("a boolean did not come back as one")
	}
	if valueOf("x").Kind != rhosql.ValueText {
		t.Error("text did not come back as text")
	}
	if valueOf(nil).Kind != rhosql.ValueNull || !valueOf(nil).Null {
		t.Error("an absent value did not come back as absent")
	}
}

func TestAFloatThatIsWholeDoesNotGrowATrailingZero(t *testing.T) {
	// A whole number stored as a float reads back with a decimal the database
	// never had, and a caller that puts it in a document is publishing a number
	// that is not the one stored.
	if got := valueOf(float64(2)).Text; got != "2" {
		t.Errorf("a whole number read back as %q", got)
	}
}

func TestBytesAreTextOnlyWhenTheyCanBe(t *testing.T) {
	// A blob whose bytes happen to be printable is still a blob, and a caller
	// that treated it as text would have lost a zero byte somewhere. What can be
	// told apart is what a zero byte marks.
	if _, ok := asText([]byte("hello")); !ok {
		t.Error("printable bytes were not recognised as text")
	}
	if _, ok := asText([]byte{'h', 0x00, 'i'}); ok {
		t.Error("bytes with a zero byte in them were taken for text")
	}
	// And one that comes back as text is text, whichever it was written as.
	if got := valueOf([]byte("hello")).Kind; got != rhosql.ValueText {
		t.Errorf("printable bytes came back as %q", got)
	}
	if got := valueOf([]byte{0x00, 0x01}).Kind; got != rhosql.ValueBlob {
		t.Errorf("bytes with a zero byte in them came back as %q", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  What the engine refuses                                                     */
/* -------------------------------------------------------------------------- */

func TestACallerCanTellWhatTheEngineRefused(t *testing.T) {
	// A caller that cannot tell "no rows" from "the engine refused" reads a
	// failure as a fact about the data, which is the failure a kind exists to
	// prevent.
	cases := map[string]rhosql.ErrorKind{
		"no such table: customers":     rhosql.ErrorNotFound,
		"no such column: nope":         rhosql.ErrorNotFound,
		"constraint failed: UNIQUE":    rhosql.ErrorInvalid,
		"datatype mismatch":            rhosql.ErrorInvalid,
		"unable to open database file": rhosql.ErrorEngine,
		"database is locked":           rhosql.ErrorEngine,
	}
	for message, want := range cases {
		failure, ok := engineError(errorOf(message), "SELECT 1").(rhosql.EngineError)
		if !ok {
			t.Errorf("%q was refused as %T, so a caller cannot read the kind", message,
				engineError(errorOf(message), "SELECT 1"))
			continue
		}
		if failure.Response.Kind != want {
			t.Errorf("%q was refused as %q, want %q", message, failure.Response.Kind, want)
		}
	}
}

func TestARefusalCarriesTheStatementItRefused(t *testing.T) {
	// A caller that gets a refusal with no statement cannot find the statement
	// among the ones it wrote.
	failure := engineError(errorOf("no such table: customers"), "SELECT id FROM customers")
	typed, ok := failure.(rhosql.EngineError)
	if !ok {
		t.Fatalf("the failure is a %T", failure)
	}
	if typed.Response.Details["query"] != "SELECT id FROM customers" {
		t.Errorf("the refusal carries %v, not the statement", typed.Response.Details)
	}
	// A refusal with no statement carries no empty one, which would read as a
	// statement called "".
	bare := engineError(errorOf("no such table"), "")
	if typed, ok := bare.(rhosql.EngineError); ok && len(typed.Response.Details) != 0 {
		t.Errorf("a refusal with no statement carries %v", typed.Response.Details)
	}
}

/* -------------------------------------------------------------------------- */
/*  What a migration creates                                                    */
/* -------------------------------------------------------------------------- */

func TestACreateKeepsWhatTheServiceWrote(t *testing.T) {
	// The declaration is written as the service wrote it, and only what this
	// dialect spells differently is changed. An engine that rewrote more would be
	// an engine deciding what a service meant.
	table := rhosql.Table{
		Name: "orders",
		Columns: []rhosql.Column{
			{Name: "customer", Type: "integer", NotNull: true},
			{Name: "reference", Type: "text", Default: "'a;b'"},
		},
		PrimaryKey: []string{"customer", "placed_at"},
	}
	created := createTable(table, false)
	for _, want := range []string{
		`CREATE TABLE "orders"`,
		`"customer" integer NOT NULL`,
		`DEFAULT 'a;b'`,
		`PRIMARY KEY ("customer", "placed_at")`,
	} {
		if !strings.Contains(created, want) {
			t.Errorf("the create does not contain %q:\n%s", want, created)
		}
	}
}

func TestAColumnThatIsTheKeySaysSo(t *testing.T) {
	// `id integer PRIMARY KEY` is an identity the database assigns; losing the
	// key creates a table that is not the one the service declared.
	created := createTable(rhosql.Table{
		Name:    "t",
		Columns: []rhosql.Column{{Name: "id", Type: "integer", PrimaryKey: true}},
	}, false)
	if !strings.Contains(created, `"id" integer PRIMARY KEY`) {
		t.Errorf("the key was lost:\n%s", created)
	}
}

func TestIfNotExistsSurvivesUnlessItIsForced(t *testing.T) {
	table := rhosql.Table{Name: "t", IfNotExists: true,
		Columns: []rhosql.Column{{Name: "id", Type: "integer"}}}
	if !strings.Contains(createTable(table, false), "IF NOT EXISTS") {
		t.Error("IF NOT EXISTS was lost; a service applying a schema twice fails the second time")
	}
	if strings.Contains(createTable(table, true), "IF NOT EXISTS") {
		t.Error("a forced migration kept IF NOT EXISTS, so the force does nothing")
	}
}

func TestANameIsQuotedWhateverItWasWrittenAs(t *testing.T) {
	// A mixed-case or reserved word survives a create only if it is quoted, and a
	// name that is already quoted must not be quoted twice.
	if got := quoteIdentifier("limit"); got != `"limit"` {
		t.Errorf("a name was written as %s", got)
	}
	if got := quoteIdentifier(`say "hi"`); got != `"say ""hi"""` {
		t.Errorf("a name containing a quote was written as %s", got)
	}
}
