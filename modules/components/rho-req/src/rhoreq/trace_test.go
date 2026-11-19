//
// @file trace_test.go
// @brief The trace, the test results it reads, and the rendered document.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestTagsAreFoundWithTheirSymbols(t *testing.T) {
	dir := fixture(t)
	w := load(t, filepath.Join(dir, "gateway"), false)
	got := []string{}
	for _, tag := range w.Focus.Tags() {
		test := ""
		if tag.Test != nil {
			test = " " + tag.Test.Kind + ":" + tag.Test.Name
		}
		got = append(got, tag.Kind+" "+strings.Join(tag.IDs, ",")+" "+tag.File+" "+tag.Symbol+test)
	}
	want := []string{
		"req GW-0001 src/session.rho restore",
		"req GW-0002,SYS-0002 src/session.rho enroll",
		"verifies GW-0001 tests/session_test.rho sessions_survive_restart case:sessions_survive_restart",
		"verifies GW-0002 tests/session_test.rho enroll_once case:enroll_once",
		"verifies GW-0099 tests/session_test.rho points_at_nothing case:points_at_nothing",
		"verifies GW-AUTH-0001 tests/smoke.py main suite:smoke",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSymbolsInOtherLanguages(t *testing.T) {
	cases := map[[2]string]string{
		{"func TestSessions(t *testing.T) {", "go"}:                        "TestSessions",
		{"func (s *Server) Restore() error {", "go"}:                       "Restore",
		{"static int test_restore(void)", "c"}:                             "test_restore",
		{"TEST(Sessions, SurviveRestart) {", "c"}:                          "Sessions.SurviveRestart",
		{"pub fn restore(x: i32) i32 {", "rho"}:                            "restore",
		{"def test_restart():", "py"}:                                      "test_restart",
		{"check_restart() {", "sh"}:                                        "check_restart",
		{"  - name: smoke   # the gateway smoke", "yml"}:                   "smoke",
		{"  - name: every manifest matches the toolchain's schema", "yml"}: "every manifest matches the toolchain's schema",
		{`  - name: "quoted: a suite"`, "yml"}:                             "quoted: a suite",
		{"  - name: 'it''s single-quoted'", "yml"}:                         "it's single-quoted",
	}
	for in, want := range cases {
		if got := symbolOf(in[0], in[1]); got != want {
			t.Errorf("symbolOf(%q, %s) = %q, want %q", in[0], in[1], got, want)
		}
	}
	if got := strings.Join(tagIDs(" GW-0001, GW-0002 SYS-0001 because it matters */"), " "); got != "GW-0001 GW-0002 SYS-0001" {
		t.Errorf("tagIDs = %q", got)
	}
}

func TestResultsFromEveryFormat(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.xml": `<?xml version="1.0"?><testsuites><testsuite name="x" package="tests/x_test.rho">` +
			`<testcase name="one" classname="tests/x_test.rho"/><testcase name="two" classname="tests/x_test.rho"><failure message="f"/></testcase>` +
			`<testcase name="three"><skipped/></testcase></testsuite></testsuites>`,
		"b.txt": "[PASS] tests/y: four\n[FAIL] five\n[SKIP] six  (not called)\nok 1 - seven\nnot ok 2 - eight\n    --- PASS: TestNine (0.00s)\n--- FAIL: TestTen (0.01s)\n",
	})
	results := ReadResults(dir, dir)
	for name, want := range map[string]string{
		"one": ResultPass, "two": ResultFail, "three": ResultSkip, "four": ResultPass, "five": ResultFail, "six": ResultSkip,
		"seven": ResultPass, "eight": ResultFail, "TestNine": ResultPass, "TestTen": ResultFail, "eleven": ResultNotRun,
	} {
		if got, _ := results.Lookup(&TestRef{Kind: "case", Name: name}, ""); got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
	if got, _ := results.Lookup(&TestRef{Kind: "suite", Name: "tests/y"}, ""); got != ResultPass {
		t.Errorf("suite tests/y = %s", got)
	}
	if got, _ := results.Lookup(&TestRef{Kind: "script", Name: "tests/z.sh"}, ""); got != ResultNotRun {
		t.Errorf("a script no suite runs = %s", got)
	}
}

func TestTheNewerReportWins(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"old.txt": "[FAIL] flaky\n", "new.txt": "[PASS] flaky\n"})
	now := time.Now()
	os.Chtimes(filepath.Join(dir, "old.txt"), now.Add(-time.Hour), now.Add(-time.Hour))
	os.Chtimes(filepath.Join(dir, "new.txt"), now, now)
	if got, report := ReadResults(dir, dir).Lookup(&TestRef{Kind: "case", Name: "flaky"}, ""); got != ResultPass || report != "new.txt" {
		t.Fatalf("got %s from %s", got, report)
	}
}

func TestTraceTheFixture(t *testing.T) {
	dir := fixture(t)
	matrix := Trace(load(t, filepath.Join(dir, "gateway"), true))
	evidence := map[string]string{}
	for _, row := range matrix.Rows {
		evidence[row.ID] = row.Evidence
	}
	want := map[string]string{
		"SYS-0001":     EvidencePassing, // through GW-0001, and GW-AUTH-0001 below it
		"SYS-0002":     EvidenceFailing, // through GW-0002
		"SYS-0003":     EvidenceNone,
		"GW-0001":      EvidencePassing,
		"GW-0002":      EvidenceFailing,
		"GW-0003":      EvidenceNone,
		"GW-AUTH-0001": EvidencePassing,
	}
	for id, state := range want {
		if evidence[id] != state {
			t.Errorf("%s evidence %s, want %s", id, evidence[id], state)
		}
	}
	gaps := []string{}
	for _, gap := range matrix.Gaps {
		gaps = append(gaps, gap.Code+" "+gap.ID)
	}
	wantGaps := "orphan-adr ADR-0002,verified-unproven GW-0002,untested GW-0003,dangling-tag GW-0099,verified-unproven SYS-0002"
	if got := strings.Join(gaps, ","); got != wantGaps {
		t.Fatalf("gaps:\n%s\nwant\n%s", got, wantGaps)
	}
	if matrix.Blocking() != 4 {
		t.Fatalf("blocking %d", matrix.Blocking())
	}

	// No reports at all: every test is not run.
	os.RemoveAll(filepath.Join(dir, "gateway", ReportsDir))
	matrix = Trace(load(t, filepath.Join(dir, "gateway"), false))
	if row := matrix.Row("GW-0001"); row.Evidence != EvidenceNotRun || row.Tests[0].Result != ResultNotRun {
		t.Fatalf("GW-0001 without reports: %+v", row)
	}
}

func TestRenderGolden(t *testing.T) {
	dir := fixture(t)
	w := load(t, filepath.Join(dir, "gateway"), true)
	got := Render(w, Trace(w), "")
	golden := filepath.Join("testdata", "golden", "render.md")
	if *update {
		os.MkdirAll(filepath.Dir(golden), 0755)
		if err := os.WriteFile(golden, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if want := read(t, golden); got != want {
		t.Fatalf("render differs from %s (go test -run TestRenderGolden -update rewrites it):\n%s", golden, got)
	}
	one := Render(w, Trace(w), "SUB")
	if strings.Contains(one, "## Node (NODE)") || !strings.Contains(one, "## Subsystem (SUB)") {
		t.Fatal("--level limits the level sections")
	}
}
