//
// @file ast_test.go
// @brief Tests for the contract's own shape: its lookups and its spelling.
//
// A contract is read by other programs, so the questions they ask of it — which
// table is this, what columns does it have, does this engine take that marker —
// are part of the interface rather than a convenience. The cases here are the ones
// where being wrong is quiet: a lookup that misses because of case, a hook list
// that drifts from the hooks a plugin declares, a scheme named twice.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import "testing"

// theContract is a small contract, written out rather than parsed so that a
// failure is about the shape and not about the reading.
func theContract() Schema {
	return Schema{
		Version: Version,
		Module:  "catalog",
		Tables: []Table{{
			Name:       "customers",
			Comment:    "Customers of the catalogue.",
			PrimaryKey: []string{"id"},
			Columns: []Column{
				{Name: "id", Type: "integer", PrimaryKey: true},
				{Name: "address", Type: "text", NotNull: true},
				{Name: "name", Type: "text"},
			},
		}},
		Queries: []Query{{
			Name:       "find_customer",
			Verb:       VerbSelect,
			Table:      "customers",
			SQL:        "SELECT id, name FROM customers WHERE address = :address",
			Parameters: []Parameter{{Name: "address", Ordinal: 1, Marker: MarkerNamed}},
		}},
	}
}

func TestALookupFindsWhatItNames(t *testing.T) {
	schema := theContract()
	if table, found := schema.Table("customers"); !found || table.Name != "customers" {
		t.Errorf("the customers table was not found: %+v", table)
	}
	if query, found := schema.Query("find_customer"); !found || query.Verb != VerbSelect {
		t.Errorf("the find_customer statement was not found: %+v", query)
	}
	table, _ := schema.Table("customers")
	if column, found := table.Column("address"); !found || column.NotNull != true {
		t.Errorf("the address column was not found, or its constraint was lost: %+v", column)
	}
	if _, found := schema.Table("nowhere"); found {
		t.Error("a table that is not there was found")
	}
	if _, found := schema.Query("nowhere"); found {
		t.Error("a statement that is not there was found")
	}
}

func TestALookupIsNotFooledByCase(t *testing.T) {
	// SQL names are not case sensitive in every engine and are in some, so a
	// lookup that misses where one spelling would have matched is a lookup that
	// reports a column the service wrote.
	schema := theContract()
	if _, found := schema.Table("CUSTOMERS"); !found {
		t.Error("a table was not found under a different case")
	}
	table, _ := schema.Table("customers")
	if _, found := table.Column("ADDRESS"); !found {
		t.Error("a column was not found under a different case")
	}
	query, _ := schema.Query("find_customer")
	if _, found := query.Parameter("ADDRESS"); !found {
		t.Error("a parameter was not found under a different case")
	}
}

func TestACaseComparisonIsNotALengthComparison(t *testing.T) {
	// The one that makes a lookup work on a name of a different length is the
	// one that makes a lookup miss.
	if equalFold("id", "address") {
		t.Error("two names of different lengths compared equal")
	}
	if equalFold("", "") {
		t.Error("two empty names compared equal; an absent name is not a name")
	}
	if !equalFold("Id", "iD") {
		t.Error("two spellings of one name did not compare equal")
	}
}

func TestColumnsAreListedInTheOrderTheyWereDeclared(t *testing.T) {
	// The order is the order a row is in, and a caller that reads columns by
	// position rather than by name needs it to be the declared one.
	schema := theContract()
	table, _ := schema.Table("customers")
	names := table.ColumnNames()
	want := []string{"id", "address", "name"}
	if len(names) != len(want) {
		t.Fatalf("the columns read as %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("column %d is %q, want %q", i, names[i], want[i])
		}
	}
}

func TestTheKeyIsListedInItsOwnOrder(t *testing.T) {
	// A composite key's order is the key: `PRIMARY KEY (a, b)` is not the same
	// as `PRIMARY KEY (b, a)`, and a caller reading one and writing the other
	// writes a key the database will refuse.
	schema := Schema{Tables: []Table{{
		Name:       "orders",
		PrimaryKey: []string{"customer", "placed_at"},
	}}}
	table, _ := schema.Table("orders")
	if len(table.PrimaryKey) != 2 ||
		table.PrimaryKey[0] != "customer" || table.PrimaryKey[1] != "placed_at" {
		t.Errorf("the key read as %v, in the order it was written", table.PrimaryKey)
	}
}

func TestAnErrorIsAnErrorAndAWarningIsNot(t *testing.T) {
	// They read the same way to a person, so the distinction has to be in the
	// level: a statement that cannot be used is an error, and a file being
	// written is a note.
	if !HasErrors([]Diagnostic{{Level: "error"}, {Level: "warning"}}) {
		t.Error("a diagnostic with an error in it was not an error")
	}
	if HasErrors([]Diagnostic{{Level: "warning"}}) {
		t.Error("a warning was treated as an error")
	}
	if HasErrors(nil) {
		t.Error("nothing at all was treated as an error")
	}
}

func TestAnEngineIsAskedWhetherItSpeaksAMarker(t *testing.T) {
	// A caller checks this before it sends a statement written in a marker, so
	// an engine that answers wrongly is one that gets a statement it cannot run.
	engine := Engine{Markers: []Marker{MarkerPositional, MarkerNumbered}}
	if !engine.Speaks(MarkerPositional) {
		t.Error("the engine does not say it takes a marker it declared")
	}
	if engine.Speaks(MarkerNamed) {
		t.Error("the engine says it takes a marker it did not declare")
	}
	// An engine that declares nothing takes nothing, which is the safe answer:
	// a caller that sends a marker anyway gets a statement the engine refuses,
	// rather than a value in the wrong place.
	if (Engine{}).Speaks(MarkerPositional) {
		t.Error("an engine that declared nothing says it takes a marker")
	}
}

func TestTheHooksAnEngineIsOfferedAreTheOnesAPluginCanName(t *testing.T) {
	// A plugin declares the hooks it answers and the validator measures them
	// against what the component offers. If the two lists drift, a plugin is
	// refused for a hook this component still runs, or accepted for one it does
	// not.
	want := []string{HookDescribe, HookMigrate, HookQuery, HookDescribeQuery}
	if len(OfferedHooks) != len(want) {
		t.Fatalf("the component offers %v, want %v", OfferedHooks, want)
	}
	seen := map[string]bool{}
	for _, hook := range OfferedHooks {
		if seen[hook] {
			t.Errorf("the hook %q is offered twice", hook)
		}
		seen[hook] = true
	}
	for _, hook := range want {
		if !seen[hook] {
			t.Errorf("the hook %q is not offered, so a plugin may not answer it", hook)
		}
	}
}

func TestTheComponentNamesItself(t *testing.T) {
	// A plugin has to say which component it extends for the validator to load
	// it, and the component knows its own name without being told.
	if ComponentName != "rho-sql" {
		t.Errorf("the component is named %q", ComponentName)
	}
}

func TestAnEngineErrorReadsAsItsKindAndItsReason(t *testing.T) {
	// A caller acts on the kind and reads the message, so both have to survive
	// being carried through the boundary.
	failure := EngineError{Response: ErrorResponse{
		Kind: ErrorNotFound, Message: "no such table: customers",
	}}
	if got := failure.Error(); got != "not_found: no such table: customers" {
		t.Errorf("the failure reads as %q", got)
	}
	// An error with no message still names its kind, so a caller that prints it
	// says something rather than nothing.
	bare := EngineError{Response: ErrorResponse{Kind: ErrorEngine}}
	if got := bare.Error(); got != "engine" {
		t.Errorf("a failure with no reason reads as %q", got)
	}
}

func TestTheMarkersAreTheThreeStatementsAreWrittenIn(t *testing.T) {
	// The whole point of carrying a marker on a parameter is that an engine
	// knows how it is written, and these are the three ways it is written.
	for _, marker := range []Marker{MarkerPositional, MarkerNumbered, MarkerNamed} {
		if marker == "" {
			t.Error("a marker is unnamed, so an engine cannot tell it from none")
		}
	}
	// And they are distinct, because two spellings that reduced to one would
	// lose the difference a rendering is made of.
	if MarkerPositional == MarkerNumbered || MarkerNumbered == MarkerNamed ||
		MarkerPositional == MarkerNamed {
		t.Error("two markers have the same spelling")
	}
}

func TestTheVerbsAreTheFourAStatementCanBe(t *testing.T) {
	// A statement that is none of them is one this reading does not model, and
	// it is reported rather than filed under the nearest.
	for _, verb := range []Verb{VerbSelect, VerbInsert, VerbUpdate, VerbDelete} {
		if verb == "" {
			t.Error("a verb is unnamed")
		}
	}
	if VerbSelect == VerbInsert {
		t.Error("a select and an insert are the same verb")
	}
}

func TestAVersionIsPartOfEveryContract(t *testing.T) {
	// A consumer that does not recognise the version says so rather than
	// guessing at a shape it does not know, so the version has to be there.
	if Version < 1 {
		t.Errorf("the contract's version is %d", Version)
	}
	if theContract().Version != Version {
		t.Error("a contract does not carry the version this component writes")
	}
}
