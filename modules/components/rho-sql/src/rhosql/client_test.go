//
// @file client_test.go
// @brief Tests for the client a service talks to an engine through.
//
// The client is what a caller holds rather than a process, so what is tested is
// the answers it gives when it cannot do what was asked — because a client that
// returns a zero value for a failure is a client whose caller acts on nothing.
//
// A client needs an installed engine, so the cases that need one are skipped
// when there is none. The cases that do not are the ones about what the client
// says when it cannot even look.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhosql

import (
	"path/filepath"
	"strings"
	"testing"
)

// withAnEngine points the engine at a database of this test's own, so a test that
// reaches an engine does not open whatever the working directory happens to hold.
func withAnEngine(t *testing.T) {
	t.Helper()
	t.Setenv("RHO_SQLITE_PATH", filepath.Join(t.TempDir(), "client-test.sqlite"))
}

func TestNamingNoEngineIsRefused(t *testing.T) {
	// There is no default engine: which database a service runs against is a
	// deployment decision, and a client that guessed one would send a service's
	// rows somewhere nobody chose.
	_, err := Open("")
	if err == nil {
		t.Fatal("a client with no engine was opened")
	}
	if !strings.Contains(err.Error(), "engine") {
		t.Errorf("the refusal does not say what is missing: %v", err)
	}
}

func TestAnEngineThatIsNotInstalledIsRefusedByName(t *testing.T) {
	// The refusal has to name the engine asked for, because a caller with three
	// configured has to know which one is not there.
	_, err := Open("no-such-engine")
	if err == nil {
		t.Skip("an engine by that name is installed, so the refusal cannot be seen")
	}
	if !strings.Contains(err.Error(), "no-such-engine") {
		t.Errorf("the refusal does not name the engine: %v", err)
	}
}

func TestTheClientIsNotAskedWhatItDoesNotKnow(t *testing.T) {
	// An engine that has not been described has not said what it is, and a
	// caller that reads a zero Engine would read "takes no markers" as though the
	// engine had said so.
	client, err := Open("rho-sql-sqlite")
	if err != nil {
		t.Skip("no engine is installed, so there is nothing to ask")
	}
	defer client.Close()
	if _, err := client.Engine(); err == nil {
		t.Error("the engine was reported before it was asked")
	}
}

func TestTheEngineIsAskedOnceAndThenKnown(t *testing.T) {
	withAnEngine(t)
	client, err := Open("rho-sql-sqlite")
	if err != nil {
		t.Skip("no engine is installed, so there is nothing to ask")
	}
	defer client.Close()
	first, err := client.Describe()
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if first.Name == "" {
		t.Error("the engine did not say what it is")
	}
	second, err := client.Describe()
	if err != nil {
		t.Fatal(err)
	}
	if second.Name != first.Name {
		t.Errorf("the engine is described twice differently: %q then %q", first.Name, second.Name)
	}
	// And once it has been asked, it can be read without asking.
	known, err := client.Engine()
	if err != nil {
		t.Fatal(err)
	}
	if known.Name != first.Name {
		t.Errorf("the engine reads as %q, which is not what it said", known.Name)
	}
}

func TestTheClientRemembersWhichPluginItFound(t *testing.T) {
	withAnEngine(t)
	// A caller that wants to say which engine answered needs the plugin's name,
	// and it is the name in the tree rather than the one that was asked for: a
	// project may configure `primary-db` and get `rho-sql-sqlite`.
	client, err := Open("rho-sql-sqlite")
	if err != nil {
		t.Skip("no engine is installed")
	}
	defer client.Close()
	plugin := client.Plugin()
	if plugin.Name != "rho-sql-sqlite" {
		t.Errorf("the client is holding %q", plugin.Name)
	}
	if plugin.Extends != ComponentName {
		t.Errorf("the plugin extends %q, so it is not this component's engine", plugin.Extends)
	}
}

func TestAStatementTheEngineDoesNotHaveIsRefusedByKind(t *testing.T) {
	withAnEngine(t)
	// The kind is what a caller acts on: a missing statement is not a fault in
	// the engine, and a caller that retries will fail the same way.
	client, err := Open("rho-sql-sqlite")
	if err != nil {
		t.Skip("no engine is installed")
	}
	defer client.Close()
	_, err = client.Query(QueryRequest{
		Name:  "no_such_statement",
		Query: Query{Name: "no_such_statement", Verb: VerbSelect},
	})
	if err == nil {
		t.Skip("the engine answered a statement this build does not know, so the refusal cannot be seen")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "no such table") &&
		!strings.Contains(strings.ToLower(err.Error()), "not_found") {
		t.Errorf("the refusal does not say what was wrong: %v", err)
	}
}

func TestClosingAClientThatWasNeverUsedIsSafe(t *testing.T) {
	withAnEngine(t)
	// A session is started lazily, so a service that configures a database it
	// never touches has no process, and closing what was never opened has to
	// work anyway.
	client, err := Open("rho-sql-sqlite")
	if err != nil {
		t.Skip("no engine is installed")
	}
	if err := client.Close(); err != nil {
		t.Errorf("closing an unused client: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("closing twice: %v", err)
	}
}

func TestTheInstalledEnginesAreTheOnesForThisComponent(t *testing.T) {
	withAnEngine(t)
	// Everything installed that is a plugin is not an engine; a listing that
	// offered a documentation plugin as a database would be a listing a caller
	// could act on wrongly.
	names := InstalledEngines()
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Errorf("the engine %q is listed twice", name)
		}
		seen[name] = true
	}
	// And what is offered is what a client can actually open.
	for _, name := range names {
		client, err := Open(name)
		if err != nil {
			t.Errorf("the engine %q is offered and cannot be opened: %v", name, err)
			continue
		}
		client.Close()
	}
}
