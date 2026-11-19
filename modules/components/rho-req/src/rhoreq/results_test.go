//
// @file results_test.go
// @brief rho test's own record, results.json, is read first and decides.
//
// @project Rho
// @version 0.2
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A record as rho-test 0.3.0 writes it: a rho suite of `test fn` cases, a command
// suite with one case named for it, and a matrix expanded from one declaration.
const runnerRecordFixture = `{
  "schema": 1, "runner": "rho-test", "runner_version": "0.3.0", "rho": "0.19.1",
  "run": "2026-11-19T18:30:00Z", "seconds": 1.5, "project": "gateway", "target": "c",
  "summary": {"suites": 4, "cases": 6, "passed": 4, "failed": 1, "skipped": 1},
  "suites": [
    {"name": "tests/session", "kind": "rho", "status": "fail", "seconds": 0.5, "cases": [
      {"name": "sessions_survive_restart", "status": "pass", "file": "tests/session_test.rho", "line": 4},
      {"name": "enroll_once", "status": "fail", "message": "expected 1", "file": "tests/session_test.rho", "line": 8}]},
    {"name": "smoke", "kind": "command", "status": "pass", "seconds": 0.9, "cases": [{"name": "smoke", "status": "pass"}]},
    {"name": "boards!board=pico", "kind": "smoke", "status": "pass", "cases": [{"name": "boards!board=pico", "status": "pass"}]},
    {"name": "boards!board=uno", "kind": "smoke", "status": "skip", "cases": [{"name": "boards!board=uno", "status": "skip", "message": "no toolchain"}]}
  ]
}
`

func TestTheRunnersRecordIsRead(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{ResultsFile: runnerRecordFixture})
	results := ReadResults(dir, dir)
	for _, check := range []struct {
		ref  TestRef
		file string
		want string
	}{
		{TestRef{Kind: "case", Name: "sessions_survive_restart"}, "tests/session_test.rho", ResultPass},
		{TestRef{Kind: "case", Name: "enroll_once"}, "tests/session_test.rho", ResultFail},
		// A script's @verifies is the suite that runs it, and a command suite's
		// one case carries the suite's name.
		{TestRef{Kind: "suite", Name: "smoke"}, "tests/smoke.py", ResultPass},
		// A matrix is every cell of it: one passed and one skipped is a pass.
		{TestRef{Kind: "suite", Name: "boards"}, "tests/tests.yml", ResultPass},
		{TestRef{Kind: "suite", Name: "tests/session"}, "tests/tests.yml", ResultFail},
		{TestRef{Kind: "case", Name: "absent"}, "", ResultNotRun},
	} {
		got, report := results.Lookup(&check.ref, check.file)
		if got != check.want {
			t.Errorf("%s %s = %s, want %s", check.ref.Kind, check.ref.Name, got, check.want)
		}
		if got != ResultNotRun && report != ResultsFile {
			t.Errorf("%s came from %q", check.ref.Name, report)
		}
	}
	if got := RunTime(filepath.Dir(filepath.Dir(dir))); got != "" {
		t.Errorf("no record there, got %q", got)
	}
}

// The record decides for every test it names, even when another report is newer;
// a test it does not name falls back to the other reports.
func TestTheRunnersRecordIsAuthoritative(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		ResultsFile:  runnerRecordFixture,
		"report.txt": "[PASS] enroll_once\n[FAIL] smoke\n[PASS] only_here\n",
	})
	now := time.Now()
	os.Chtimes(filepath.Join(dir, ResultsFile), now.Add(-time.Hour), now.Add(-time.Hour))
	os.Chtimes(filepath.Join(dir, "report.txt"), now, now)
	results := ReadResults(dir, dir)
	if got, _ := results.Lookup(&TestRef{Kind: "case", Name: "enroll_once"}, ""); got != ResultFail {
		t.Errorf("enroll_once = %s, want the record's fail", got)
	}
	if got, _ := results.Lookup(&TestRef{Kind: "suite", Name: "smoke"}, ""); got != ResultPass {
		t.Errorf("smoke = %s, want the record's pass", got)
	}
	if got, report := results.Lookup(&TestRef{Kind: "case", Name: "only_here"}, ""); got != ResultPass || report != "report.txt" {
		t.Errorf("only_here = %s from %s, want the fallback", got, report)
	}
}

// A JSON file that is not rho test's record is not read as one.
func TestAnotherJSONIsNotARecord(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{ResultsFile: `{"runner":"other","suites":[{"name":"x","cases":[{"name":"x","status":"pass"}]}]}`})
	if got, _ := ReadResults(dir, dir).Lookup(&TestRef{Kind: "suite", Name: "x"}, ""); got != ResultNotRun {
		t.Errorf("x = %s", got)
	}
}

// The fixture, traced from a record alone: the same evidence the JUnit and text
// reports gave.
func TestTraceFromTheRecordAlone(t *testing.T) {
	dir := fixture(t)
	reports := filepath.Join(dir, "gateway", ReportsDir)
	os.RemoveAll(reports)
	writeFiles(t, reports, map[string]string{ResultsFile: runnerRecordFixture})
	matrix := Trace(load(t, filepath.Join(dir, "gateway"), true))
	for id, want := range map[string]string{"GW-0001": EvidencePassing, "GW-0002": EvidenceFailing, "GW-AUTH-0001": EvidencePassing, "SYS-0001": EvidencePassing} {
		if row := matrix.Row(id); row == nil || row.Evidence != want {
			t.Errorf("%s evidence %+v, want %s", id, row, want)
		}
	}
	if got := RunTime(filepath.Join(dir, "gateway")); got != "2026-11-19T18:30:00Z" {
		t.Errorf("run time %q", got)
	}
}
