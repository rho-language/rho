//
// @file session_test.go
// @brief Tests for running a plugin that outlives one request.
//
// A session is the same protocol as a single run with a different lifetime, so
// the things worth checking are the lifetime and the compatibility: that a
// plugin is started once and answered many times, that a payload survives the
// round trip, that closing ends the process, and that a plugin written for the
// per-request runner is served correctly by a session as well.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package plugins

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// counterSource is a plugin that counts the requests it has answered, so a test
// can tell one process from many.
//
// It reads frames until its input ends, which is what a session asks of a
// plugin, and writes one answer per line. It is deliberately the shape a
// well-behaved plugin has under both runners.
const counterSource = `package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type request struct {
	Version   int             ` + "`json:\"version\"`" + `
	Hook      string          ` + "`json:\"hook\"`" + `
	Command   string          ` + "`json:\"command\"`" + `
	Component string          ` + "`json:\"component\"`" + `
	Payload   json.RawMessage ` + "`json:\"payload\"`" + `
}

type response struct {
	Version  int               ` + "`json:\"version\"`" + `
	Payload  json.RawMessage   ` + "`json:\"payload,omitempty\"`" + `
	Messages []json.RawMessage ` + "`json:\"messages,omitempty\"`" + `
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	seen := []json.RawMessage{}
	for scanner.Scan() {
		var in request
		if err := json.Unmarshal(scanner.Bytes(), &in); err != nil {
			continue
		}
		seen = append(seen, in.Payload)
		// The count is carried inside the payload rather than beside it, so a
		// test reads one shape: the envelope's payload is whatever the
		// component defined, and this one carries the request back with the
		// number of requests the process has answered beside it.
		body, _ := json.Marshal(struct {
			Echoed   json.RawMessage   ` + "`json:\"echoed\"`" + `
			Requests []json.RawMessage ` + "`json:\"requests\"`" + `
		}{in.Payload, seen})
		envelope, _ := json.Marshal(response{Version: 1, Payload: body})
		os.Stdout.Write(append(envelope, '\n'))
	}
}
`

// countingPlugin builds a plugin whose command counts the requests it answers.
func countingPlugin(t *testing.T) Plugin {
	t.Helper()
	root := t.TempDir()
	writeGoSource(t, filepath.Join(root, "main.go"), counterSource)

	program := filepath.Join(root, "counter")
	build := exec.Command("go", "build", "-o", program, ".")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the plugin did not build: %v\n%s", err, out)
	}
	return Plugin{
		Name:     "counter",
		Version:  "0.1.0",
		Root:     root,
		Extends:  "rho-sql",
		Hooks:    []string{"query"},
		Commands: []Command{{Name: "count", Run: "counter", Hook: "query"}},
	}
}

func writeGoSource(t *testing.T, path, source string) {
	t.Helper()
	module := "module counter\n\ngo 1.18\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "go.mod"), []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

// countIn answers how many requests the plugin has been asked, which is how a
// test tells a session from a per-request run.
//
// The count is a field of its own beside the payload, so a test can read it
// without decoding whatever payload a plugin happens to answer with.
func countIn(t *testing.T, answer Response) int {
	t.Helper()
	var document struct {
		Requests []json.RawMessage
	}
	if err := json.Unmarshal(answer.Payload, &document); err != nil {
		t.Fatalf("cannot read the answer: %v", err)
	}
	return len(document.Requests)
}

func TestASessionAnswersManyRequestsFromOneProcess(t *testing.T) {
	session, err := Open(SessionOptions{Plugin: countingPlugin(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	for i := 1; i <= 3; i++ {
		answer, err := session.Do("counter", "query", "count", map[string]string{"n": "x"})
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		// The plugin counts the requests it has been given, so a session that
		// started a process per request would answer 1 every time.
		if got := countIn(t, answer); got != i {
			t.Errorf("after %d requests the plugin had answered %d, so it was not one process", i, got)
		}
	}
}

func TestASessionIsNotStartedUntilItIsUsed(t *testing.T) {
	session, err := Open(SessionOptions{Plugin: countingPlugin(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	// A service that configures a database it never queries must not pay for
	// the connection, so opening is not starting.
	if session.started {
		t.Error("the session started its process before it was asked anything")
	}
}

func testPayload(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{"q": "select 1", "n": 1}
}

// readPayload returns the payload the plugin echoed back, which is how a test
// checks that a request survived the round trip intact.
func readPayload(t *testing.T, answer Response) map[string]any {
	t.Helper()
	var document struct {
		Echoed map[string]any
	}
	if err := json.Unmarshal(answer.Payload, &document); err != nil {
		t.Fatalf("cannot read the echoed payload: %v", err)
	}
	return document.Echoed
}

func TestTheEnvelopeSurvivesTheRoundTrip(t *testing.T) {
	session, err := Open(SessionOptions{Plugin: countingPlugin(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	answer, err := session.Do("counter", "query", "count", testPayload(t))
	if err != nil {
		t.Fatal(err)
	}
	if answer.Version != ProtocolVersion {
		t.Errorf("version = %d, want %d", answer.Version, ProtocolVersion)
	}
	echoed := readPayload(t, answer)
	if echoed["q"] != "select 1" {
		t.Errorf("the payload did not survive: %v", echoed)
	}
}

func TestClosingStopsTheProcess(t *testing.T) {
	session, err := Open(SessionOptions{Plugin: countingPlugin(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Do("counter", "query", "count", testPayload(t)); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// A closed session says so rather than silently starting a second process.
	if _, err := session.Do("counter", "query", "count", testPayload(t)); err == nil {
		t.Error("a closed session answered another request")
	}
}

func TestClosingTwiceIsSafe(t *testing.T) {
	session, err := Open(SessionOptions{Plugin: countingPlugin(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Do("counter", "query", "count", testPayload(t)); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// Cleanup calls close as well as the test, so closing twice is the normal
	// case rather than a mistake.
	if err := session.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestOpeningAnUnknownCommandIsRefused(t *testing.T) {
	_, err := Open(SessionOptions{Plugin: countingPlugin(t), Command: "nonesuch"})
	if err == nil {
		t.Fatal("a command the plugin does not have was run")
	}
	if !strings.Contains(err.Error(), "nonesuch") {
		t.Errorf("the refusal should name what was asked for: %v", err)
	}
}

func TestAPluginWithNoCommandIsRefused(t *testing.T) {
	plugin := countingPlugin(t)
	plugin.Commands = nil
	if _, err := Open(SessionOptions{Plugin: plugin}); err == nil {
		t.Fatal("a plugin with no command was opened")
	}
}
