//
// @file serve_test.go
// @brief Tests for what an engine plugin is asked and how it answers.
//
// A plugin is a process another program runs, over a protocol neither side
// controls at the far end. So what is tested here is the protocol rather than
// the engine: that a request is answered, that a hook an engine does not have is
// refused by name rather than panicking, and that a request from something else
// is not served at all.
//
// The engine is a small stand-in, because the point of these is the boundary.
// What the boundary does with an engine's own numbers is tested in the engine's
// own package, against a real database.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhosql

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"rho/src/plugins"
)

// theEngine is an engine that answers every hook, and records what it was asked.
type theEngine struct {
	described bool
	migrated  bool
	ran       bool
	typed     bool
}

func (e *theEngine) Describe() Engine {
	e.described = true
	return Engine{Name: "stand-in", Version: "1.0", Markers: []Marker{MarkerPositional}}
}

func (e *theEngine) Migrate(request MigrateRequest) (MigrateResponse, error) {
	e.migrated = true
	applied := []string{}
	for _, table := range request.Schema.Tables {
		applied = append(applied, table.Name)
	}
	return MigrateResponse{Applied: applied}, nil
}

func (e *theEngine) Query(request QueryRequest) (QueryResponse, error) {
	e.ran = true
	return QueryResponse{
		Columns: []ResultColumn{{Name: "id", Type: "INTEGER"}},
		Rows:    [][]Value{{{Kind: ValueInteger, Text: "1"}}},
		SQL:     request.Query.SQL,
	}, nil
}

func (e *theEngine) DescribeQuery(request DescribeQueryRequest) (DescribeQueryResponse, error) {
	e.typed = true
	return DescribeQueryResponse{
		Columns: []ResultColumn{{Name: "id", Type: "INTEGER"}},
		SQL:     request.Query.SQL,
	}, nil
}

// bare is an engine with only the hook every engine must have: a caller cannot
// send it anything without knowing what it is.
type bare struct{ described bool }

func (e *bare) Describe() Engine {
	e.described = true
	return Engine{Name: "bare"}
}

// ask runs one hook against an engine and returns what it wrote.
func ask(t *testing.T, engine any, hook string, payload any) plugins.Response {
	t.Helper()
	request := plugins.Request{
		Version:   plugins.ProtocolVersion,
		Hook:      hook,
		Command:   "stand-in",
		Component: ComponentName,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request.Payload = encoded
	line, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	Serve(bytes.NewReader(append(line, '\n')), &out, engine)
	if out.Len() == 0 {
		t.Fatalf("the engine answered nothing to %s", hook)
	}
	answer := plugins.Response{}
	if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
		t.Fatalf("the answer to %s is not the envelope: %v\n%s", hook, err, out.String())
	}
	if answer.Version != plugins.ProtocolVersion {
		t.Errorf("the answer to %s carries version %d, want %d",
			hook, answer.Version, plugins.ProtocolVersion)
	}
	return answer
}

func TestEachHookAnswersItsOwnShape(t *testing.T) {
	engine := &theEngine{}
	schema := Schema{Tables: []Table{{Name: "t", Columns: []Column{{Name: "id"}}}}}
	query := Query{Name: "q", Verb: VerbSelect, SQL: "SELECT id FROM t"}

	migrated := ask(t, engine, HookMigrate, MigrateRequest{Schema: schema})
	if !engine.migrated {
		t.Fatal("migrate did not reach the engine")
	}
	applied := MigrateResponse{}
	if err := json.Unmarshal(migrated.Payload, &applied); err != nil {
		t.Fatal(err)
	}
	if len(applied.Applied) != 1 || applied.Applied[0] != "t" {
		t.Errorf("the migration applied %v", applied.Applied)
	}

	ran := ask(t, engine, HookQuery, QueryRequest{Name: "q", Query: query})
	if !engine.ran {
		t.Fatal("query did not reach the engine")
	}
	rows := QueryResponse{}
	if err := json.Unmarshal(ran.Payload, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows.Rows) != 1 {
		t.Errorf("the statement returned %d rows", len(rows.Rows))
	}

	typed := ask(t, engine, HookDescribeQuery, DescribeQueryRequest{Name: "q", Query: query})
	if !engine.typed {
		t.Fatal("describe_query did not reach the engine")
	}
	shape := DescribeQueryResponse{}
	if err := json.Unmarshal(typed.Payload, &shape); err != nil {
		t.Fatal(err)
	}
	if len(shape.Columns) != 1 {
		t.Errorf("the statement was described as returning %d columns", len(shape.Columns))
	}
}

func TestAHookAnEngineDoesNotHaveIsRefusedByName(t *testing.T) {
	// A read-only engine does not have to write a migrate that refuses
	// everything. The refusal has to name the hook, because a caller that
	// cannot tell which hook was refused cannot decide what to do about it.
	engine := &bare{}
	answer := ask(t, engine, HookMigrate, MigrateRequest{Schema: Schema{}})
	if len(answer.Diagnostics) == 0 {
		t.Fatal("a hook the engine does not have was answered as though it did")
	}
	found := false
	for _, item := range answer.Diagnostics {
		if strings.Contains(item.Message, HookMigrate) {
			found = true
		}
	}
	if !found {
		t.Errorf("the refusal does not name the hook: %v", answer.Diagnostics)
	}
}

func TestARequestForSomethingElseIsNotServed(t *testing.T) {
	// A plugin is run by a generator it knows; a request that names another
	// component is a request from something that has mislaid it, and answering it
	// would be answering as though it were ours.
	engine := &theEngine{}
	request := plugins.Request{
		Version:   plugins.ProtocolVersion,
		Hook:      HookQuery,
		Command:   "stand-in",
		Component: "rho-doc",
		Payload:   json.RawMessage(`{}`),
	}
	line, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	Serve(bytes.NewReader(append(line, '\n')), &out, engine)

	answer := plugins.Response{}
	if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
		t.Fatalf("the refusal is not the envelope: %v", err)
	}
	if engine.ran {
		t.Error("a statement was run for a request that named another component")
	}
	if len(answer.Diagnostics) == 0 {
		t.Error("a request for another component was answered without a reason")
	}
}

func TestAProtocolItDoesNotSpeakIsRefused(t *testing.T) {
	// A version it does not know is a version whose shape it would misread, and
	// misreading it is worse than refusing it.
	engine := &theEngine{}
	request := plugins.Request{
		Version:   plugins.ProtocolVersion + 1,
		Hook:      HookQuery,
		Component: ComponentName,
	}
	line, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	Serve(bytes.NewReader(append(line, '\n')), &out, engine)
	answer := plugins.Response{}
	if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
		t.Fatalf("the refusal is not the envelope: %v", err)
	}
	if len(answer.Diagnostics) == 0 {
		t.Error("a request in another protocol version was served")
	}
}

func TestARequestThatCannotBeReadIsRefused(t *testing.T) {
	// A line that is not a request is a caller that is not this one, and a
	// plugin that answered it would be answering whatever arrived.
	var out bytes.Buffer
	Serve(strings.NewReader("this is not a request\n"), &out, &theEngine{})
	answer := plugins.Response{}
	if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
		t.Fatalf("the refusal is not the envelope: %v", err)
	}
	if len(answer.Diagnostics) == 0 {
		t.Error("a line that is not a request was answered without a reason")
	}
}

func TestEveryRequestIsAnsweredOnceAndTheSessionEndsWithItsInput(t *testing.T) {
	// A session is a program that is asked many times: several requests in,
	// one answer each, and nothing after the input ends.
	engine := &theEngine{}
	var out bytes.Buffer
	var in bytes.Buffer
	for i := 0; i < 3; i++ {
		request := plugins.Request{
			Version:   plugins.ProtocolVersion,
			Hook:      HookDescribe,
			Command:   "stand-in",
			Component: ComponentName,
		}
		line, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		in.Write(append(line, '\n'))
	}
	Serve(&in, &out, engine)

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("three requests produced %d answers", len(lines))
	}
	for i, line := range lines {
		answer := plugins.Response{}
		if err := json.Unmarshal([]byte(line), &answer); err != nil {
			t.Fatalf("answer %d is not the envelope: %v", i, err)
		}
	}
}

func TestABlankLineIsNotARequest(t *testing.T) {
	// A session is line-framed, and a blank line is a frame with nothing in it.
	// Answering it would put an answer in the stream that no question asked for.
	var out bytes.Buffer
	Serve(strings.NewReader("\n\n"), &out, &theEngine{})
	if strings.TrimSpace(out.String()) != "" {
		t.Errorf("blank input was answered: %q", out.String())
	}
}
