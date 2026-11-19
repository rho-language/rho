//
// @file results_test.go
// @brief Tests for the run's own record, results.json.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhotest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func readResults(t *testing.T, path string) ResultsFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected %s: %v", path, err)
	}
	var record ResultsFile
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("%s is not JSON: %v\n%s", path, err, data)
	}
	return record
}

func suiteNamed(record ResultsFile, name string) *SuiteRecord {
	for index := range record.Suites {
		if record.Suites[index].Name == name {
			return &record.Suites[index]
		}
	}
	return nil
}

// Every run writes its record, with every case and its status, whether or not a
// report plugin is enabled and whatever the person asked to see; stdout stays the
// text it was.
func TestEveryRunWritesResultsJSON(t *testing.T) {
	root := project(t, commandManifest)
	var out bytes.Buffer
	report := RunSuite(Options{Root: root, Path: root, Runner: &fakeRunner{failing: 1}, Out: &out, RhoBinary: "/bin/false"})

	record := readResults(t, filepath.Join(root, "build", "test-reports", ResultsFileName))
	if record.Schema != ResultsSchema || record.Runner != "rho-test" || record.RunnerVersion != RunnerVersion {
		t.Errorf("unexpected header: %+v", record)
	}
	if record.Rho != "unknown" {
		t.Errorf("a rho that cannot be asked is recorded as unknown, got %q", record.Rho)
	}
	if _, err := time.Parse(time.RFC3339, record.Run); err != nil {
		t.Errorf("run time is not RFC 3339: %q", record.Run)
	}
	if record.Project != "p" || record.Target != "" && record.Target != DefaultTarget {
		t.Errorf("unexpected project/target: %q %q", record.Project, record.Target)
	}
	if record.Summary.Suites != report.Summary.Suites || record.Summary.Failed != report.Summary.Failed {
		t.Errorf("summary %+v does not match the run %+v", record.Summary, report.Summary)
	}
	pass, fail, later := suiteNamed(record, "pass"), suiteNamed(record, "fail"), suiteNamed(record, "later")
	if pass == nil || fail == nil || later == nil {
		t.Fatalf("expected the three suites, got %+v", record.Suites)
	}
	// A command suite's verdict is its exit status, and its one case is named for
	// the suite: that is the name rho req maps a script's @verifies to.
	if pass.Kind != "command" || pass.Status != "pass" || len(pass.Cases) != 1 || pass.Cases[0].Name != "pass" || pass.Cases[0].Status != "pass" {
		t.Errorf("unexpected passing suite: %+v", pass)
	}
	if fail.Status != "fail" || fail.Cases[0].Status != "fail" {
		t.Errorf("unexpected failing suite: %+v", fail)
	}
	if later.Status != "skip" || !strings.Contains(later.Reason, "no board") || later.Cases[0].Message == "" {
		t.Errorf("a skipped suite says why: %+v", later)
	}
	if strings.Contains(out.String(), ResultsFileName) {
		t.Errorf("the record is written quietly; stdout is unchanged:\n%s", out.String())
	}
}

// A Rho test file's cases are its `test fn`s, by name, with the file relative to
// the project and the line.
func TestResultsNameATestFnCaseWithItsPlace(t *testing.T) {
	root := project(t, "name: p\nsuites:\n  - name: tests/session\n    kind: rho\n")
	if err := os.WriteFile(filepath.Join(root, "tests", "session_test.rho"), []byte("// @verifies GW-0001\ntest fn sessions_survive_restart() {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	RunSuite(Options{Root: root, Path: root, Runner: &fakeRunner{failing: -1}, Out: discard(), RhoBinary: "/bin/false"})
	record := readResults(t, filepath.Join(root, "build", "test-reports", ResultsFileName))
	suite := suiteNamed(record, "tests/session")
	if suite == nil || suite.Kind != "rho" || len(suite.Cases) != 1 {
		t.Fatalf("expected the rho suite with one case, got %+v", record.Suites)
	}
	c := suite.Cases[0]
	if c.Name != "sessions_survive_restart" || c.Status != "pass" || c.File != "tests/session_test.rho" || c.Line != 2 {
		t.Errorf("unexpected case: %+v", c)
	}
}

// --report-dir moves the record with the reports, and a second run replaces it.
func TestResultsFollowTheReportDirAndAreReplaced(t *testing.T) {
	root := project(t, commandManifest)
	dir := filepath.Join(t.TempDir(), "reports")
	RunSuite(Options{Root: root, Path: root, Runner: &fakeRunner{failing: -1}, Out: discard(), ReportDir: dir, RhoBinary: "/bin/false"})
	RunSuite(Options{Root: root, Path: root, Selection: Selection{Names: []string{"pass"}}, Runner: &fakeRunner{failing: -1}, Out: discard(), ReportDir: dir, RhoBinary: "/bin/false"})
	record := readResults(t, filepath.Join(dir, ResultsFileName))
	if len(record.Suites) != 1 || record.Suites[0].Name != "pass" {
		t.Errorf("expected the second run's one suite, got %+v", record.Suites)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected only results.json, got %v", entries)
	}
}

// A list or a dry run ran nothing and leaves no record.
func TestAListOrDryRunWritesNoResults(t *testing.T) {
	root := project(t, commandManifest)
	RunSuite(Options{Root: root, Path: root, List: true, Out: discard()})
	RunSuite(Options{Root: root, Path: root, DryRun: true, Out: discard()})
	if _, err := os.Stat(filepath.Join(root, "build", "test-reports", ResultsFileName)); err == nil {
		t.Error("expected no results.json")
	}
}

// The version recorded is the module's.
func TestRunnerVersionIsTheManifests(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "module.yml"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^  version:\s*(\S+)`).FindSubmatch(data)
	if match == nil || string(match[1]) != RunnerVersion {
		t.Errorf("RunnerVersion %s does not match module.yml %s", RunnerVersion, match)
	}
}
