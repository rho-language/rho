//
// @file runner_test.go
// @brief Tests for a run, end to end.
//
// These do not run anything. A run is tested through a fake runner and a
// temporary project, because what is being checked is the arrangement — what a
// run decided to do, in what order, and what it said — and a run that really
// compiled something would be testing the compiler and the C toolchain instead.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                  */
/* -------------------------------------------------------------------------- */

// fakeRunner takes steps without taking them, and records what it was asked to.
type fakeRunner struct {
	// outputs is what each step produces, in order. A step past the end produces
	// nothing, which is how a run of two steps is tested with three.
	outputs []string
	// failing is the step that fails, or -1 for none.
	failing int
	// taken is every step it was asked to take.
	taken []Step
}

func (f *fakeRunner) RunStep(step Step, suite Suite) StepOutcome {
	f.taken = append(f.taken, step)
	index := len(f.taken) - 1
	output := ""
	if index < len(f.outputs) {
		output = f.outputs[index]
	}
	if step.Action != nil {
		// A fake runner does not run an in-process step. A step whose work is a
		// build would really build, and these tests are about how a run accounts for
		// what it was told — not about whether a toolchain is installed. The engine's
		// handling of an action is covered where an action is what is under test.
		return StepOutcome{Output: output, Failed: index == f.failing}
	}
	return StepOutcome{Output: output, Failed: index == f.failing}
}

// project writes a project with a suite manifest, and returns its root.
func project(t *testing.T, manifest string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  name: p\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "tests.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// A kind says who knows how to run a suite; a tag says what class of test it is.
// They are different questions and the manifest keeps them apart, which is why a
// project's unit tests can all be selected with `--tag unit` whether each is
// built by the runner, by gtest or by a command.
const commandManifest = `name: p
suites:
  - name: pass
    kind: command
    command: ["/bin/true"]
    tags: [unit, fast]
  - name: fail
    kind: command
    command: ["/bin/false"]
    tags: [unit, slow]
  - name: later
    kind: smoke
    tags: [smoke, hardware]
    status: skip
    reason: no board is wired to this machine yet
`

/* -------------------------------------------------------------------------- */
/*  A project with no manifest                                                 */
/* -------------------------------------------------------------------------- */

// A project with Rho test files and no manifest is still described by what it
// has: the discovery a runner did before there were manifests is a description
// too, and refusing it would make adding a manifest mandatory rather than better.
func TestARunFallsBackToTheRhoTestFilesItFinds(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  name: p\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "math_test.rho"), []byte("test fn adds() {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{failing: -1}

	report := RunSuite(Options{
		Root:   root,
		Path:   root,
		Runner: runner,
		Out:    discard(),
	})

	if report.Err != nil {
		t.Fatalf("expected a run, got %v", report.Err)
	}
	if len(report.Suites) != 1 || report.Suites[0].Name != "math" {
		t.Fatalf("expected the file's suite, got %+v", report.Suites)
	}
}

// A project with neither a manifest nor any test file has said nothing about
// what it has, and a run that reports "nothing to do" would be indistinguishable
// from one that ran nothing successfully.
func TestAProjectThatDeclaresNothingIsAnError(t *testing.T) {
	root := t.TempDir()
	report := RunSuite(Options{Root: root, Path: root, Out: discard()})
	if report.Err == nil {
		t.Fatal("expected a project with no manifest and no tests to be an error")
	}
	if !strings.Contains(report.Err.Error(), "no suite manifest") {
		t.Errorf("expected the error to say what was missing, got %v", report.Err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Selection                                                                  */
/* -------------------------------------------------------------------------- */

// A run that finds nothing where a filter named something is a filter that did
// not match, and that is worth saying rather than reporting as a clean run.
func TestAFilterThatMatchesNothingSaysSo(t *testing.T) {
	root := project(t, commandManifest)
	var printed strings.Builder
	report := RunSuite(Options{
		Root:      root,
		Path:      root,
		Selection: Selection{Tags: []string{"nonexistent"}},
		Out:       &fileFor{&printed},
	})

	if report.Err != nil {
		t.Fatalf("expected the run to complete, got %v", report.Err)
	}
	if !strings.Contains(printed.String(), "No suites matched") {
		t.Errorf("expected to be told nothing matched, got:\n%s", printed.String())
	}
}

func TestATagSelectsSuites(t *testing.T) {
	root := project(t, commandManifest)
	runner := &fakeRunner{failing: -1}
	report := RunSuite(Options{
		Root:      root,
		Path:      root,
		Selection: Selection{Tags: []string{"fast"}},
		Runner:    runner,
		Out:       discard(),
	})

	if len(report.Suites) != 1 || report.Suites[0].Name != "pass" {
		t.Fatalf("expected only the fast suite, got %+v", report.Suites)
	}
}

func TestAnExcludedTagKeepsASuiteOut(t *testing.T) {
	root := project(t, commandManifest)
	report := RunSuite(Options{
		Root:      root,
		Path:      root,
		Selection: Selection{Excluded: []string{"slow", "hardware"}},
		Runner:    &fakeRunner{failing: -1},
		Out:       discard(),
	})

	if len(report.Suites) != 1 || report.Suites[0].Name != "pass" {
		t.Fatalf("expected only the fast, non-hardware suite, got %+v", report.Suites)
	}
}

func TestAKindSelectsSuites(t *testing.T) {
	root := project(t, commandManifest)
	report := RunSuite(Options{
		Root:      root,
		Path:      root,
		Selection: Selection{Kinds: []string{"smoke"}},
		Runner:    &fakeRunner{failing: -1},
		Out:       discard(),
	})

	if len(report.Suites) != 1 || report.Suites[0].Name != "later" {
		t.Fatalf("expected only the smoke suite, got %+v", report.Suites)
	}
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                    */
/* -------------------------------------------------------------------------- */

func TestASuiteThatFailsFailsTheRun(t *testing.T) {
	root := project(t, commandManifest)
	report := RunSuite(Options{
		Root:   root,
		Path:   root,
		Runner: &fakeRunner{failing: 1},
		Out:    discard(),
	})

	// Two unit suites were selected: the first succeeded, the second failed.
	if report.Summary.Failed == 0 {
		t.Fatalf("expected the run to fail, got %+v", report.Summary)
	}
	if report.ExitCode() != 1 {
		t.Errorf("expected a failing exit code, got %d", report.ExitCode())
	}
}

func TestARunThatPassesExitsZero(t *testing.T) {
	root := project(t, commandManifest)
	report := RunSuite(Options{
		Root:      root,
		Path:      root,
		Selection: Selection{Tags: []string{"fast"}},
		Runner:    &fakeRunner{failing: -1},
		Out:       discard(),
	})

	if report.Summary.Failed != 0 {
		t.Fatalf("expected nothing to fail, got %+v", report.Summary)
	}
	if report.ExitCode() != 0 {
		t.Errorf("expected a clean exit code, got %d", report.ExitCode())
	}
}

// A suite declared as not run is not a suite that passed, and it must not make a
// run pass silently either — it must be visible in the summary.
func TestASkippedSuiteIsCountedAsSkippedAndNotAsPassed(t *testing.T) {
	root := project(t, commandManifest)
	report := RunSuite(Options{
		Root:      root,
		Path:      root,
		Selection: Selection{Names: []string{"later"}},
		Runner:    &fakeRunner{failing: -1},
		Out:       discard(),
	})

	if report.Summary.SkippedSuites != 1 {
		t.Fatalf("expected one skipped suite, got %+v", report.Summary)
	}
	if report.Summary.Failed != 0 {
		t.Errorf("a skipped suite is not a failure, got %+v", report.Summary)
	}
	if len(report.Results) != 1 || !report.Results[0].Skipped() {
		t.Fatalf("expected a skipped result, got %+v", report.Results)
	}
	if !strings.Contains(report.Results[0].Reason, "no board is wired") {
		t.Errorf("expected the declared reason to be carried, got %q", report.Results[0].Reason)
	}
}

// A suite that names cases, ran, and named none has tested nothing. Reporting it
// as a pass is how a run claims to have tested something it did not — which is
// why this is a failure and not a silence.
func TestASuiteThatReportsNoCasesFails(t *testing.T) {
	root := project(t, `name: p
suites:
  - name: silent
    kind: rho
`)
	if err := os.WriteFile(filepath.Join(root, "silent_test.rho"), []byte("test fn nothing() {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := RunSuite(Options{
		Root:   root,
		Path:   root,
		Runner: &fakeRunner{failing: -1},
		Out:    discard(),
	})

	if report.Summary.Failed == 0 {
		t.Fatalf("expected a silent suite to fail, got %+v", report.Summary)
	}
	found := false
	for _, result := range report.Results {
		if strings.Contains(result.Reason, "no cases") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the reason to say the suite reported nothing, got %+v", report.Results)
	}
}

// A suite whose outcome is its status — a build — has no cases to name and is not
// expected to. It passes when it succeeded, and a run that reported silence for
// it would leave the reader unable to tell it from one that never ran.
func TestAStatusSuitePassesWhenItSucceeds(t *testing.T) {
	root := project(t, commandManifest)
	report := RunSuite(Options{
		Root:      root,
		Path:      root,
		Selection: Selection{Tags: []string{"fast"}},
		Runner:    &fakeRunner{failing: -1},
		Out:       discard(),
	})

	if len(report.Results) != 1 {
		t.Fatalf("expected one result, got %+v", report.Results)
	}
	if !report.Results[0].Passed() {
		t.Fatalf("expected the build to pass, got %+v", report.Results[0])
	}
}

// A suite that is not there to run — a kind nothing is installed for — is
// reported rather than dropped. A run that passed while a declared test went
// unrun is the outcome a smoke suite must never have.
func TestASuiteWithNoRunnerIsSkippedWithAReason(t *testing.T) {
	root := project(t, `name: p
suites:
  - name: needs-gtest
    kind: gtest
`)
	report := RunSuite(Options{
		Root:   root,
		Path:   root,
		Runner: &fakeRunner{failing: -1},
		Out:    discard(),
	})

	if len(report.Suites) != 1 || !report.Suites[0].Skipped() {
		t.Fatalf("expected the suite to be skipped, got %+v", report.Suites)
	}
	if !strings.Contains(report.Suites[0].Skip, "gtest") {
		t.Errorf("expected the reason to name the kind, got %q", report.Suites[0].Skip)
	}
	if len(report.Diagnostics) == 0 {
		t.Error("expected the missing runner to be reported")
	}
}

// A suite resolved to nothing is a plugin that answered and said nothing, which
// is different from a plugin that did not answer: it is a broken extension, and
// a broken extension must not leave a run looking complete.
func TestASuiteResolvedToNothingIsReported(t *testing.T) {
	root := project(t, `name: p
suites:
  - name: empty
    kind: empty-kind
`)
	report := RunSuite(Options{
		Root:   root,
		Path:   root,
		Runner: &fakeRunner{failing: -1},
		Out:    discard(),
		Plugins: WithPlugins([]Plugin{fakePlugin(t, root, map[string]string{
			"empty-kind": "{}",
		}, map[string]string{
			"empty-kind": `{"suites":[{"declared":"empty","steps":[]}]}`,
		})}),
	})

	if !report.Suites[0].Skipped() {
		t.Fatalf("expected a suite with no steps to be skipped, got %+v", report.Suites[0])
	}
	if len(report.Diagnostics) == 0 {
		t.Error("expected the empty suite to be reported")
	}
}

/* -------------------------------------------------------------------------- */
/*  Listing                                                                    */
/* -------------------------------------------------------------------------- */

func TestAListSaysWhatTheProjectDeclares(t *testing.T) {
	root := project(t, commandManifest)
	var printed strings.Builder
	RunSuite(Options{Root: root, Path: root, List: true, Out: &fileFor{&printed}})

	for _, want := range []string{"pass", "fail", "later", "hardware", "no board is wired"} {
		if !strings.Contains(printed.String(), want) {
			t.Errorf("expected the listing to mention %q, got:\n%s", want, printed.String())
		}
	}
}

func TestADryRunSaysWhatItWouldDoWithoutDoingIt(t *testing.T) {
	root := project(t, commandManifest)
	runner := &fakeRunner{failing: -1}
	var printed strings.Builder
	RunSuite(Options{
		Root:      root,
		Path:      root,
		Selection: Selection{Tags: []string{"unit"}},
		DryRun:    true,
		Runner:    runner,
		Out:       &fileFor{&printed},
	})

	if len(runner.taken) != 0 {
		t.Fatalf("expected a dry run to take no steps, got %d", len(runner.taken))
	}
	for _, want := range []string{"selected: tag=unit", "run   pass", "run   fail"} {
		if !strings.Contains(printed.String(), want) {
			t.Errorf("expected the plan to say %q, got:\n%s", want, printed.String())
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Capture                                                                    */
/* -------------------------------------------------------------------------- */

// discard is where a test's output goes when the test is not about it.
func discard() *os.File {
	read, write, err := os.Pipe()
	if err != nil {
		return os.Stderr
	}
	go func() {
		read.Close()
	}()
	return write
}

// fileFor is a sink the run's output goes to, so a test can read what it said.
type fileFor struct{ into *strings.Builder }

func (f *fileFor) Write(p []byte) (int, error) { return f.into.Write(p) }
func (f *fileFor) Name() string                { return "test-sink" }

/* -------------------------------------------------------------------------- */
/*  A plugin, without a process                                                */
/* -------------------------------------------------------------------------- */

// fakePlugin is a plugin that answers from a table rather than from a program.
//
// The process-based plugin harness lives in plugin_test.go, and it is the right
// way to test the protocol. This is here for the cases that are about the run's
// reaction rather than about the protocol: what it does with a suite that
// resolved to nothing needs a plugin whose answer is known to be empty, and that
// is awkward to arrange with a real subprocess.
func fakePlugin(t *testing.T, root string, hooks, payloads map[string]string) Plugin {
	t.Helper()
	path := filepath.Join(root, "fake-plugin")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, filepath.Join(path, helperName)); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	hookNames := []string{}
	for hook := range hooks {
		hookNames = append(hookNames, "  - "+hook)
	}
	commands := "commands:\n"
	for hook := range hooks {
		commands += "  - name: " + hook + "\n    run: " + helperName +
			" -test.run TestRunnerHelperProcess -- payload:" + hook + "\n    hook: " + hook + "\n"
	}
	manifest := "module:\n  name: fake-plugin\n  type: plugin\n  version: 0.1.0\n\n" +
		"main: fake-plugin\n\nextends: " + ComponentName + "\n\nhooks:\n" +
		strings.Join(hookNames, "\n") + "\n" + commands
	if err := os.WriteFile(filepath.Join(path, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	found := DiscoverPlugins(Source{Tree: root, TrustTree: true})
	if len(found) != 1 {
		t.Fatalf("expected the fake plugin to be found, got %+v", found)
	}
	return found[0]
}
