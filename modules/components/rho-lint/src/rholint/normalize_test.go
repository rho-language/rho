//
// @file normalize_test.go
// @brief Tests for reading an analyzer's output, and for SARIF both ways.
//
// This is where the mistakes that matter live, because nothing above here can see
// them. A finding that is read with a zero-based column points an editor at the
// character before the one meant; a severity that means the opposite of what it looks
// like turns a note into a build failure; a file that is dropped because it was not
// one the run asked about loses the finding a build most wants. So each reader is
// tested against the real output of the analyzer it reads, and against the case where
// a thing is *not* there — an absent column, an absent rule, an absent location.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  The gcc spelling                                                            */
/* -------------------------------------------------------------------------- */

func TestReadGCCReadsWhatClangWrites(t *testing.T) {
	// A real clang-tidy invocation's output, with the three shapes it uses: with a
	// column, without one, and with a `-W` name in the trailing bracket.
	spec := Spec{Name: "clang-tidy"}
	output := []byte(
		"/project/build/generated/c/runtime.c:212:14: warning: ISO C++17 requires " +
			"'std::size_t' to be defined [-Wdeprecated]  [clang-analyzer-core.NullDereference]\n" +
			"/project/src/main.c:9: warning: unused variable 'x' [-Wunused-variable]\n" +
			"/project/src/main.c:12: error: use of undeclared identifier 'q'\n" +
			"/project/src/main.c:14:1: note:  candidate function not viable\n")

	found := ReadGCC(output, spec, "/project")
	if len(found) != 4 {
		t.Fatalf("read %d findings, want 4: %v", len(found), found)
	}
	if found[0].Line != 212 || found[0].Column != 14 {
		t.Errorf("first at %d:%d, want 212:14", found[0].Line, found[0].Column)
	}
	if found[0].Severity != SeverityWarning {
		t.Errorf("severity %q, want a warning", found[0].Severity)
	}
	// The rule name carries the engine's, because `-Wdeprecated` in one analyzer and
	// in another are two checks and a project that suppresses one has said nothing
	// about the other.
	if found[0].Rule != "clang-tidy-clang-analyzer-core.NullDereference" {
		t.Errorf("rule %q, want it qualified by the engine", found[0].Rule)
	}
	// A diagnostic with no column is the compiler's own, and refusing it would drop
	// every one of them.
	if found[1].Column != 0 || found[1].Rule != "clang-tidy-Wunused-variable" {
		t.Errorf("second is %d:%d %q, want no column and the rule name", found[1].Line,
			found[1].Column, found[1].Rule)
	}
	if found[2].Severity != SeverityError {
		t.Errorf("third severity %q, want an error", found[2].Severity)
	}
	if found[3].Severity != SeverityNote || found[3].Column != 1 {
		t.Errorf("fourth is %q at column %d, want a note at column 1",
			found[3].Severity, found[3].Column)
	}
	for _, one := range found {
		if strings.HasSuffix(one.File, "/project/src/main.c") {
			t.Errorf("file %q is absolute, want it relative to the project", one.File)
		}
		if one.Engine != "clang-tidy" {
			t.Errorf("engine %q, want clang-tidy", one.Engine)
		}
	}
}

func TestReadGCCDoesNotMistakeAMessageForARule(t *testing.T) {
	// `subscript [3]` in prose is not a rule name, and treating it as one would put
	// `[3]` in a report as though it were a check somebody could suppress.
	spec := Spec{Name: "gcc"}
	found := ReadGCC([]byte("main.c:4:2: warning: array subscript [3] is out of bounds\n"), spec, "/project")
	if len(found) != 1 {
		t.Fatalf("read %d, want 1", len(found))
	}
	if found[0].Rule != "" {
		t.Errorf("rule %q, want none", found[0].Rule)
	}
	if !strings.Contains(found[0].Message, "subscript [3]") {
		t.Errorf("message %q, want the bracket left in it", found[0].Message)
	}
}

func TestReadGCCIgnoresLinesThatAreNotFindings(t *testing.T) {
	// An analyzer writes progress and errors to the same stream its findings come
	// out of, and a reader that guessed at those lines would produce findings about
	// text rather than about code.
	spec := Spec{Name: "clang-tidy"}
	found := ReadGCC([]byte(
		"Enabled checks: bugprone-*, clang-analyzer-*\n"+
			"error: no compilation commands found in compile_commands.json\n"+
			"1 warnings generated.\n"), spec, "/project")
	if len(found) != 0 {
		t.Errorf("read %v, want nothing from lines that are not diagnostics", found)
	}
}

func TestReadGCCHonoursAnUnqualifiedEngine(t *testing.T) {
	// A plugin may say the names are already unique, and then they must not be
	// qualified — otherwise every existing suppression of that name stops matching.
	off := false
	spec := Spec{Name: "cppcheck", Qualified: &off}
	found := ReadGCC([]byte("main.c:1:1: warning: something [-Wthing]\n"), spec, "/project")
	if len(found) != 1 || found[0].Rule != "Wthing" {
		t.Errorf("read %v, want the rule unqualified", found)
	}
}

/* -------------------------------------------------------------------------- */
/*  The cppcheck spelling                                                       */
/* -------------------------------------------------------------------------- */

func TestReadCPPCheckMapsItsOwnSeverities(t *testing.T) {
	// cppcheck's vocabulary is its own, and `style` and `information` in particular
	// do not mean what `warning` means anywhere else.
	spec := Spec{Name: "cppcheck"}
	output := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<results version="2">
  <cppcheck version="2.10"/>
  <errors>
    <error id="unusedVariable" severity="warning" msg="Unused variable: x">
      <location file="/project/src/main.c" line="7" column="12"/>
    </error>
    <error id="nullPointer" severity="error" msg="Null pointer dereference: p">
      <location file="/project/src/main.c" line="9" column="5"/>
    </error>
    <error id="constParameter" severity="style" msg="Function parameter not used">
      <location file="/project/src/main.c" line="11"/>
    </error>
    <error id="unreadVariable" severity="information" msg="Variable is never read">
      <location file="/project/src/main.c" line="13"/>
    </error>
    <error id="someInternal" severity="debug" msg="Something about the analysis">
      <location file="/project/src/main.c" line="15"/>
    </error>
  </errors>
</results>`)

	found, err := ReadCPPCheck(output, spec, "/project")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// The `debug` finding is the analyzer talking about its own analysis, not about
	// the code, and reporting it would drown a person in a page of nothing.
	if len(found) != 4 {
		t.Fatalf("read %d, want 4: %v", len(found), idsOf(found))
	}
	want := []Severity{SeverityWarning, SeverityError, SeverityWarning, SeverityNote}
	for index, one := range found {
		if one.Severity != want[index] {
			t.Errorf("finding %d is %q, want %q", index, one.Severity, want[index])
		}
		// cppcheck's own spelling, qualified by the engine: `unusedVariable` in
		// cppcheck and in another analyzer are two different checks.
		if !strings.HasPrefix(one.Rule, "cppcheck-") {
			t.Errorf("finding %d rule %q, want it qualified", index, one.Rule)
		}
	}
	// A location with a line and no column is normal for cppcheck.
	if found[2].Column != 0 {
		t.Errorf("column %d, want none", found[2].Column)
	}
}

func TestReadCPPCheckKeepsAProjectWideFinding(t *testing.T) {
	// cppcheck reports "unused function" against the configuration rather than
	// against a source, and it is the finding most worth acting on — so a report
	// that dropped it for having no location would be losing the most useful thing
	// the analyzer said.
	spec := Spec{Name: "cppcheck"}
	output := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<results version="2">
  <errors>
    <error id="unusedFunction" severity="warning" msg="Unused function: say">
      <location file="build/generated/c/main.c" line="40" column="7"/>
    </error>
    <error id="unmatchedSuppression" severity="information" msg="Unmatched suppression">
    </error>
  </errors>
</results>`)

	found, err := ReadCPPCheck(output, spec, "/project")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("read %d, want 2", len(found))
	}
	if found[1].File != "" || found[1].Line != 0 {
		t.Errorf("the finding with no location is %q:%d, want it kept with no place",
			found[1].File, found[1].Line)
	}
}

func TestReadCPPCheckSaysInconclusiveInTheSentence(t *testing.T) {
	// An inconclusive finding is one a person may want to look at and one a pipeline
	// must not gate on, and with one field to put it in, the sentence is where it
	// goes. Dropping it would present a guess as a finding.
	spec := Spec{Name: "cppcheck"}
	output := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<results version="2">
  <errors>
    <error id="nullPointer" severity="warning" msg="Possible null dereference"
           inconclusive="true" cwe="476">
      <location file="/project/src/main.c" line="9"/>
    </error>
  </errors>
</results>`)
	found, err := ReadCPPCheck(output, spec, "/project")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("read %d, want 1", len(found))
	}
	if !strings.HasPrefix(found[0].Message, "inconclusive:") {
		t.Errorf("message %q, want it marked inconclusive", found[0].Message)
	}
}

func TestReadCPPCheckRefusesSomethingThatIsNotItsReport(t *testing.T) {
	// A run that reported a malformed report as a clean tree would be worse than one
	// that said nothing.
	if _, err := ReadCPPCheck([]byte("<not xml"), Spec{Name: "cppcheck"}, "/project"); err == nil {
		t.Error("a document that is not a cppcheck report was accepted")
	}
}

/* -------------------------------------------------------------------------- */
/*  This component's own shape                                                  */
/* -------------------------------------------------------------------------- */

func TestReadJSONReadsBothShapes(t *testing.T) {
	// A flat list and a `{diagnostics: [...]}` envelope, because a project that has
	// already normalized its analyzers has one of them and should not be told to
	// write the other.
	spec := Spec{Name: "house", Qualified: nil}
	flat := []byte(`[{"rule":"x","message":"m","severity":"warning","file":"src/a.c","line":3}]`)
	envelope := []byte(`{"diagnostics":[{"rule":"x","message":"m","severity":"warning","file":"src/a.c","line":3}]}`)

	for name, payload := range map[string][]byte{"flat": flat, "envelope": envelope} {
		found, err := ReadJSON(payload, spec, "/project")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(found) != 1 || found[0].Rule != "house-x" || found[0].Engine != "house" {
			t.Errorf("%s: read %v, want one finding qualified and attributed", name, found)
		}
		if found[0].File != "src/a.c" {
			t.Errorf("%s: file %q", name, found[0].File)
		}
	}
	if _, err := ReadJSON([]byte(`{"nothing":1}`), spec, "/project"); err == nil {
		t.Error("a JSON file that is not findings was accepted")
	}
}

/* -------------------------------------------------------------------------- */
/*  SARIF                                                                       */
/* -------------------------------------------------------------------------- */

func TestWriteSARIFIsOneRunPerEngine(t *testing.T) {
	// The format says a run is one tool's results, and a consumer that merged them
	// would lose the attribution — which is what makes "these two disagree about this
	// line" answerable at all.
	first := []Diagnostic{{
		Rule: "c-missing-include-guard", Severity: SeverityWarning,
		Message: "no guard", File: "src/led.h", Line: 3, Column: 1, Engine: "c",
	}}
	second := []Diagnostic{{
		Rule: "cppcheck-unusedVariable", Severity: SeverityError,
		Message: "unused", File: "src/main.c", Line: 9, Column: 5, Engine: "cppcheck",
	}}
	runs := []Run{
		{Tool: Tool{Name: "c", Rules: []Rule{{ID: "c-missing-include-guard", Severity: SeverityWarning}}},
			Invocation: Invocation{Successful: true}, Results: first},
		{Tool: Tool{Name: "cppcheck"}, Invocation: Invocation{Successful: true, ExitCode: 1},
			Results: second},
	}

	encoded := writeSARIF(t, runs, "/project")
	runsOut := decodeRuns(t, encoded)
	if len(runsOut) != 2 {
		t.Fatalf("%d runs, want 2", len(runsOut))
	}
	if runsOut[0]["tool"].(map[string]any)["name"] != "c" {
		t.Errorf("first run's tool is %v, want c", runsOut[0]["tool"])
	}
	results := runsOut[0]["results"].([]any)
	firstResult := results[0].(map[string]any)
	if firstResult["level"] != "warning" {
		t.Errorf("level %v, want warning", firstResult["level"])
	}
	if firstResult["ruleId"] != "c-missing-include-guard" {
		t.Errorf("ruleId %v", firstResult["ruleId"])
	}
	location := firstResult["locations"].([]any)[0].(map[string]any)
	physical := location["physicalLocation"].(map[string]any)
	artifact := physical["artifactLocation"].(map[string]any)
	// Relative to the project and against a declared base: a `uri` that differs per
	// machine cannot be grouped by a code scanning service.
	if artifact["uri"] != "src/led.h" || artifact["uriBaseId"] != SourceRootBaseID {
		t.Errorf("artifact location %v, want src/led.h against the source root", artifact)
	}
	region := physical["region"].(map[string]any)
	if region["startLine"] != float64(3) || region["startColumn"] != float64(1) {
		t.Errorf("region %v, want 3:1", region)
	}

	// An analyzer that found something and exited non-zero has not failed, and a
	// consumer that only reads the exit code should not be told the tool is broken
	// every time it works.
	invocation := runsOut[1]["invocations"].([]any)[0].(map[string]any)
	if invocation["executionSuccessful"] != true || invocation["exitCode"] != float64(1) {
		t.Errorf("invocation %v, want successful with the analyzer's own exit code", invocation)
	}
}

func TestWriteSARIFHasNoTimestamp(t *testing.T) {
	// Every field a run does not need is left out, and a clock reading is the one
	// that guarantees two runs of the same tree differ. A pipeline that diffs its own
	// output cannot do that if every run carries a time.
	encoded := writeSARIF(t, []Run{{
		Tool: Tool{Name: "c"}, Invocation: Invocation{Successful: true},
	}}, "/project")
	for _, banned := range []string{"startTimeUtc", "endTimeUtc", "timestamp", "lastModified"} {
		if bytes.Contains(encoded, []byte(banned)) {
			t.Errorf("the report carries %q, which makes two runs of the same tree differ", banned)
		}
	}
}

func TestWriteSARIFNestsTheRuleSeverityAsTheFormatRequires(t *testing.T) {
	// `defaultConfiguration` is a `reportingConfiguration`, and a bare level string
	// there is a schema violation — the kind that makes a strict validator reject the
	// whole file and therefore every finding in it.
	encoded := writeSARIF(t, []Run{{
		Tool:       Tool{Name: "c", Rules: []Rule{{ID: "rho-parse", Severity: SeverityError}}},
		Invocation: Invocation{Successful: true},
	}}, "/project")
	runs := decodeRuns(t, encoded)
	rule := runs[0]["tool"].(map[string]any)["rules"].([]any)[0].(map[string]any)
	configuration, ok := rule["defaultConfiguration"].(map[string]any)
	if !ok {
		t.Fatalf("defaultConfiguration is %v, want an object", rule["defaultConfiguration"])
	}
	if configuration["level"] != "error" {
		t.Errorf("level %v, want error", configuration["level"])
	}
}

func TestWriteSARIFRoundTrips(t *testing.T) {
	// The other direction, and the reason the format is a setting on an engine's spec
	// rather than a parser per plugin: an analyzer that already speaks SARIF is a
	// plugin whose adapter is a format name.
	written := []Diagnostic{{
		Rule: "clang-tidy-Wunused", Severity: SeverityError,
		Message: "unused variable", File: "/project/src/main.c", Line: 7, Column: 12,
	}}
	encoded := writeSARIF(t, []Run{{
		Tool:       Tool{Name: "clang-tidy"},
		Invocation: Invocation{Successful: true},
		Results:    written,
	}}, "/project")

	back, err := ReadSARIF(encoded)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(back) != 1 {
		t.Fatalf("read %d findings, want 1", len(back))
	}
	one := back[0]
	if one.Rule != "clang-tidy-Wunused" || one.Message != "unused variable" {
		t.Errorf("read %+v, want the rule and the message", one)
	}
	if one.Severity != SeverityError {
		t.Errorf("severity %q, want error", one.Severity)
	}
	if one.File != "/project/src/main.c" || one.Line != 7 || one.Column != 12 {
		t.Errorf("place %q:%d:%d, want the source root resolved back to the file",
			one.File, one.Line, one.Column)
	}
	if one.Engine != "clang-tidy" {
		t.Errorf("engine %q, want the run's tool", one.Engine)
	}
}

func TestReadSARIFRefusesAFileThatIsNotOne(t *testing.T) {
	for name, payload := range map[string][]byte{
		"not json":   []byte("<html>"),
		"no version": []byte(`{"runs":[]}`),
	} {
		if _, err := ReadSARIF(payload); err == nil {
			t.Errorf("%s: accepted as SARIF", name)
		}
	}
}

func TestSARIFKeepsAFindingWithNoPlace(t *testing.T) {
	// cppcheck's project-wide findings have no location, and the format allows a
	// result without one. Refusing to parse it would lose the most useful thing an
	// analyzer says.
	found, err := ReadSARIF([]byte(`{"version":"2.1.0","runs":[
	  {"tool":{"name":"cppcheck"},"results":[
	    {"ruleId":"cppcheck-unusedFunction","level":"warning","message":{"text":"Unused function"}}
	  ]}]}`))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(found) != 1 || found[0].File != "" || found[0].Line != 0 {
		t.Errorf("read %+v, want one finding with no place", found)
	}
}

/* -------------------------------------------------------------------------- */

func writeSARIF(t *testing.T, runs []Run, root string) []byte {
	t.Helper()
	buffer := bytes.Buffer{}
	if err := WriteSARIF(&buffer, runs, root); err != nil {
		t.Fatalf("write SARIF: %v", err)
	}
	return buffer.Bytes()
}

func decodeRuns(t *testing.T, encoded []byte) []map[string]any {
	t.Helper()
	document := struct {
		Version string           `json:"version"`
		Runs    []map[string]any `json:"runs"`
	}{}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("the report is not JSON: %v", err)
	}
	if document.Version != SARIFVersion {
		t.Errorf("version %q, want %q", document.Version, SARIFVersion)
	}
	return document.Runs
}

/* -------------------------------------------------------------------------- */

func TestSeverityRankAndThreshold(t *testing.T) {
	// The threshold is compared as `rank <= rank` with the most severe at 0, so a
	// rule that fails on notes would not have to get the polarity of the comparison
	// right.
	if SeverityError.Rank() >= SeverityWarning.Rank() {
		t.Error("an error does not outrank a warning")
	}
	if SeverityNote.AtLeast(SeverityWarning) {
		t.Error("a note crosses a warning threshold")
	}
	if !SeverityWarning.AtLeast(SeverityWarning) {
		t.Error("a warning does not cross a warning threshold")
	}
	// `never` is a threshold and not a severity, which is why it is checked before
	// the ranking rather than being the bottom of the list.
	if SeverityError.AtLeast(ThresholdNever) {
		t.Error("`never` crossed by an error")
	}
	if SeverityNone.Rank() < len(Severities) {
		t.Error("`none` outranks a note")
	}
}

func TestMergeTakesOutOnlyTheSameFindingTwice(t *testing.T) {
	// Two analyzers reporting the same defect with the same wording is one defect;
	// two analyzers reporting the same *place* differently are two claims, and
	// collapsing those would discard the disagreement that is the reason to run two.
	first := []Diagnostic{{Rule: "a", Message: "same", File: "m.c", Line: 3, Engine: "a"}}
	second := []Diagnostic{
		{Rule: "a", Message: "same", File: "m.c", Line: 3, Engine: "b"},
		{Rule: "b", Message: "different", File: "m.c", Line: 3, Engine: "b"},
	}
	merged := Merge([][]Diagnostic{first, second})
	if len(merged) != 2 {
		t.Fatalf("merged into %d, want 2", len(merged))
	}
	if merged[0].Engine != "a" {
		t.Errorf("attributed to %q, want the first engine to report it", merged[0].Engine)
	}
}

func TestSortIsTotal(t *testing.T) {
	// A pipeline that diffs two runs of the same tree needs a total order to diff,
	// so the sentence is the last tiebreaker rather than nothing.
	shuffled := []Diagnostic{
		{Rule: "b", File: "m.c", Line: 1, Message: "second"},
		{Rule: "a", File: "m.c", Line: 1, Message: "first"},
		{Rule: "a", File: "a.c", Line: 9, Severity: SeverityError, Message: "z"},
		{Rule: "a", File: "m.c", Line: 1, Severity: SeverityError, Message: "worst"},
	}
	Sort(shuffled)
	if shuffled[0].File != "a.c" {
		t.Errorf("first is %q, want the earlier file", shuffled[0].File)
	}
	if shuffled[1].Severity != SeverityError {
		t.Errorf("second is %q, want the worst thing on that line first", shuffled[1].Severity)
	}
	if shuffled[2].Message != "first" {
		t.Errorf("third is %q, want the sentence to break the tie", shuffled[2].Message)
	}
	Sort(shuffled)
	if shuffled[3].Message != "second" {
		t.Error("sorting twice changed the order, so it is not total")
	}
}

func TestCountsAndTheirThreshold(t *testing.T) {
	counts := Of([]Diagnostic{
		{Severity: SeverityError}, {Severity: SeverityWarning}, {Severity: SeverityWarning},
		{Severity: SeverityNote}, {Severity: Severity("critical")},
	})
	if counts.Errors != 1 || counts.Warnings != 2 || counts.Notes != 1 || counts.Others != 1 {
		t.Errorf("counted %+v, want 1 error, 2 warnings, 1 note and 1 other", counts)
	}
	// An unrecognized severity is counted rather than dropped, so a run whose
	// analyzers all speak a spelling this component has not met still says how many
	// findings it read.
	if counts.Total() != 5 {
		t.Errorf("total %d, want 5", counts.Total())
	}
	if !counts.AtLeast(SeverityNote) || counts.AtLeast(ThresholdNever) {
		t.Error("the threshold is wrong")
	}
	if got := counts.SeverityText(); got != "1 error, 2 warnings, 1 note, 1 finding" {
		t.Errorf("text %q", got)
	}
	if Plural(1, "file") != "1 file" || Plural(2, "file") != "2 files" {
		t.Error("plural is wrong, and a report that says `1 files` is worse than useless")
	}
}
