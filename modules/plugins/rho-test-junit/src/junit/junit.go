//
// @file junit.go
// @brief A test-run plugin that writes a JUnit report.
//
// A test run is read by machines. A CI system wants a document it can parse
// rather than a line it has to read, and JUnit XML is the one nearly every one
// of them reads. So this plugin is a test-run plugin: it is handed the results
// of a run and answers with the report, which the runner writes to a file.
//
// The protocol is the toolchain's, over a shared envelope, so this plugin is
// written the same way as one that extends a documentation build:
//
//	{"version":1,"hook":"run","command":"junit","component":"rho-test",
//	 "payload":{"root":"...","target":"c","passed":3,"total":4,
//	            "results":[{"case":{"name":"...","file":"...","line":1,
//	                              "passed":false,"output":"..."}]}}
//
//	{"version":1,"payload":{"report":"<testsuite ...>...</testsuite>"}}
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package junit

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProtocolVersion is the version of the request and response shapes this plugin
// was written against. A generator that speaks another version is reported
// rather than answered with something it would misread.
const ProtocolVersion = 1

// ComponentName is the component this plugin extends, which is what makes a
// test run load it at all.
const ComponentName = "rho-test"

// HookRun is the hook this plugin answers: once, over the results of a finished
// run.
const HookRun = "run"

/* -------------------------------------------------------------------------- */
/*  The protocol                                                              */
/* -------------------------------------------------------------------------- */

// Request is what a test run sends. The envelope is the toolchain's, so only the
// fields this plugin reads are spelled out here.
type Request struct {
	Version   int    `json:"version"`
	Hook      string `json:"hook"`
	Command   string `json:"command"`
	Component string `json:"component"`
	Payload   struct {
		Root    string   `json:"root"`
		Target  string   `json:"target"`
		Passed  int      `json:"passed"`
		Total   int      `json:"total"`
		Results []Result `json:"results"`
	} `json:"payload"`
}

// Result is one test case's outcome, as a test run reports it.
type Result struct {
	Case struct {
		Name string `json:"name"`
		File string `json:"file"`
		Line int    `json:"line"`
	} `json:"case"`
	// Status is what the run concluded: `pass`, `fail` or `skip`.
	//
	// The status rather than a verdict of its own, because the run is what
	// concluded: a report that decided for itself what passed would be a
	// second opinion on the run, and a run that passed would be reported as
	// failed by a report that never read it.
	Status string `json:"status"`
	Reason string `json:"reason"`
	Output string `json:"output"`
}

// Passed reports whether the case passed.
func (r Result) Passed() bool { return r.Status == "pass" }

// Response is what this plugin answers with: the report it wrote, and a note
// saying where it was written.
type Response struct {
	Version int `json:"version"`
	Payload struct {
		Report string   `json:"report"`
		Notes  []string `json:"notes"`
	} `json:"payload"`
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                   */
/* -------------------------------------------------------------------------- */

// Run answers one request, reading it from standard input and writing the answer
// to standard output.
func Run(input io.Reader, output io.Writer) error {
	payload, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("cannot read the request: %w", err)
	}
	var request Request
	if err := json.Unmarshal(payload, &request); err != nil {
		return fmt.Errorf("cannot read the request: %w", err)
	}
	if request.Version != ProtocolVersion {
		return json.NewEncoder(output).Encode(Response{
			Version: ProtocolVersion,
			Payload: struct {
				Report string   `json:"report"`
				Notes  []string `json:"notes"`
			}{Notes: []string{fmt.Sprintf("rho-test-junit speaks protocol %d, the runner speaks %d", ProtocolVersion, request.Version)}},
		})
	}
	if request.Hook != HookRun {
		// A hook this plugin does not answer is not an error: a runner may offer
		// more than one, and a plugin answers the one it was written for.
		return json.NewEncoder(output).Encode(Response{Version: ProtocolVersion})
	}

	report := Document(request.Payload.Root, request.Payload.Target, request.Payload.Results)
	response := Response{Version: ProtocolVersion}
	response.Payload.Report = report
	// The note is the file the runner writes the report to, so a person
	// reading a run knows where the machine-readable version is.
	response.Payload.Notes = []string{fmt.Sprintf("junit report: %d cases, %d passed", len(request.Payload.Results), request.Payload.Passed)}
	encoded, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = output.Write(encoded)
	return err
}

/* -------------------------------------------------------------------------- */
/*  The report                                                                */
/* -------------------------------------------------------------------------- */

// suite is one testsuite element, which is what a suite of tests is called in
// the format: one element per test file, each holding its cases.
type suite struct {
	XMLName  xml.Name   `xml:"testsuite"`
	Name     string     `xml:"name,attr"`
	Tests    int        `xml:"tests,attr"`
	Failures int        `xml:"failures,attr"`
	Errors   int        `xml:"errors,attr"`
	Skipped  int        `xml:"skipped,attr"`
	Time     string     `xml:"time,attr"`
	Package  string     `xml:"package,attr,omitempty"`
	Cases    []testcase `xml:"testcase"`
}

// testcase is one testcase element: a test function and what came of it.
type testcase struct {
	XMLName xml.Name `xml:"testcase"`
	Name    string   `xml:"name,attr"`
	Class   string   `xml:"classname,attr"`
	Time    string   `xml:"time,attr"`
	Failure *failure `xml:"failure,omitempty"`
	Skipped *skipped `xml:"skipped,omitempty"`
}

type skipped struct {
	Message string `xml:"message,attr,omitempty"`
}

// failure is what a case carries when it did not pass.
type failure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

// Document renders a run as a JUnit report.
//
// A suite is a test file and a class is the test function, which is the shape
// every reader expects: a report of a project is a list of files, each with the
// cases it holds.
func Document(root, target string, results []Result) string {
	byFile := map[string]*suite{}
	order := []string{}
	for _, result := range results {
		name := result.Case.File
		if name == "" {
			name = "tests"
		}
		found, seen := byFile[name]
		if !seen {
			found = &suite{
				Name:    filepath.Base(name),
				Package: relative(root, name),
				Time:    "0",
			}
			byFile[name] = found
			order = append(order, name)
		}
		found.Tests++
		entry := testcase{
			Name:  result.Case.Name,
			Class: relative(root, name),
			Time:  "0",
		}
		if result.Status == "skip" {
			found.Skipped++
			entry.Skipped = &skipped{Message: result.Reason}
		} else if !result.Passed() {
			found.Failures++
			entry.Failure = &failure{
				Message: "test failed",
				Type:    "failure",
				Text:    strings.TrimSpace(result.Output),
			}
		}
		found.Cases = append(found.Cases, entry)
	}

	suites := make([]suite, 0, len(order))
	for _, name := range order {
		suites = append(suites, *byFile[name])
	}
	report := testsuites{Suites: suites}
	encoded, err := xml.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Sprintf("<!-- the report could not be rendered: %v -->", err)
	}
	var b strings.Builder
	b.WriteString(xml.Header)
	b.Write(encoded)
	b.WriteString("\n<!-- generated by rho-test-junit, a plugin for ")
	b.WriteString(ComponentName)
	b.WriteString(" on the ")
	b.WriteString(target)
	b.WriteString(" target, at ")
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	b.WriteString(" -->\n")
	return b.String()
}

// testsuites is the document element: the suites of one run.
type testsuites struct {
	XMLName xml.Name `xml:"testsuites"`
	Suites  []suite  `xml:"testsuite"`
}

// relative is a path as it should read in a report: under the project, and
// always with forward slashes, because a report is read by something that is not
// necessarily running on the machine that produced it.
func relative(root, path string) string {
	if root == "" {
		return filepath.ToSlash(path)
	}
	if shortened, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(shortened, "..") {
		return filepath.ToSlash(shortened)
	}
	return filepath.ToSlash(path)
}

// Duration is how a case's duration is written, in the seconds the format
// expects. It is here so a caller does not have to know that.
func Duration(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}

// Main runs the plugin, which is what the module's command is.
func Main() int {
	if err := Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "rho-test-junit:", err)
		return 1
	}
	return 0
}
