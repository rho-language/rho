//
// @file results.go
// @brief The run's own record of what it found, written to a file every run.
//
// What a run prints is for a person, and says nothing about a case that passed
// unless --verbose was given. A tool that asks what the last run found — `rho req
// trace`, which maps each requirement to the tests that verify it — needs every
// case, with its status, whatever the person asked to see. So every run that ran
// something writes `results.json` beside the plugins' reports, in
// `build/test-reports/` (or --report-dir), and overwrites the one before it:
//
//	{
//	  "schema": 1,
//	  "runner": "rho-test", "runner_version": "0.3.0",
//	  "rho": "0.19.1",
//	  "run": "2026-11-19T18:30:00Z", "seconds": 4.2,
//	  "project": "gateway", "target": "c",
//	  "summary": {"suites": 2, "cases": 5, "passed": 4, "failed": 1, "skipped": 0},
//	  "suites": [
//	    {"name": "session", "kind": "rho", "status": "fail", "seconds": 1.3,
//	     "cases": [{"name": "sessions_survive_restart", "status": "pass",
//	                "file": "tests/session_test.rho", "line": 12}, ...]},
//	    {"name": "smoke", "kind": "command", "status": "pass", "seconds": 2.9,
//	     "cases": [{"name": "smoke", "status": "pass"}]}
//	  ]
//	}
//
// A case is named as the run names it: a Rho `test fn` by its name, a suite whose
// verdict is its exit status (a `command` suite) by the suite's own name. A
// suite expanded from a matrix is named `base!axis=value`.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhotest

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunnerVersion is this runner's version, recorded in every results file. The
// test beside this file checks it against the module's manifest.
const RunnerVersion = "0.4.0"

// ResultsFileName is the name of the run's own record in the report directory.
const ResultsFileName = "results.json"

// ResultsSchema is the version of the results file's shape. A reader that finds
// a higher one reads what it knows and ignores the rest.
const ResultsSchema = 1

// ResultsFile is the run's own record.
type ResultsFile struct {
	Schema        int            `json:"schema"`
	Runner        string         `json:"runner"`
	RunnerVersion string         `json:"runner_version"`
	Rho           string         `json:"rho"`
	Run           string         `json:"run"`
	Seconds       float64        `json:"seconds"`
	Project       string         `json:"project"`
	Target        string         `json:"target"`
	Summary       ResultsSummary `json:"summary"`
	Suites        []SuiteRecord  `json:"suites"`
	Diagnostics   []string       `json:"diagnostics,omitempty"`
}

// ResultsSummary is the run's counts.
type ResultsSummary struct {
	Suites  int `json:"suites"`
	Cases   int `json:"cases"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// SuiteRecord is one suite of the run.
type SuiteRecord struct {
	Name string   `json:"name"`
	Kind string   `json:"kind"`
	Tags []string `json:"tags,omitempty"`
	// Status is fail when any case failed, skip when the suite was not run or
	// every case was skipped, and pass otherwise.
	Status  string       `json:"status"`
	Seconds float64      `json:"seconds"`
	Reason  string       `json:"reason,omitempty"`
	Cases   []CaseRecord `json:"cases"`
}

// CaseRecord is one case of a suite.
type CaseRecord struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	// Message is why a case failed or was skipped: the reason, or the end of what
	// it printed.
	Message string `json:"message,omitempty"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

// BuildResultsFile is the record of a finished run.
func BuildResultsFile(report Report, root, target, rhoVersion string, at time.Time) ResultsFile {
	file := ResultsFile{
		Schema:        ResultsSchema,
		Runner:        "rho-test",
		RunnerVersion: RunnerVersion,
		Rho:           rhoVersion,
		Run:           at.UTC().Format(time.RFC3339),
		Seconds:       round(report.Seconds),
		Project:       nameOr(report.Manifest.Name, filepath.Base(root)),
		Target:        target,
		Summary: ResultsSummary{
			Suites: report.Summary.Suites, Cases: report.Summary.Cases,
			Passed: report.Summary.Passed, Failed: report.Summary.Failed, Skipped: report.Summary.Skipped,
		},
		Suites:      []SuiteRecord{},
		Diagnostics: report.Diagnostics,
	}
	index := map[string]int{}
	add := func(name, kind string, tags []string) int {
		if at, ok := index[name]; ok {
			return at
		}
		index[name] = len(file.Suites)
		file.Suites = append(file.Suites, SuiteRecord{Name: name, Kind: kind, Tags: SortedTags(tags), Cases: []CaseRecord{},
			Seconds: round(report.SuiteSeconds[name])})
		return index[name]
	}
	for _, suite := range report.Suites {
		at := add(suite.Name, suite.Kind, suite.Tags)
		file.Suites[at].Reason = firstNonEmpty(suite.Skip, suite.Broken)
	}
	for _, result := range report.Results {
		at := add(nameOr(result.Suite, result.Case.Name), "", nil)
		record := CaseRecord{Name: result.Case.Name, Status: string(result.Status), File: relativeTo(root, result.Case.File), Line: result.Case.Line}
		switch result.Status {
		case StatusFail:
			record.Message = firstNonEmpty(result.Reason, lastLines(result.Output, 20))
		case StatusSkip:
			record.Message = result.Reason
		}
		file.Suites[at].Cases = append(file.Suites[at].Cases, record)
	}
	for at := range file.Suites {
		file.Suites[at].Status = suiteStatus(file.Suites[at])
	}
	return file
}

// suiteStatus is what a suite's cases come to.
func suiteStatus(suite SuiteRecord) string {
	if len(suite.Cases) == 0 {
		return string(StatusSkip)
	}
	status := string(StatusSkip)
	for _, record := range suite.Cases {
		switch record.Status {
		case string(StatusFail):
			return string(StatusFail)
		case string(StatusPass):
			status = string(StatusPass)
		}
	}
	return status
}

// writeResultsFile writes the run's record where the reports go. A failure to
// write it is said on standard error and does not change the run's verdict.
func writeResultsFile(options Options, root string, report Report) {
	dir := options.ReportDir
	if dir == "" {
		dir = filepath.Join(root, "build", "test-reports")
	}
	record := BuildResultsFile(report, root, options.Target, toolchainVersion(options.RhoBinary), time.Now())
	if err := WriteResultsFile(dir, record); err != nil {
		fmt.Fprintf(os.Stderr, "rho test: cannot write %s: %v\n", filepath.Join(dir, ResultsFileName), err)
	}
}

// WriteResultsFile writes a record to dir/results.json, replacing the one there
// in one step so a reader never sees half of it.
func WriteResultsFile(dir string, record ResultsFile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".results-*.json")
	if err != nil {
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		os.Remove(temporary.Name())
		return err
	}
	if err := temporary.Close(); err != nil {
		os.Remove(temporary.Name())
		return err
	}
	if err := os.Chmod(temporary.Name(), 0o644); err != nil {
		os.Remove(temporary.Name())
		return err
	}
	return os.Rename(temporary.Name(), filepath.Join(dir, ResultsFileName))
}

// toolchainVersion is what `rho --version` says, without the program's name, or
// "unknown" when it cannot be asked.
func toolchainVersion(binary string) string {
	if binary == "" {
		binary = rhoBinary()
	}
	out, err := exec.Command(binary, "--version").Output()
	if err != nil {
		return "unknown"
	}
	text := strings.TrimSpace(string(out))
	text = strings.TrimSpace(strings.TrimPrefix(text, "rho"))
	if text == "" {
		return "unknown"
	}
	return strings.Fields(text)[0]
}

// relativeTo is a path relative to the project, with forward slashes, when it is
// inside it; otherwise as given.
func relativeTo(root, path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return filepath.ToSlash(path)
		}
		path = absolute
	}
	if absRoot, err := filepath.Abs(root); err == nil {
		root = absRoot
	}
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// lastLines is the end of a text, which is where a failure says why.
func lastLines(text string, limit int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func round(seconds float64) float64 {
	return float64(int64(seconds*1000+0.5)) / 1000
}
