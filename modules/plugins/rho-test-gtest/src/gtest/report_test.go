//
// @file report_test.go
// @brief Tests for reading what GoogleTest wrote down.
//
// The framework's report is the only place it states its results unambiguously, so
// what matters here is that each of the three things a test can be — pass, fail,
// skip — comes back as itself, and that its name comes back as the framework wrote
// it rather than as something tidied up.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package gtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                  */
/* -------------------------------------------------------------------------- */

// googleTestReport is what a GoogleTest binary writes with --gtest_output=xml.
const googleTestReport = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="4" failures="1" disabled="0" errors="0" time="0.01" name="AllTests">
  <testsuite name="arithmetic" tests="3" failures="1" disabled="0" skipped="1" time="0.01">
    <testcase name="adds" file="tests/test_arith.c" line="12" status="run" result="completed" time="0.001" classname="arithmetic"/>
    <testcase name="fails" file="tests/test_arith.c" line="20" status="run" result="completed" time="0.002" classname="arithmetic">
      <failure message="Expected equality of these values: 1, 2" type="">
tests/test_arith.c:20: Failure
Expected equality of these values: 1, 2
some stack frames here
more frames
yet more
      </failure>
    </testcase>
    <testcase name="notyet" file="tests/test_arith.c" line="30" status="notrun" result="suppressed" time="0" classname="arithmetic"/>
  </testsuite>
  <testsuite name="strings" tests="1" failures="0" disabled="0" skipped="0" time="0.001">
    <testcase name="compares" file="tests/test_str.c" line="8" status="run" result="completed" time="0.001" classname="strings"/>
  </testsuite>
</testsuites>`

/* -------------------------------------------------------------------------- */
/*  Reading                                                                   */
/* -------------------------------------------------------------------------- */

func TestEveryOutcomeIsReadAsItself(t *testing.T) {
	outcomes, err := Parse(strings.NewReader(googleTestReport))
	if err != nil {
		t.Fatalf("expected the report to be readable, got %v", err)
	}
	if len(outcomes) != 4 {
		t.Fatalf("expected four tests, got %d: %+v", len(outcomes), outcomes)
	}

	byName := map[string]Outcome{}
	for _, outcome := range outcomes {
		byName[outcome.Name] = outcome
	}
	if got := byName["arithmetic.adds"].Status; got != StatusPass {
		t.Errorf("expected `adds` to pass, got %q", got)
	}
	if got := byName["strings.compares"].Status; got != StatusPass {
		t.Errorf("expected `compares` to pass, got %q", got)
	}
	if got := byName["arithmetic.fails"].Status; got != StatusFail {
		t.Errorf("expected `fails` to fail, got %q", got)
	}
	if got := byName["arithmetic.notyet"].Status; got != StatusSkip {
		t.Errorf("expected `notyet` to be skipped, got %q", got)
	}
}

// The name the framework printed is the name a person's filter and their CI
// selection are written against, so it is kept whole rather than tidied.
func TestATestKeepsTheNameTheFrameworkGaveIt(t *testing.T) {
	outcomes, err := Parse(strings.NewReader(googleTestReport))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, outcome := range outcomes {
		names = append(names, outcome.Name)
	}
	for _, want := range []string{"arithmetic.adds", "strings.compares"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q among %v", want, names)
		}
	}
}

// A failure's first line is the assertion; the frames after it are what a person
// scrolls past. Trimming keeps a report readable, and the marker says there was
// more rather than pretending there was not.
func TestAFailureKeepsItsAssertionAndNotItsWholeStack(t *testing.T) {
	outcomes, err := Parse(strings.NewReader(googleTestReport))
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range outcomes {
		if outcome.Name != "arithmetic.fails" {
			continue
		}
		if !strings.Contains(outcome.Output, "Expected equality of these values: 1, 2") {
			t.Errorf("expected the assertion in the output, got %q", outcome.Output)
		}
		if strings.Contains(outcome.Output, "yet more") {
			t.Errorf("expected the stack trimmed, got %q", outcome.Output)
		}
		if !strings.Contains(outcome.Output, "...") {
			t.Errorf("expected a marker that there was more, got %q", outcome.Output)
		}
		return
	}
	t.Fatal("the failing test was not found")
}

// The order is stable, so two runs of the same suite report the same way and a
// diff between two reports is about what changed.
func TestOutcomesComeBackInAStableOrder(t *testing.T) {
	first, err := Parse(strings.NewReader(googleTestReport))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Parse(strings.NewReader(googleTestReport))
	if err != nil {
		t.Fatal(err)
	}
	for index := range first {
		if first[index].Name != second[index].Name {
			t.Fatalf("expected the same order, got %q then %q", first[index].Name, second[index].Name)
		}
	}
	if first[0].Name != "arithmetic.adds" {
		t.Errorf("expected the tests sorted, got %q first", first[0].Name)
	}
}

// A binary that wrote no report did not pass. It either crashed before the
// framework could write, or it is not a GoogleTest binary, and both are facts.
func TestAMissingReportIsAnErrorNotAnEmptyPass(t *testing.T) {
	if _, err := Read(filepath.Join(t.TempDir(), "absent.xml")); err == nil {
		t.Fatal("expected a missing report to be an error")
	}
}

func TestAReportThatIsNotAReportIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "junk.xml")
	if err := os.WriteFile(path, []byte("this is not xml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("expected an unreadable report to be an error")
	}
}

/* -------------------------------------------------------------------------- */
/*  Saying it                                                                 */
/* -------------------------------------------------------------------------- */

// What the plugin prints is what the runner's parser reads, so the format is the
// runner's and not the framework's.
func TestOutcomesAreSaidTheWayTheRunnerReadsThem(t *testing.T) {
	outcomes, err := Parse(strings.NewReader(googleTestReport))
	if err != nil {
		t.Fatal(err)
	}
	said := strings.Join(Lines(outcomes), "\n")

	for _, want := range []string{
		"[PASS] arithmetic.adds",
		"[PASS] strings.compares",
		"[FAIL] arithmetic.fails",
		"[SKIP] arithmetic.notyet",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("expected %q in:\n%s", want, said)
		}
	}
	// A failure carries its assertion under it, indented, so a report line reads
	// as one entry rather than as several.
	if !strings.Contains(said, "  Expected equality") {
		t.Errorf("expected the assertion under the failure, got:\n%s", said)
	}
}
