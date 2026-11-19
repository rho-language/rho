package rhotest

import "testing"

// A runner's own skipped cases are read as skipped, with the reason it gave, in
// each format that has a skip line.
func TestSkippedCasesAreRead(t *testing.T) {
	for line, want := range map[string][2]string{
		"[SKIP] e2e_power  (GATEWAY_BIN is not set)": {"e2e_power", "GATEWAY_BIN is not set"},
		"[SKIP] plain":                              {"plain", ""},
		"[  SKIPPED ] Suite.Case":                   {"Suite.Case", ""},
		"ok 3 - needs a board # SKIP no board here": {"needs a board", "no board here"},
	} {
		results := ParseCases(line + "\n")
		if len(results) != 1 || results[0].Status != StatusSkip || results[0].Case.Name != want[0] || results[0].Reason != want[1] {
			t.Errorf("%q: got %+v", line, results)
		}
	}
}

// A suite whose every case was skipped is skipped, not "ran but reported no
// cases": the cases were reported, and none of them ran.
func TestASuiteWhoseCasesAreAllSkippedIsSkipped(t *testing.T) {
	suite := Suite{Name: "e2e", Kind: "gtest", Steps: []Step{{Name: "run", Command: []string{"e2e"}}}}
	report := Execute([]Suite{suite}, ExecuteOptions{Runner: &fakeRunner{outputs: []string{"[SKIP] power  (no gateway)\n[SKIP] plug  (no gateway)\n"}, failing: -1}})
	if report.Summary.Failed != 0 || report.Summary.Skipped != 2 || report.Summary.FailedSuites != 0 {
		t.Fatalf("got %+v / %+v", report.Summary, report.Results)
	}
}
