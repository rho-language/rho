//
// @file junit_test.go
// @brief Tests for the JUnit test-run plugin.
//
// The plugin is the worked example of a plugin that extends a component, so what
// is tested here is the contract: the request it is handed, the report it writes
// from it, and what it says when it is handed something it cannot read.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package junit

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
)

// request builds what a test run sends.
func request(results []Result) []byte {
	body := Request{Version: ProtocolVersion, Hook: HookRun, Command: "junit", Component: ComponentName}
	body.Payload.Root = "/project"
	body.Payload.Target = "go"
	body.Payload.Results = results
	for _, result := range results {
		if result.Passed() {
			body.Payload.Passed++
		}
	}
	body.Payload.Total = len(results)
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return encoded
}

func result(name, file string, line int, passed bool, output string) Result {
	// The run's own verdict, which is what a report reads: a report that
	// decided for itself what passed would be a second opinion on the run.
	status := "fail"
	if passed {
		status = "pass"
	}
	out := Result{Status: status, Output: output}
	out.Case.Name = name
	out.Case.File = file
	out.Case.Line = line
	return out
}

// answer runs the plugin and returns what it wrote.
func answer(t *testing.T, payload []byte) Response {
	t.Helper()
	var out bytes.Buffer
	if err := Run(bytes.NewReader(payload), &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	response := Response{}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("the plugin did not answer with the protocol: %v: %s", err, out.String())
	}
	return response
}

func TestARunBecomesAReportOfSuites(t *testing.T) {
	// A report of a project is a list of files, each with the cases it holds,
	// which is the shape every CI system reads.
	response := answer(t, request([]Result{
		result("Some holds a value", "/project/tests/option.rho", 7, true, ""),
		result("None holds nothing", "/project/tests/option.rho", 12, false, "assert failed"),
		result("strings joins", "/project/tests/strings.rho", 4, true, ""),
	}))

	document := parse(t, response.Payload.Report)
	if len(document.Suites) != 2 {
		t.Fatalf("expected one suite per test file, got %d", len(document.Suites))
	}
	if document.Suites[0].Name != "option.rho" || document.Suites[0].Tests != 2 {
		t.Fatalf("unexpected first suite: %+v", document.Suites[0])
	}
	if document.Suites[0].Failures != 1 {
		t.Fatalf("expected the failure to be counted, got %+v", document.Suites[0])
	}
	if len(document.Suites[1].Cases) != 1 {
		t.Fatalf("expected one case in the second suite, got %+v", document.Suites[1].Cases)
	}
	// The failing case carries why, which is the whole reason a machine reads
	// the document.
	failure := document.Suites[0].Cases[1].Failure
	if failure == nil || !strings.Contains(failure.Text, "assert failed") {
		t.Fatalf("expected the output of the failure, got %+v", failure)
	}
}

func TestAPassingRunIsReportedAsSuch(t *testing.T) {
	response := answer(t, request([]Result{
		result("works", "/project/tests/a.rho", 1, true, ""),
		result("also works", "/project/tests/b.rho", 1, true, ""),
	}))

	document := parse(t, response.Payload.Report)
	for _, suite := range document.Suites {
		if suite.Failures != 0 {
			t.Fatalf("expected no failures in a passing run, got %+v", suite)
		}
		if len(suite.Cases) != 1 || suite.Cases[0].Failure != nil {
			t.Fatalf("expected a case with nothing wrong with it, got %+v", suite.Cases)
		}
	}
	if len(response.Payload.Notes) != 1 || !strings.Contains(response.Payload.Notes[0], "2 cases") {
		t.Fatalf("expected a note saying what was reported, got %+v", response.Payload.Notes)
	}
}

func TestPathsAreReportedRelativeToTheProject(t *testing.T) {
	// A report is read by something that is not necessarily on the machine that
	// produced it, so a path that means nothing elsewhere is no use.
	response := answer(t, request([]Result{
		result("works", "/project/tests/a.rho", 1, true, ""),
	}))

	document := parse(t, response.Payload.Report)
	if document.Suites[0].Package != "tests/a.rho" {
		t.Fatalf("expected the path under the project, got %q", document.Suites[0].Package)
	}
}

func TestAPathOutsideTheProjectIsKeptWhole(t *testing.T) {
	// A test run over a directory that is not under the project root still has
	// to name the file it ran, so the path is kept rather than mangled.
	response := answer(t, request([]Result{
		result("works", "/elsewhere/tests/a.rho", 1, true, ""),
	}))

	document := parse(t, response.Payload.Report)
	if document.Suites[0].Package != "/elsewhere/tests/a.rho" {
		t.Fatalf("expected the path as it is, got %q", document.Suites[0].Package)
	}
}

func TestARequestInAnotherProtocolVersionIsReported(t *testing.T) {
	// A plugin written against another version says so, rather than answering
	// with something the runner would misread.
	body := Request{Version: ProtocolVersion + 1, Hook: HookRun}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response := answer(t, encoded)
	if len(response.Payload.Notes) != 1 || !strings.Contains(response.Payload.Notes[0], "speaks protocol") {
		t.Fatalf("expected the version mismatch to be reported, got %+v", response.Payload.Notes)
	}
	if response.Payload.Report != "" {
		t.Fatal("expected no report from a request the plugin cannot read")
	}
}

func TestAHookThePluginDoesNotAnswerIsNotAFailure(t *testing.T) {
	// A runner may offer more hooks than one plugin answers, so an answer to the
	// discovery hook is nothing rather than an error.
	body := Request{Version: ProtocolVersion, Hook: "discover", Command: "discover"}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response := answer(t, encoded)
	if response.Payload.Report != "" || len(response.Payload.Notes) != 0 {
		t.Fatalf("expected an empty answer, got %+v", response.Payload)
	}
}

// parse reads a report, so a test can ask about its shape rather than its text.
func parse(t *testing.T, report string) testsuites {
	t.Helper()
	document := testsuites{}
	if err := xml.Unmarshal([]byte(report), &document); err != nil {
		t.Fatalf("the report is not the document it claims to be: %v\n%s", err, report)
	}
	return document
}
