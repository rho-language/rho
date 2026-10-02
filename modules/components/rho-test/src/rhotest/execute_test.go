//
// @file execute_test.go
// @brief Tests for taking a suite's steps and saying what came of them.
//
// The engine is where a run's honesty lives: a step that failed ends its suite,
// a suite that said nothing is not a suite that passed, and a skipped suite is not
// a passing one. Those are the cases here, taken against a runner that does not
// run anything, because what is being checked is the accounting.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

/* -------------------------------------------------------------------------- */
/*  A runner that does not run                                                */
/* -------------------------------------------------------------------------- */

// scriptedRunner answers each step from a script, so a run can be arranged
// exactly and its consequences checked.
type scriptedRunner struct {
	// script is one entry per step: what it prints, and whether it failed.
	script []stepOutcome
	// err is what the failing step returns, if anything.
	err error
}

type stepOutcome struct {
	output string
	failed bool
}

func (r scriptedRunner) RunStep(step Step, suite Suite) (string, bool, error) {
	if r.err != nil && step.Optional == false {
		// Used by the optional-step tests, where the point is the error rather
		// than the status.
		defer func() {}()
	}
	index := 0
	for _, taken := range r.script {
		if taken.output != "" || taken.failed {
			if index == 0 {
				return taken.output, taken.failed, r.err
			}
		}
		index++
	}
	return "", false, nil
}

// countingRunner records the steps it took and answers from a fixed output.
type countingRunner struct {
	steps []Step
	out   string
	fail  int
	err   error
	// skipping is the step that reports it could not be tried, and reason is what
	// it says about that.
	skipping int
	reason   string
}

// It does not run an in-process step. A step whose work is a build would really
// build, and a test of the engine's accounting should not also depend on a
// toolchain being installed — so the one test that is about such a step uses
// actionRunner, which does run it.
func (r *countingRunner) RunStep(step Step, suite Suite) StepOutcome {
	r.steps = append(r.steps, step)
	index := len(r.steps) - 1
	if r.reason != "" && index == r.skipping {
		return StepOutcome{Output: r.out, Skipped: true, Reason: r.reason}
	}
	if r.err != nil {
		return StepOutcome{Output: r.out, Failed: index == r.fail, Err: r.err}
	}
	return StepOutcome{Output: r.out, Failed: index == r.fail}
}

// actionRunner runs an in-process step, which is what the engine does with one and
// what this stands in for.
type actionRunner struct{ taken []Step }

func (r *actionRunner) RunStep(step Step, suite Suite) StepOutcome {
	r.taken = append(r.taken, step)
	if step.Action != nil {
		output, failed, err := step.Action(suite)
		return StepOutcome{Output: output, Failed: failed, Err: err}
	}
	return StepOutcome{}
}

func oneStepSuite(name string, verdict Verdict) Suite {
	return Suite{
		Name:    name,
		Kind:    KindCommand,
		Verdict: verdict,
		Root:    ".",
		Steps:   []Step{{Name: "run", Command: []string{"true"}}},
	}
}

/* -------------------------------------------------------------------------- */
/*  Taking steps                                                               */
/* -------------------------------------------------------------------------- */

// The steps of a suite are taken in order, and one that fails ends the suite —
// which is what makes a later step's success irrelevant.
func TestASuiteTakesItsStepsInOrderAndStopsAtTheFirstFailure(t *testing.T) {
	runner := &countingRunner{fail: 1}
	report := Execute([]Suite{{
		Name:    "pipeline",
		Kind:    KindCommand,
		Verdict: VerdictStatus,
		Steps: []Step{
			{Name: "configure", Command: []string{"cmake"}},
			{Name: "build", Command: []string{"cmake", "--build"}},
			{Name: "run", Command: []string{"ctest"}},
		},
	}}, ExecuteOptions{Runner: runner})

	if len(runner.steps) != 2 {
		t.Errorf("expected the steps up to the failure, got %d", len(runner.steps))
	}
	if report.Summary.Failed == 0 {
		t.Errorf("expected the suite to fail, got %+v", report.Summary)
	}
	if !strings.Contains(firstOutput(report), "build") {
		t.Errorf("expected the failing step to be named, got %q", firstOutput(report))
	}
}

func firstOutput(report RunReport) string {
	for _, result := range report.Results {
		return result.Output
	}
	return ""
}

// A step that discovers whether something is present is allowed to fail without
// failing the suite. That is the whole difference between an optional step and a
// required one.
func TestAnOptionalStepMayFailWithoutFailingTheSuite(t *testing.T) {
	runner := &countingRunner{err: errors.New("no such toolchain")}
	report := Execute([]Suite{{
		Name:    "probe",
		Kind:    KindCommand,
		Verdict: VerdictStatus,
		Steps: []Step{
			{Name: "look for a toolchain", Command: []string{"arm-none-eabi-gcc"}, Optional: true},
		},
	}}, ExecuteOptions{Runner: runner})

	if report.Summary.Failed != 0 {
		t.Errorf("an optional step failing is not the suite failing, got %+v", report.Summary)
	}
	if len(report.Diagnostics) == 0 {
		t.Error("expected the optional step's failure to be reported as a diagnostic")
	}
}

// An in-process step and a step that is a program are taken the same way. It is
// what lets a suite be four operations and one of them be this runner's own.
func TestAnActionStepIsTakenLikeACommand(t *testing.T) {
	report := Execute([]Suite{{
		Name:    "rho",
		Kind:    KindRho,
		Verdict: VerdictStatus,
		Steps: []Step{{
			Name: "build and run",
			Action: func(Suite) (string, bool, error) {
				return "an action ran", false, nil
			},
		}},
	}}, ExecuteOptions{Runner: &actionRunner{}})

	if report.Summary.Failed != 0 {
		t.Errorf("expected the suite to pass, got %+v", report.Summary)
	}
	if !strings.Contains(report.Results[0].Output, "an action ran") {
		t.Errorf("expected the action's output to be kept, got %+v", report.Results[0])
	}
}

// A step that reports it could not be tried ends the suite as skipped, and carries
// the reason in the step's own words rather than in the runner's.
//
// This is what a build says when its vendor tree is not fetched or its board has no
// spec. A run that read that as a failure would report a machine with one toolchain
// as a platform with fifty broken boards.
func TestAStepThatCouldNotBeTriedSkipsTheSuiteAndKeepsItsReason(t *testing.T) {
	report := Execute([]Suite{{
		Name:    "sw-smoke",
		Kind:    KindBuild,
		Verdict: VerdictStatus,
		Steps: []Step{{
			Name:    "build hello_world",
			Verdict: StepVerdictBuild,
			Command: []string{"true"},
		}},
	}}, ExecuteOptions{Runner: &countingRunner{reason: "no vendor tree fetched for STM32F7"}})

	if report.Summary.Skipped != 1 {
		t.Fatalf("expected the suite to be skipped, got %+v", report.Summary)
	}
	if report.Summary.Failed != 0 {
		t.Errorf("a step that could not be tried is not a failure, got %+v", report.Summary)
	}
	if !strings.Contains(report.Results[0].Reason, "STM32F7") {
		t.Errorf("expected the step's own reason, got %q", report.Results[0].Reason)
	}
}

// A build's exit code is the contract: 0 built, CannotRunHere could not be tried,
// anything else did not build. The distinction is read from the code rather than
// from the message, because a runner that parsed the message would have to be
// updated every time the toolchain reworded one.
func TestABuildsExitCodeDecidesBetweenBuiltAndCannotBeTried(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no shell to test with")
	}
	built := ExecRunner{}.RunStep(Step{
		Name:    "build",
		Verdict: StepVerdictBuild,
		Command: []string{"/bin/sh", "-c", "echo 'all good'; exit 0"},
	}, Suite{Root: "."})
	if built.Failed || built.Skipped {
		t.Errorf("expected a build that worked to pass, got %+v", built)
	}

	notTried := ExecRunner{}.RunStep(Step{
		Name:    "build",
		Verdict: StepVerdictBuild,
		Command: []string{"/bin/sh", "-c", "echo 'no vendor tree fetched for STM32F7'; exit 77"},
	}, Suite{Root: "."})
	if notTried.Failed {
		t.Errorf("expected a build that cannot be tried to skip, not fail, got %+v", notTried)
	}
	if !notTried.Skipped {
		t.Errorf("expected the suite to be skipped, got %+v", notTried)
	}
	if !strings.Contains(notTried.Reason, "STM32F7") {
		t.Errorf("expected the build's own reason, got %q", notTried.Reason)
	}

	broke := ExecRunner{}.RunStep(Step{
		Name:    "build",
		Verdict: StepVerdictBuild,
		Command: []string{"/bin/sh", "-c", "echo 'error: undefined reference'; exit 1"},
	}, Suite{Root: "."})
	if !broke.Failed || broke.Skipped {
		t.Errorf("expected a build that did not compile to fail, got %+v", broke)
	}
}

/* -------------------------------------------------------------------------- */
/*  Reading cases out of what a suite said                                      */
/* -------------------------------------------------------------------------- */

// A framework that names its cases is believed, because the name it prints is
// what a person's muscle memory and their CI filter are written against.
func TestCasesAreReadFromWhatAFrameworkPrinted(t *testing.T) {
	runner := &countingRunner{fail: -1, out: strings.Join([]string{
		"[==========] Running 3 tests.",
		"[ RUN      ] arithmetic.adds",
		"[       OK ] arithmetic.adds",
		"[ RUN      ] arithmetic.fails",
		"[  FAILED  ] arithmetic.fails",
		"[ RUN      ] strings.compares",
		"[       OK ] strings.compares",
		"[==========] 3 tests, 2 passed, 1 failed.",
	}, "\n")}

	report := Execute([]Suite{{
		Name:    "unit",
		Kind:    "gtest",
		Verdict: VerdictCases,
		Steps:   []Step{{Name: "run", Command: []string{"tests/unit", "--gtest_output=xml:out.xml"}}},
	}}, ExecuteOptions{Runner: runner})

	if len(report.Results) != 3 {
		t.Fatalf("expected three cases, got %+v", report.Results)
	}
	if report.Summary.Passed != 2 || report.Summary.Failed != 1 {
		t.Errorf("expected two passed and one failed, got %+v", report.Summary)
	}
	// In the order they were run, not the order they were found.
	if report.Results[0].Case.Name != "arithmetic.adds" || report.Results[2].Case.Name != "strings.compares" {
		t.Errorf("expected the cases in the order they ran, got %+v", report.Results)
	}
	if !strings.HasPrefix(report.Results[1].Case.Name, "arithmetic.fails") {
		t.Errorf("expected the failing case to be named, got %q", report.Results[1].Case.Name)
	}
}

// The summary lines a framework prints are not cases. Believing them would add a
// test called "3 passed" to every run.
func TestASummaryLineIsNotACase(t *testing.T) {
	cases := ParseCases(strings.Join([]string{
		"[==========] 3 tests from 1 test suite ran.",
		"[       OK ] arithmetic.adds",
		"[  PASSED  ] 2 tests.",
	}, "\n"))
	if len(cases) != 1 {
		t.Fatalf("expected only the real case, got %+v", cases)
	}
	if cases[0].Case.Name != "arithmetic.adds" {
		t.Errorf("expected the real case, got %q", cases[0].Case.Name)
	}
}

// A program that prints something which happens to look like a result is not
// making a result. Guessing here would invent a failing test out of a log line,
// which is worse than naming the suite and saying it failed.
func TestOutputThatMerelyLooksLikeAResultIsNotOne(t *testing.T) {
	if cases := ParseCases("starting up\nlogging [PASS] to the console\ndone\n"); len(cases) != 0 {
		t.Fatalf("expected no cases from a program's own output, got %+v", cases)
	}
}

/* -------------------------------------------------------------------------- */
/*  Counting                                                                   */
/* -------------------------------------------------------------------------- */

// Suites and cases are counted separately, because they are different things: one
// cell of a smoke matrix that did not build is one failure, not one failure per
// case it would have had.
func TestSuitesAndCasesAreCountedSeparately(t *testing.T) {
	summary := Summarize(
		[]Suite{{Name: "a"}, {Name: "b"}, {Name: "c", Skip: "not ready"}},
		[]Result{
			{Suite: "a", Case: Case{Name: "one"}, Status: StatusPass},
			{Suite: "a", Case: Case{Name: "two"}, Status: StatusPass},
			{Suite: "b", Case: Case{Name: "one"}, Status: StatusFail},
		},
	)
	if summary.Suites != 3 || summary.Cases != 3 {
		t.Errorf("expected three of each, got %+v", summary)
	}
	if summary.Passed != 2 || summary.Failed != 1 {
		t.Errorf("expected two passed and one failed, got %+v", summary)
	}
	if summary.FailedSuites != 1 {
		t.Errorf("expected one failing suite, got %+v", summary)
	}
	if summary.SkippedSuites != 1 {
		t.Errorf("expected one skipped suite, got %+v", summary)
	}
	if summary.OK() {
		t.Error("expected the verdict to follow the failure")
	}
}

// One suite of a hundred cases that would not build is one failing suite, and
// reporting it as a hundred failures is what makes a matrix unreadable.
func TestOneFailingSuiteIsOneFailureNotOnePerCase(t *testing.T) {
	results := []Result{{Suite: "smoke", Case: Case{Name: "build"}, Status: StatusFail}}
	for index := 0; index < 99; index++ {
		results = append(results, Result{Suite: "smoke", Case: Case{Name: "case"}, Status: StatusFail})
	}
	summary := Summarize([]Suite{{Name: "smoke"}}, results)
	if summary.FailedSuites != 1 {
		t.Errorf("expected one failing suite, got %d", summary.FailedSuites)
	}
}

// A run with skipped suites is not a failing run — a skipped suite says why it
// was not run — but the counts have to carry them, or the gap disappears between
// one run and the next.
func TestASkippedSuiteIsNotAFailureButIsCounted(t *testing.T) {
	summary := Summarize([]Suite{{Name: "a", Skip: "not ready"}}, nil)
	if !summary.OK() {
		t.Error("a skipped suite is not a failure")
	}
	if summary.SkippedSuites != 1 {
		t.Errorf("expected it to be counted, got %+v", summary)
	}
}

/* -------------------------------------------------------------------------- */
/*  Taking a real step                                                         */
/* -------------------------------------------------------------------------- */

// The execution engine is exercised here against a real process, because bounding
// one is the part that cannot be faked: a step that hangs must not hang the run.
func TestAStepThatHangsIsKilledAndTheSuiteFails(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no shell to test with")
	}
	runner := ExecRunner{Timeout: 100 * time.Millisecond}
	outcome := runner.RunStep(Step{
		Name:    "hang",
		Command: []string{"/bin/sh", "-c", "sleep 30"},
	}, Suite{Root: "."})

	if !outcome.Failed || outcome.Err == nil {
		t.Fatalf("expected a bounded step to fail, got %+v", outcome)
	}
	if !strings.Contains(outcome.Err.Error(), "timed out") {
		t.Errorf("expected the error to say it timed out, got %v", outcome.Err)
	}
}

// A step's output is read through a pipe, and every process that inherits the
// write end holds it open. So a step that started a child of its own is not
// finished when the step is: killing only the step leaves the child running, the
// pipe open, and the run waiting on it for as long as the child lives — which is
// exactly what the timeout is supposed to prevent.
func TestATimeoutTakesDownAChildTheStepStarted(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no shell to test with")
	}
	started := time.Now()
	runner := ExecRunner{Timeout: 200 * time.Millisecond}
	outcome := runner.RunStep(Step{
		Name:    "leaves a child",
		Command: []string{"/bin/sh", "-c", "sleep 60 & sleep 60"},
	}, Suite{Root: "."})

	if !outcome.Failed || outcome.Err == nil {
		t.Fatalf("expected the step to be bounded, got %+v", outcome)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("expected the step's whole tree to be taken down, but it took %s", elapsed)
	}
}

func TestAStepThatSucceedsProducesItsOutput(t *testing.T) {
	if _, err := os.Stat("/bin/echo"); err != nil {
		t.Skip("no echo to test with")
	}
	outcome := ExecRunner{}.RunStep(Step{
		Name:    "say",
		Command: []string{"/bin/echo", "hello"},
	}, Suite{Root: "."})
	if outcome.Err != nil || outcome.Failed {
		t.Fatalf("expected the step to succeed, got %+v", outcome)
	}
	if strings.TrimSpace(outcome.Output) != "hello" {
		t.Errorf("expected the step's output, got %q", outcome.Output)
	}
}

// A step that cannot start is a failure with a reason, not a silent no-op.
func TestAStepThatCannotStartIsReported(t *testing.T) {
	outcome := ExecRunner{}.RunStep(Step{
		Name:    "absent",
		Command: []string{"/definitely/not/a/program"},
	}, Suite{Root: "."})
	if !outcome.Failed || outcome.Err == nil {
		t.Fatalf("expected a missing program to be reported, got %+v", outcome)
	}
	if !strings.Contains(outcome.Err.Error(), "absent") {
		t.Errorf("expected the error to name the step, got %v", outcome.Err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Resolving                                                                  */
/* -------------------------------------------------------------------------- */

// A kind this runner does not know is a gap in the installation, and it is
// reported as one: a run that passed while a declared test went unrun is the
// outcome a smoke suite must never have.
func TestAnUnknownKindIsSkippedWithAReason(t *testing.T) {
	suites, problems := ResolveAll(
		[]Declared{{Name: "needs-gtest", Kind: "gtest"}},
		"/project",
		BuiltinResolvers()...,
	)
	if len(suites) != 1 || !suites[0].Skipped() {
		t.Fatalf("expected the suite to be skipped, got %+v", suites)
	}
	if !strings.Contains(suites[0].Skip, "gtest") {
		t.Errorf("expected the reason to name the kind, got %q", suites[0].Skip)
	}
	if len(problems) != 1 {
		t.Errorf("expected the gap to be reported, got %+v", problems)
	}
}

// A suite declared as not run is not resolved at all — the answer was given in the
// manifest, and asking a plugin again would be asking a question with a known
// answer.
func TestASkippedSuiteIsNotResolved(t *testing.T) {
	resolver := ResolverFunc{Kind: "rho", Run: func(Declared, string) (Suite, error) {
		t.Fatal("a suite declared as not run must not be resolved")
		return Suite{}, nil
	}}
	suites, problems := ResolveAll([]Declared{{Name: "a", Kind: "rho", Skip: "not written yet"}}, "/p", resolver)
	if len(problems) != 0 {
		t.Errorf("expected no problems from a suite nobody will run, got %+v", problems)
	}
	if !suites[0].Skipped() || !strings.Contains(suites[0].Skip, "not written yet") {
		t.Errorf("expected the declared reason to be kept, got %+v", suites[0])
	}
}

// A resolver that produces a suite with no steps has answered and said nothing,
// which is a broken extension rather than an empty test.
func TestAResolverThatProducesNoStepsIsReported(t *testing.T) {
	resolver := ResolverFunc{Kind: "rho", Run: func(d Declared, root string) (Suite, error) {
		return Suite{Name: d.Name}, nil
	}}
	suites, problems := ResolveAll([]Declared{{Name: "a", Kind: "rho"}}, "/p", resolver)
	if len(suites) != 1 || !suites[0].Skipped() {
		t.Fatalf("expected the suite to be skipped, got %+v", suites)
	}
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), "no steps") {
		t.Errorf("expected the empty suite to be reported, got %+v", problems)
	}
}

// A Rho suite with no tests in it would compile, run, assert nothing and pass.
// That is a green run over an empty file, so it is caught when the suite is
// resolved rather than becoming a pass nobody can explain.
func TestARhoSuiteWithNoTestsIsReported(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty_test.rho"), []byte("fn helper() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	suites, problems := ResolveAll([]Declared{{Name: "empty", Kind: KindRho}}, root, BuiltinResolvers()...)
	if len(suites) != 1 || !suites[0].Skipped() {
		t.Fatalf("expected the suite to be skipped, got %+v", suites)
	}
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), "declares no tests") {
		t.Errorf("expected the empty file to be reported, got %+v", problems)
	}
}

func TestARhoSuiteResolvesToItsCasesAndOneStep(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "math_test.rho"), []byte("test fn adds() {\n}\ntest fn subtracts() {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	suites, problems := ResolveAll([]Declared{{Name: "math", Kind: KindRho}}, root, BuiltinResolvers()...)
	if len(problems) != 0 {
		t.Fatalf("expected it to resolve, got %+v", problems)
	}
	if len(suites[0].Cases) != 2 {
		t.Errorf("expected the file's two cases, got %+v", suites[0].Cases)
	}
	if len(suites[0].Steps) != 1 || !suites[0].Steps[0].Runs() {
		t.Errorf("expected one step that does something, got %+v", suites[0].Steps)
	}
}

func TestACommandSuiteIsResolvedFromItsCommand(t *testing.T) {
	suites, problems := ResolveAll(
		[]Declared{{Name: "smoke", Kind: KindCommand, Command: []string{"./smoke.sh", "--fast"}}},
		"/project",
		BuiltinResolvers()...,
	)
	if len(problems) != 0 {
		t.Fatalf("expected it to resolve, got %+v", problems)
	}
	if suites[0].Verdict != VerdictStatus {
		t.Errorf("a command suite's outcome is its status, got %q", suites[0].Verdict)
	}
	if strings.Join(suites[0].Steps[0].Command, " ") != "./smoke.sh --fast" {
		t.Errorf("expected the declared command, got %v", suites[0].Steps[0].Command)
	}
}

// A command suite with neither a command nor a script is a claim nobody can act
// on, and it is reported rather than skipped in silence.
func TestACommandSuiteWithNothingToRunIsReported(t *testing.T) {
	suites, problems := ResolveAll(
		[]Declared{{Name: "smoke", Kind: KindCommand}},
		t.TempDir(),
		BuiltinResolvers()...,
	)
	if !suites[0].Skipped() {
		t.Errorf("expected the suite to be skipped, got %+v", suites[0])
	}
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), "declares no command") {
		t.Errorf("expected the missing command to be reported, got %+v", problems)
	}
}
