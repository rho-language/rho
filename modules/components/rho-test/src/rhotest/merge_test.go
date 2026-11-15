package rhotest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func declaredSuite(names ...string) Suite {
	suite := Suite{Name: "tests/csv", Kind: KindRho}
	for _, name := range names {
		suite.Cases = append(suite.Cases, Case{Name: name})
	}
	return suite
}

func statusOf(results []Result) map[string]Status {
	out := map[string]Status{}
	for _, result := range results {
		out[result.Case.Name] = result.Status
	}
	return out
}

// A printed [FAIL] line reports the declared case of that name; it does not
// replace the list of cases.
func TestAPrintedFailReportsItsCaseAndKeepsTheRest(t *testing.T) {
	results, suiteFailed := mergeDeclaredCases(declaredSuite("a", "b", "c"), "[PASS] a\n[FAIL] b\n[PASS] c\n", false)
	got := statusOf(results)
	if len(results) != 3 || got["a"] != StatusPass || got["b"] != StatusFail || got["c"] != StatusPass || suiteFailed {
		t.Fatalf("got %+v suiteFailed=%v", results, suiteFailed)
	}
	for _, result := range results {
		if result.Suite != "tests/csv" {
			t.Fatalf("a result does not name its suite: %+v", result)
		}
	}
}

// A name that is not a declared case does not become one; a failure under such
// a name still fails the suite.
func TestAnUnknownNameNeverBecomesACase(t *testing.T) {
	results, suiteFailed := mergeDeclaredCases(declaredSuite("a"), "[PASS] a\n[PASS] extra\n", false)
	if len(results) != 1 || suiteFailed {
		t.Fatalf("an unknown [PASS] changed the cases: %+v %v", results, suiteFailed)
	}
	results, suiteFailed = mergeDeclaredCases(declaredSuite("a"), "[PASS] a\n[FAIL] extra\n", false)
	if len(results) != 1 || results[0].Status != StatusPass || !suiteFailed {
		t.Fatalf("an unknown [FAIL] was dropped: %+v %v", results, suiteFailed)
	}
}

// A program that stopped fails the cases it never reported, and a program that
// failed after reporting every case a pass fails the suite.
func TestAStoppedProgramFailsWhatItDidNotReport(t *testing.T) {
	results, suiteFailed := mergeDeclaredCases(declaredSuite("a", "b"), "[PASS] a\nASSERTION FAILED", true)
	got := statusOf(results)
	if got["a"] != StatusPass || got["b"] != StatusFail || suiteFailed {
		t.Fatalf("got %+v suiteFailed=%v", results, suiteFailed)
	}
	if !strings.Contains(results[1].Reason, "before this case reported") {
		t.Errorf("the reason does not say why: %q", results[1].Reason)
	}
	_, suiteFailed = mergeDeclaredCases(declaredSuite("a"), "[PASS] a\n", true)
	if !suiteFailed {
		t.Fatal("a failing program whose cases all passed did not fail its suite")
	}
}

// A program that reports its cases and says nothing of one has not shown that
// it ran — a test fn main never calls — so that case is skipped, with the reason;
// a program that reports none is judged by its exit status.
func TestAnUnreportedCaseOfAReportingProgramIsSkipped(t *testing.T) {
	results, suiteFailed := mergeDeclaredCases(declaredSuite("a", "b"), "[PASS] a\n", false)
	got := statusOf(results)
	if got["a"] != StatusPass || got["b"] != StatusSkip || suiteFailed {
		t.Fatalf("got %+v suiteFailed=%v", results, suiteFailed)
	}
	results, _ = mergeDeclaredCases(declaredSuite("a", "b"), "hello\n", false)
	got = statusOf(results)
	if got["a"] != StatusPass || got["b"] != StatusPass {
		t.Fatalf("a silent program that exited 0 did not pass its cases: %+v", results)
	}
}

// A failure wins over a pass for the same name, whichever was printed last.
func TestAFailureWinsOverAPass(t *testing.T) {
	results, _ := mergeDeclaredCases(declaredSuite("a"), "[FAIL] a\n[PASS] a\n", false)
	if results[0].Status != StatusFail {
		t.Fatalf("got %+v", results)
	}
}

// The whole run: a failing case fails the run, and its suite counts as failed.
func TestARhoSuiteWithAFailingCaseFailsTheRun(t *testing.T) {
	suite := declaredSuite("a", "b")
	suite.Steps = []Step{{Name: "build and run", Action: func(Suite) (string, bool, error) { return "", false, nil }}}
	report := Execute([]Suite{suite}, ExecuteOptions{Runner: &fakeRunner{outputs: []string{"[PASS] a\n[FAIL] b\n"}, failing: 0}})
	if report.Summary.Failed != 1 || report.Summary.Passed != 1 || report.Summary.FailedSuites != 1 {
		t.Fatalf("got %+v", report.Summary)
	}
}

// A suite of another kind that names cases and reports none has tested nothing.
func TestAPluginSuiteThatReportsNoCasesFails(t *testing.T) {
	suite := Suite{Name: "unit", Kind: "gtest", Steps: []Step{{Name: "run", Command: []string{"unit"}}}}
	report := Execute([]Suite{suite}, ExecuteOptions{Runner: &fakeRunner{failing: -1}})
	if report.Summary.Failed != 1 || !strings.Contains(report.Results[0].Reason, "no cases") {
		t.Fatalf("got %+v", report.Results)
	}
}

// TAP's number, dash and directive are not part of a case's name.
func TestTapNames(t *testing.T) {
	cases := map[string]string{
		"ok 3 - parses quotes":       "parses quotes",
		"ok 3 parses quotes":         "parses quotes",
		"not ok 4 - empty # TODO no": "empty",
		"ok 5":                       "5",
		"ok - unnumbered":            "unnumbered",
	}
	for line, want := range cases {
		results := ParseCases(line + "\n")
		if len(results) != 1 || results[0].Case.Name != want {
			t.Errorf("%q: got %+v, want %q", line, results, want)
		}
	}
}

// --echo does not print the result lines the report prints again.
func TestEchoLeavesOutResultLines(t *testing.T) {
	got := withoutResultLines("starting\n[PASS] a\nok 2 - b\n  [FAIL] c\nend\n")
	if got != "starting\nend\n" {
		t.Fatalf("got %q", got)
	}
}

// A test file named on the command line runs that file's suite and no other.
func TestANamedTestFileRunsOnlyItsSuite(t *testing.T) {
	root := project(t, `name: p
suites:
  - name: tests/a   # the first
    kind: rho
  - name: tests/b
    kind: rho
`)
	for _, name := range []string{"a", "b"} {
		writeTest(t, filepath.Join(root, "tests", name+"_test.rho"), "test fn one() {\n}\n")
	}
	report := RunSuite(Options{
		Root:   root,
		Path:   filepath.Join(root, "tests", "b_test.rho"),
		Runner: &fakeRunner{failing: -1},
		Out:    discard(),
	})
	if len(report.Suites) != 1 || report.Suites[0].Name != "tests/b" {
		t.Fatalf("got %+v", report.Suites)
	}
}

func writeTest(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
