//
// @file results.go
// @brief The latest result of each test, from the reports a run left behind.
//
// `rho test` (rho-test 0.3.0 and later) writes its own record of every run to
// `build/test-reports/results.json`: every suite and case with its status. That
// file is read first and is authoritative for the suites it names — a test is
// looked up there before anywhere else.
//
// The other files there are what report plugins produce (report-1.txt,
// report-2.txt, ... one per plugin) or what a person saved, and they are the
// fallback for a test the record does not name. Each is recognised by what it
// holds rather than by its name:
//
//   - JUnit XML (rho-test-junit's report, or any other JUnit producer):
//     <testcase name=... classname=...> with <failure>, <error> or <skipped>
//   - the runner's own text: `[PASS] suite: case`, `[FAIL] case`, `[SKIP] case`
//     (passes are printed only with `rho test --verbose`)
//   - TAP: `ok 3 - name`, `not ok 4 - name`
//   - `go test -v`: `--- PASS: TestX`, `--- FAIL: TestX`, `--- SKIP: TestX`
//
// When two reports name the same case, the newer file wins.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ReportsDir is where a repository's test reports are read from.
const ReportsDir = "build/test-reports"

// Result statuses.
const (
	ResultPass   = "pass"
	ResultFail   = "fail"
	ResultSkip   = "skip"
	ResultNotRun = "not run"
)

// ResultsFile is the name of rho test's own record of its latest run.
const ResultsFile = "results.json"

// CaseResult is one case as a report gives it.
type CaseResult struct {
	Suite  string
	Name   string
	File   string
	Line   int
	Status string
	Report string
	// Runner marks a case from rho test's own record, which is authoritative.
	Runner bool
	when   time.Time
}

// Results are the cases of every report in a repository.
type Results struct {
	Cases []CaseResult
}

// Results reads a repository's reports, once.
func (r *Repo) Results() *Results {
	if r.results == nil {
		r.results = ReadResults(filepath.Join(r.Dir, ReportsDir), r.Dir)
	}
	return r.results
}

// ReadResults reads every report in a directory.
func ReadResults(dir, repoDir string) *Results {
	out := &Results{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue // a record being written
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		info, _ := entry.Info()
		when := time.Time{}
		if info != nil {
			when = info.ModTime()
		}
		rel, _ := filepath.Rel(repoDir, path)
		rel = filepath.ToSlash(rel)
		cases := []CaseResult{}
		if entry.Name() == ResultsFile {
			cases = parseRunnerRecord(data)
		} else {
			cases = parseReport(string(data))
		}
		for _, c := range cases {
			c.Report, c.when = rel, when
			out.Cases = append(out.Cases, c)
		}
	}
	// Oldest first, so a later entry is the one that counts.
	sort.SliceStable(out.Cases, func(a, b int) bool {
		if !out.Cases[a].when.Equal(out.Cases[b].when) {
			return out.Cases[a].when.Before(out.Cases[b].when)
		}
		return out.Cases[a].Report < out.Cases[b].Report
	})
	return out
}

// runnerRecord is the part of rho test's results.json a trace reads.
type runnerRecord struct {
	Schema int    `json:"schema"`
	Runner string `json:"runner"`
	Run    string `json:"run"`
	Suites []struct {
		Name  string `json:"name"`
		Kind  string `json:"kind"`
		Cases []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			File   string `json:"file"`
			Line   int    `json:"line"`
		} `json:"cases"`
	} `json:"suites"`
}

// parseRunnerRecord reads rho test's own record. A file that is not one is read
// as nothing rather than guessed at.
func parseRunnerRecord(data []byte) []CaseResult {
	var record runnerRecord
	if err := json.Unmarshal(data, &record); err != nil || record.Runner != "rho-test" || record.Schema < 1 {
		return nil
	}
	out := []CaseResult{}
	for _, suite := range record.Suites {
		for _, c := range suite.Cases {
			status := ResultSkip
			switch c.Status {
			case "pass":
				status = ResultPass
			case "fail":
				status = ResultFail
			}
			out = append(out, CaseResult{Suite: suite.Name, Name: c.Name, File: c.File, Line: c.Line, Status: status, Runner: true})
		}
	}
	return out
}

// RunTime is when the run recorded in a repository's results.json happened, as
// the record says (RFC 3339), or "" when there is no record.
func RunTime(repoDir string) string {
	data, err := os.ReadFile(filepath.Join(repoDir, ReportsDir, ResultsFile))
	if err != nil {
		return ""
	}
	var record runnerRecord
	if json.Unmarshal(data, &record) != nil || record.Runner != "rho-test" {
		return ""
	}
	return record.Run
}

func parseReport(text string) []CaseResult {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "<?xml") || strings.HasPrefix(trimmed, "<testsuite") {
		return parseJUnit(trimmed)
	}
	return parseText(text)
}

type junitCase struct {
	Name    string    `xml:"name,attr"`
	Class   string    `xml:"classname,attr"`
	File    string    `xml:"file,attr"`
	Failure *struct{} `xml:"failure"`
	Error   *struct{} `xml:"error"`
	Skipped *struct{} `xml:"skipped"`
}

type junitSuite struct {
	Name    string       `xml:"name,attr"`
	Package string       `xml:"package,attr"`
	Cases   []junitCase  `xml:"testcase"`
	Suites  []junitSuite `xml:"testsuite"`
}

func parseJUnit(text string) []CaseResult {
	var doc struct {
		XMLName xml.Name
		Name    string       `xml:"name,attr"`
		Package string       `xml:"package,attr"`
		Cases   []junitCase  `xml:"testcase"`
		Suites  []junitSuite `xml:"testsuite"`
	}
	if err := xml.Unmarshal([]byte(text), &doc); err != nil {
		return nil
	}
	out := []CaseResult{}
	var walk func(s junitSuite)
	walk = func(s junitSuite) {
		for _, c := range s.Cases {
			status := ResultPass
			switch {
			case c.Failure != nil || c.Error != nil:
				status = ResultFail
			case c.Skipped != nil:
				status = ResultSkip
			}
			file := c.File
			if file == "" {
				file = c.Class
			}
			if file == "" {
				file = s.Package
			}
			out = append(out, CaseResult{Suite: s.Name, Name: c.Name, File: file, Status: status})
		}
		for _, inner := range s.Suites {
			walk(inner)
		}
	}
	walk(junitSuite{Name: doc.Name, Package: doc.Package, Cases: doc.Cases, Suites: doc.Suites})
	return out
}

var (
	textResult = regexp.MustCompile(`^\[(PASS|FAIL|SKIP)\]\s+(.+?)(?:\s{2,}\(.*\))?\s*$`)
	tapResult  = regexp.MustCompile(`^(not ok|ok)\s+\d*\s*-?\s*(.+?)(?:\s+#\s*(SKIP|skip|TODO).*)?$`)
	goResult   = regexp.MustCompile(`^\s*--- (PASS|FAIL|SKIP): (\S+)`)
)

func parseText(text string) []CaseResult {
	out := []CaseResult{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := textResult.FindStringSubmatch(line); m != nil {
			label := m[2]
			suiteName, name := "", label
			if before, after, ok := strings.Cut(label, ": "); ok {
				suiteName, name = before, after
			}
			out = append(out, CaseResult{Suite: suiteName, Name: name, Status: textStatus(m[1])})
			continue
		}
		if m := goResult.FindStringSubmatch(line); m != nil {
			out = append(out, CaseResult{Name: m[2], Status: textStatus(m[1])})
			continue
		}
		if m := tapResult.FindStringSubmatch(line); m != nil {
			status := ResultPass
			if m[1] == "not ok" {
				status = ResultFail
			}
			if m[3] != "" {
				status = ResultSkip
			}
			out = append(out, CaseResult{Name: strings.TrimSpace(m[2]), Status: status})
		}
	}
	return out
}

func textStatus(word string) string {
	switch word {
	case "PASS":
		return ResultPass
	case "FAIL":
		return ResultFail
	}
	return ResultSkip
}

// Lookup is the latest result of a test, and the report it came from. rho test's
// own record is asked first and decides for a test it names; the other reports
// are asked only for a test it does not. A case is matched by name, preferring a
// report entry whose file is the tag's file; a suite by a case of that name (a
// command suite's single case) or by every case of a suite of that name — or of
// a matrix expanded from it, `name!axis=value` — failing if any of them failed.
func (r *Results) Lookup(ref *TestRef, file string) (status, report string) {
	if ref == nil || ref.Kind == "script" {
		return ResultNotRun, ""
	}
	if status, report := r.lookup(ref, file, true); status != ResultNotRun {
		return status, report
	}
	return r.lookup(ref, file, false)
}

func (r *Results) lookup(ref *TestRef, file string, runner bool) (status, report string) {
	status = ResultNotRun
	if ref.Kind == "case" {
		var best, fileMatch *CaseResult
		for index := range r.Cases {
			c := &r.Cases[index]
			if c.Runner != runner {
				continue
			}
			if c.Name != ref.Name && !strings.HasSuffix(c.Name, "."+ref.Name) && !strings.HasSuffix(c.Name, "/"+ref.Name) {
				continue
			}
			best = c
			if c.File != "" && (c.File == file || strings.HasSuffix(file, c.File) || strings.HasSuffix(c.File, file)) {
				fileMatch = c
			}
		}
		if fileMatch != nil {
			best = fileMatch
		}
		if best == nil {
			return ResultNotRun, ""
		}
		return best.Status, best.Report
	}
	// A suite: its latest report decides.
	names := func(c *CaseResult) bool {
		return c.Runner == runner && (c.Name == ref.Name || c.Suite == ref.Name || strings.HasPrefix(c.Suite, ref.Name+"!"))
	}
	latest := ""
	for index := range r.Cases {
		if c := &r.Cases[index]; names(c) {
			latest = c.Report
		}
	}
	if latest == "" {
		return ResultNotRun, ""
	}
	status = ResultSkip
	for index := range r.Cases {
		c := &r.Cases[index]
		if c.Report != latest || !names(c) {
			continue
		}
		switch {
		case c.Status == ResultFail:
			return ResultFail, latest
		case c.Status == ResultPass:
			status = ResultPass
		}
	}
	return status, latest
}
