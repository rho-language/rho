//
// @file report.go
// @brief Reading the JUnit report CTest writes.
//
// `ctest --output-junit` is where CTest states its results unambiguously: which
// test, and whether it passed, failed or was not run. Reading the console would be
// guessing at a format meant for people, so the run step writes this file and the
// report step reads it, and the runner sees one line per test.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package ctest

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// report is the document: CTest writes one <testsuite>, and other producers wrap
// it in <testsuites>, so both are read.
type report struct {
	XMLName xml.Name
	Suites  []suite    `xml:"testsuite"`
	Cases   []testcase `xml:"testcase"`
}

type suite struct {
	Name  string     `xml:"name,attr"`
	Cases []testcase `xml:"testcase"`
}

type testcase struct {
	Name      string   `xml:"name,attr"`
	ClassName string   `xml:"classname,attr"`
	Status    string   `xml:"status,attr"`
	Failure   []fault  `xml:"failure"`
	Error     []fault  `xml:"error"`
	Skipped   *skipped `xml:"skipped"`
	Output    string   `xml:"system-out"`
}

type skipped struct {
	Message string `xml:"message,attr"`
}

type fault struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

// Outcome is one test's result.
type Outcome struct {
	Name   string
	Status string
	Output string
}

const (
	StatusPass = "pass"
	StatusFail = "fail"
	StatusSkip = "skip"
)

// Read reads a report from a file.
func Read(path string) ([]Outcome, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("ctest wrote no report at %s: %w", path, err)
	}
	defer file.Close()
	return Parse(file)
}

// Parse reads a report.
func Parse(source io.Reader) ([]Outcome, error) {
	var parsed report
	if err := xml.NewDecoder(source).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("read the ctest report: %w", err)
	}
	cases := append([]testcase{}, parsed.Cases...)
	for _, one := range parsed.Suites {
		cases = append(cases, one.Cases...)
	}
	outcomes := make([]Outcome, 0, len(cases))
	for _, test := range cases {
		outcomes = append(outcomes, outcomeOf(test))
	}
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].Name < outcomes[j].Name })
	return outcomes, nil
}

func outcomeOf(test testcase) Outcome {
	outcome := Outcome{Name: test.Name, Status: StatusPass}
	switch {
	case len(test.Failure) > 0 || len(test.Error) > 0:
		outcome.Status = StatusFail
		outcome.Output = failureText(append(append([]fault{}, test.Failure...), test.Error...), test.Output)
	case test.Skipped != nil || test.Status == "notrun" || test.Status == "disabled":
		outcome.Status = StatusSkip
		outcome.Output = "not run"
		if test.Skipped != nil && test.Skipped.Message != "" {
			outcome.Output = test.Skipped.Message
		}
	}
	return outcome
}

// failureText is what a failed test said, trimmed to the first lines: the tail of
// a long log is in the report file for whoever wants it.
func failureText(failures []fault, output string) string {
	parts := []string{}
	for _, failure := range failures {
		text := strings.TrimSpace(failure.Text)
		if text == "" {
			text = failure.Message
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, strings.TrimSpace(output))
	}
	lines := strings.Split(strings.Join(parts, "\n"), "\n")
	if len(lines) > 6 {
		lines = append(lines[:6], "...")
	}
	return strings.Join(lines, "\n")
}

// Lines says outcomes in the form the run's parser reads.
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
			for _, line := range strings.Split(outcome.Output, "\n") {
				if line != "" {
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
