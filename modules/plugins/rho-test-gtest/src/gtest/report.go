//
// @file report.go
// @brief Reading what GoogleTest wrote down, and saying it plainly.
//
// GoogleTest is given a file to write its results to, and that file is the one
// place the framework states its own outcome unambiguously: which test, inside
// which case, passed or failed or was skipped, and what it printed when it failed.
// So that is what this plugin reads.
//
// It does not parse the console output, because the console output is written to be
// read by a person — it is a table, with progress lines through it, and its exact
// shape is a presentation choice the framework is free to change. Reading a
// framework's report is reading a contract; reading its screen is guessing.
//
// What comes back out is one line per test in the form the runner already
// understands, which is the whole reason the run needs a parser at all: the
// runner's parser is for a step that says what happened, and this is a step that
// says it.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package gtest

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  GoogleTest's own report                                                    */
/* -------------------------------------------------------------------------- */

// report is the shape of the XML GoogleTest writes.
//
// Only the fields this plugin reads are named. The framework's report carries far
// more — timings, disabled tests, sharding information — and a reader that named
// all of it would be a reader that breaks when the framework adds one. What is
// needed to tell a person what happened is here.
type report struct {
	XMLName xml.Name `xml:"testsuites"`
	Suites  []suite  `xml:"testsuite"`
}

type suite struct {
	Name      string  `xml:"name,attr"`
	Tests     int     `xml:"tests,attr"`
	Failures  int     `xml:"failures,attr"`
	Disabled  int     `xml:"disabled,attr"`
	Skipped   int     `xml:"skipped,attr"`
	TestCases []gtest `xml:"testcase"`
}

type gtest struct {
	Name      string `xml:"name,attr"`
	ClassName string `xml:"classname,attr"`
	Time      string `xml:"time,attr"`
	// Status and Result are attributes, not child elements, and both carry part of
	// the answer: a test that ran says status="run", one that did not says
	// status="notrun" or result="suppressed". Which of them is present varies with
	// the framework's version, so both are read rather than one of them guessed at.
	Status  string   `xml:"status,attr"`
	Result  string   `xml:"result,attr"`
	Failure []fault  `xml:"failure"`
	Skipped *skipped `xml:"skipped"`
}

type skipped struct {
	Message string `xml:"message,attr"`
}

type fault struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

// Outcome is what one test turned out to be.
type Outcome struct {
	// Name is what the framework calls the test: its suite, then its case.
	Name string
	// Status is pass, fail or skip.
	Status string
	// Output is what the test printed, kept for the failure line under a report.
	Output string
}

// The statuses this plugin reports, which are the runner's own rather than the
// framework's.
const (
	StatusPass = "pass"
	StatusFail = "fail"
	StatusSkip = "skip"
)

/* -------------------------------------------------------------------------- */
/*  Reading it                                                                */
/* -------------------------------------------------------------------------- */

// Read parses a GoogleTest XML report.
//
// A report that cannot be read is an error rather than an empty result. A test
// binary that produced no report has not passed: it either crashed before the
// framework could write, or it is not a GoogleTest binary at all, and both are
// facts a run has to state rather than a green result it has to invent.
func Read(path string) ([]Outcome, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("google test wrote no report at %s: %w", path, err)
	}
	defer file.Close()
	return Parse(file)
}

// Parse reads a GoogleTest XML report from a stream.
func Parse(source io.Reader) ([]Outcome, error) {
	var parsed report
	if err := xml.NewDecoder(source).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("read the google test report: %w", err)
	}
	outcomes := []Outcome{}
	for _, one := range parsed.Suites {
		for _, test := range one.TestCases {
			outcomes = append(outcomes, outcomeOf(one, test))
		}
	}
	// Sorted by name, so two runs of the same suite report in the same order and a
	// diff between two reports is about what changed rather than about the order the
	// framework happened to schedule its tests in.
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].Name < outcomes[j].Name })
	return outcomes, nil
}

func outcomeOf(one suite, test gtest) Outcome {
	name := test.Name
	if test.ClassName != "" {
		name = test.ClassName + "." + test.Name
	} else if one.Name != "" {
		name = one.Name + "." + test.Name
	}
	outcome := Outcome{Name: name, Status: StatusPass}

	switch {
	case test.Skipped != nil:
		outcome.Status = StatusSkip
		if test.Skipped.Message != "" {
			outcome.Output = test.Skipped.Message
		} else {
			outcome.Output = "skipped"
		}
	case len(test.Failure) > 0:
		outcome.Status = StatusFail
		outcome.Output = failureText(test.Failure)
	case notRun(test):
		outcome.Status = StatusSkip
		outcome.Output = notRunReason(test)
	}
	return outcome
}

// notRun reports whether the framework said the test did not run.
//
// Which of the two attributes carries it varies — a disabled test is written one
// way and a filtered one another — so both are read. A test that has neither and
// has no failure has run and held, which is what an ordinary passing test looks
// like.
func notRun(test gtest) bool {
	return test.Status == "notrun" || test.Result == "suppressed" || test.Result == "skipped"
}

func notRunReason(test gtest) string {
	switch test.Result {
	case "suppressed":
		return "disabled: the framework did not run it"
	case "skipped":
		return "skipped"
	}
	return "the framework marked it not run"
}

// failureText is what a failing test said, most of it.
//
// The first line is what matters — it is the assertion — and the rest is the
// framework's stack and source, which is long and which a reader scrolls past. So
// it is cut short here rather than dumped into a report that a person scrolls.
func failureText(failures []fault) string {
	parts := []string{}
	for _, failure := range failures {
		text := strings.TrimSpace(failure.Text)
		if text == "" {
			text = failure.Message
		}
		if text == "" {
			continue
		}
		lines := strings.Split(text, "\n")
		if len(lines) > 4 {
			lines = append(lines[:4], "...")
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "\n")
}

/* -------------------------------------------------------------------------- */
/*  Saying it                                                                 */
/* -------------------------------------------------------------------------- */

// Lines renders outcomes as the lines a run's parser reads.
//
// The form is the runner's, not the framework's, which is the point: this is a
// step like any other, and a step says what happened in a form the run understands.
func Lines(outcomes []Outcome) []string {
	lines := make([]string, 0, len(outcomes))
	for _, outcome := range outcomes {
		switch outcome.Status {
		case StatusPass:
			lines = append(lines, "[PASS] "+outcome.Name)
		case StatusSkip:
			lines = append(lines, "[SKIP] "+outcome.Name+"  ("+firstLine(outcome.Output)+")")
		default:
			lines = append(lines, "[FAIL] "+outcome.Name)
			if outcome.Output != "" {
				for _, line := range strings.Split(outcome.Output, "\n") {
					lines = append(lines, "  "+line)
				}
			}
		}
	}
	return lines
}

func firstLine(text string) string {
	if index := strings.Index(text, "\n"); index >= 0 {
		return text[:index]
	}
	return text
}
