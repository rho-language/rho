//
// @file execute.go
// @brief Running a plan and saying what came of it.
//
// A suite is run by taking its steps in order. A step that fails ends the suite,
// and the suite fails with it — unless the step said it was optional, which is
// how a step that discovers whether something is present stays optional.
//
// The two questions a run has to answer are kept apart on purpose. Which cases
// passed is the suite's own business: a framework that can name its cases names
// them, and one that cannot has its one unnamed case rather than a list of
// invented ones. Whether the suite as a whole passed is this file's: a suite whose
// steps failed did not pass, whatever its cases said, and a suite that was skipped
// did not pass either.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhotest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

/* -------------------------------------------------------------------------- */
/*  How a run reaches its tests                                                */
/* -------------------------------------------------------------------------- */

// Runner is what actually takes a suite's steps. It is an interface so a run can
// be tested without building anything, and so a suite whose steps are already
// resolved by a plugin do not have to be re-executed to be inspected.
type Runner interface {
	// RunStep takes one step and says what came of it.
	RunStep(step Step, suite Suite) StepOutcome
}

// StepOutcome is what one step left behind.
//
// Four things rather than two, because a step has four outcomes and collapsing
// them loses the one that matters most here: a step that was not tried at all is
// not a step that failed, and a run that cannot say the difference will report a
// machine with one toolchain as a platform with fifty broken boards.
type StepOutcome struct {
	// Output is what the step printed.
	Output string
	// Failed is that the step failed.
	Failed bool
	// Skipped is that the step was not tried, with a reason for why. A suite whose
	// step was skipped is a suite that did not run.
	Skipped bool
	// Reason is why, in the words the step used.
	Reason string
	// Err is the failure, when there was one.
	Err error
}

// ExecRunner runs steps as processes.
//
// Bounded, because a test run that hangs is the failure nobody notices until a CI
// job times out an hour later, and a step's own timeout says how long its author
// thought it needed.
type ExecRunner struct {
	// Timeout bounds one step. Zero means DefaultStepTimeout.
	Timeout time.Duration
	// Env is the environment a step runs in, on top of the process's own.
	Env []string
}

// DefaultStepTimeout is how long a step may take before it is killed.
//
// Generous, because a first build of a test binary is slow and a timeout that
// fires on the first build of a slow machine reads as a broken test rather than
// as a slow one. Short enough that a hang is a failure rather than an afternoon.
const DefaultStepTimeout = 15 * time.Minute

// RunStep takes one step.
//
// A step with no command does nothing and succeeds: that is a step that only
// declares something for the next one to read.
func (r ExecRunner) RunStep(step Step, suite Suite) StepOutcome {
	if step.Action != nil {
		// An action is already this runner's own work, so it is bounded rather than
		// spawned: the pieces it runs as programs are bounded by the runner it uses.
		output, failed, err := step.Action(suite)
		return StepOutcome{Output: output, Failed: failed, Err: err}
	}
	if !step.Runs() {
		return StepOutcome{}
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = DefaultStepTimeout
	}
	dir := step.Dir
	if dir == "" {
		dir = suite.Root
	}
	if dir != "" && !filepath.IsAbs(dir) {
		dir = filepath.Join(suite.Root, dir)
	}

	process := exec.Command(step.Command[0], step.Command[1:]...)
	process.Dir = dir
	process.Env = append(append(os.Environ(), r.Env...), step.Env...)
	// Its own process group, so that a timeout can take the whole tree down.
	//
	// This is not a refinement. A step's output is collected through a pipe, and
	// every process that inherits the write end of that pipe keeps it open — so a
	// step that started a child of its own would not be killable by killing it: the
	// child holds the pipe, Wait waits for the child, and the timeout that was
	// supposed to end the step is the thing waiting on it. A suite that hangs would
	// hang the run for as long as the child chose to live, which is exactly the
	// failure the timeout exists to prevent.
	setProcessGroup(process)

	var out bytes.Buffer
	process.Stdout = &out
	process.Stderr = &out

	if err := process.Start(); err != nil {
		return StepOutcome{Output: out.String(), Failed: true, Err: fmt.Errorf("start %s: %w", step.Name, err)}
	}
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()

	select {
	case err := <-done:
		return verdictFor(step, out.String(), err, step.Name)
	case <-time.After(timeout):
		// The tree is killed rather than abandoned, so a step that timed out does not
		// go on holding the tree it was built in, and so the next suite is not run
		// against a directory somebody else is still writing to.
		killTree(process)
		<-done
		return StepOutcome{
			Output: out.String(),
			Failed: true,
			Err:    fmt.Errorf("%s: timed out after %s", step.Name, timeout),
		}
	}
}

// verdictFor reads a step's exit status, which for some steps says more than
// "failed".
//
// A build is the case this exists for: it exits CannotRunHere to say the code is
// not the problem and the machine is, and a runner that read that as a failure
// would report every board without a toolchain as a broken platform. The reason is
// taken from what the build printed rather than invented, because the build is the
// only thing that knows which of its inputs was missing.
func verdictFor(step Step, output string, err error, name string) StepOutcome {
	if err == nil {
		return StepOutcome{Output: output}
	}
	if step.Verdict == StepVerdictBuild {
		if exit, ok := exitStatus(err); ok && exit == CannotRunHere {
			return StepOutcome{
				Output:  output,
				Skipped: true,
				Reason:  reasonFrom(output, name),
			}
		}
	}
	return StepOutcome{Output: output, Failed: true, Err: fmt.Errorf("%s: %w", name, err)}
}

// exitStatus is a process's exit code, when the error carries one.
func exitStatus(err error) (int, bool) {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return 0, false
	}
	return exit.ExitCode(), true
}

// reasonFrom is what a skipped step said about itself.
//
// The last line that is not empty: a build narrates as it goes, and the thing that
// went wrong is at the end of it.
func reasonFrom(output, name string) string {
	lines := []string{}
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return fmt.Sprintf("%s cannot be run on this machine", name)
	}
	return lines[len(lines)-1]
}

/* -------------------------------------------------------------------------- */
/*  What a run concluded                                                       */
/* -------------------------------------------------------------------------- */

// RunReport is everything a run concluded: what it did, what came of it, and the
// notes it wants reported.
type RunReport struct {
	// Suites are the suites the run considered, in the order it ran them.
	Suites []Suite
	// Results are the case results, in the order the suites ran.
	Results []Result
	// Summary is the count of all of it.
	Summary Summary
	// Diagnostics are the things that were wrong but did not fail a suite: an
	// optional step that did not find what it was looking for, a plugin's note.
	Diagnostics []string
	// Seconds is how long the run took, for a report that says more than a count.
	Seconds float64
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                   */
/* -------------------------------------------------------------------------- */

// ExecuteOptions configure a run.
type ExecuteOptions struct {
	// Root is the project the run is over. Every suite's relative paths resolve
	// against it.
	Root string
	// Runner takes the steps. ExecRunner is used when this is nil.
	Runner Runner
	// Verbose prints each step as it is taken, which is what makes a slow run
	// legible rather than a silence followed by a verdict.
	Verbose bool
	// Echo prints a suite's output whether it passed or failed, which is what
	// finds the print a passing test got wrong.
	Echo bool
}

// Execute runs suites and reports what came of them.
func Execute(suites []Suite, options ExecuteOptions) RunReport {
	started := time.Now()
	runner := options.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	report := RunReport{Suites: suites}

	for _, suite := range suites {
		if suite.Skipped() {
			report.Results = append(report.Results, Result{
				Suite:  suite.Name,
				Case:   Case{Name: suite.Name},
				Status: StatusSkip,
				Reason: suite.Skip,
			})
			if options.Verbose {
				fmt.Printf("  skip   %s  (%s)\n", suite.Name, suite.Skip)
			}
			continue
		}
		results, output, failed, skipped, reason, diagnostics := runSuite(suite, runner, options)
		report.Results = append(report.Results, results...)
		report.Diagnostics = append(report.Diagnostics, diagnostics...)

		// A suite whose step was not tried is a suite that did not run, and it is
		// reported as exactly that. It is neither a pass — nothing was checked — nor
		// a failure — nothing broke — and a run that has to pick one of those two is
		// the reason a smoke suite over many boards gets either all-green or
		// all-red.
		if skipped {
			report.Results = append(report.Results, Result{
				Suite:  suite.Name,
				Case:   Case{Name: suite.Name},
				Status: StatusSkip,
				Reason: reason,
			})
			if options.Verbose {
				fmt.Printf("  skip   %s  (%s)\n", suite.Name, reason)
			}
			continue
		}

		// A suite whose outcome is its status has one result, not a list of cases —
		// and it has one even when nothing went wrong, because a build that linked
		// is a fact the run should report rather than a silence.
		if len(results) == 0 && !failed {
			if suite.Verdict == VerdictStatus {
				report.Results = append(report.Results, Result{
					Suite:  suite.Name,
					Case:   Case{Name: suite.Name},
					Status: StatusPass,
					Output: output,
				})
			} else {
				// A suite that names cases, ran, and named none has tested nothing.
				// Reporting it as a pass is how a run claims to have tested something
				// it did not.
				report.Results = append(report.Results, Result{
					Suite:  suite.Name,
					Case:   Case{Name: suite.Name},
					Status: StatusFail,
					Output: output,
					Reason: "the suite ran but reported no cases",
				})
			}
		}
		if failed {
			report.Results = append(report.Results, Result{
				Suite:  suite.Name,
				Case:   Case{Name: suite.Name},
				Status: StatusFail,
				Output: output,
			})
		}
		if options.Echo && output != "" {
			fmt.Print(indentBlock(output, "    "))
		}
	}
	report.Summary = Summarize(report.Suites, report.Results)
	report.Seconds = time.Since(started).Seconds()
	return report
}

// runSuite takes one suite's steps and turns them into results.
//
// The suite's own output is kept whatever happened, because a failure report that
// does not include what the test printed is a report that sends the reader back
// to the machine to get it.
//
// A step that reports it could not be tried ends the suite as skipped rather than
// as failed, and says why in the step's own words.
func runSuite(suite Suite, runner Runner, options ExecuteOptions) (results []Result, output string, failed bool, skipped bool, reason string, diagnostics []string) {
	for index, step := range suite.Steps {
		label := step.Name
		if label == "" {
			label = fmt.Sprintf("step %d", index+1)
		}
		if options.Verbose && step.Runs() {
			fmt.Printf("    %s: %s\n", suite.Name, strings.Join(step.Command, " "))
		}
		outcome := runner.RunStep(step, suite)
		output += outcome.Output
		if outcome.Output != "" {
			// A step that only declares something has no output of its own, but the
			// step after it may, and both belong to the suite's record.
			output += "\n"
		}
		if outcome.Skipped {
			return nil, output, false, true, outcome.Reason, diagnostics
		}
		if outcome.Err != nil {
			if step.Optional {
				diagnostics = append(diagnostics, fmt.Sprintf("%s: %v", suite.Name, outcome.Err))
				continue
			}
			// The failing step's own words go into the suite's output, because a
			// report that says a suite failed without saying which step is the one
			// thing a reader cannot go and check.
			output += label + ": " + outcome.Err.Error() + "\n"
			return nil, output, true, false, "", diagnostics
		}
		if outcome.Failed {
			output += label + ": the step reported a failure\n"
			return nil, output, true, false, "", diagnostics
		}
		// A step that ran and produced cases is the run. Everything before it was
		// in order to make it possible, and everything after it is another suite's
		// business.
		if cases := ParseCases(outcome.Output); len(cases) > 0 {
			return cases, output, false, false, "", diagnostics
		}
	}
	return nil, output, false, false, "", diagnostics
}

func indentBlock(text, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for index, line := range lines {
		lines[index] = prefix + line
	}
	return strings.Join(lines, "\n") + "\n"
}

/* -------------------------------------------------------------------------- */
/*  Cases a framework printed                                                 */
/* -------------------------------------------------------------------------- */

// CaseFormat is how a suite's own runner announces a case's result.
//
// A framework that prints nothing parseable still gets its cases reported — one
// case for the run, named after the suite — because a suite that ran and said
// nothing is not a suite that reported nothing. What this is for is the other
// case: a framework that does print lines, and a run that can name the failures
// individually rather than as one anonymous failure.
type CaseFormat struct {
	// PassPrefix starts a line that reports a passing case.
	PassPrefix string
	// FailPrefix starts a line that reports a failing case.
	FailPrefix string
}

// CommonCaseFormats are the shapes a test runner prints its cases in.
//
// Read from output rather than taken on trust, because a suite that names a
// format and a binary that prints another is a mismatch the reader can see. A
// format not listed is simply not parsed, and the suite gets one case.
var CommonCaseFormats = []CaseFormat{
	// GoogleTest's per-case lines, which carry the case's name on them. Its
	// `[  PASSED  ] N tests.` summary is deliberately not here: that line is a
	// count, not a case, and reading it as one adds a test called "2 tests" to
	// every run.
	{PassPrefix: "[       OK ] ", FailPrefix: "[  FAILED  ] "},
	// A runner of this toolchain's own.
	{PassPrefix: "[PASS] ", FailPrefix: "[FAIL] "},
	// TAP.
	{PassPrefix: "ok ", FailPrefix: "not ok "},
}

// ParseCases reads the case results a suite's own runner printed.
//
// Only the two prefix forms are read, and a line that is neither is not a case:
// the alternative is guessing at whatever a program printed, and a run that
// invents a failing case out of a log line is worse than one that names the suite
// and says it failed.
func ParseCases(output string) []Result {
	lines := strings.Split(output, "\n")
	// A framework prints its cases after everything else, so only the tail is
	// considered: a program's own output can contain anything at all, and a line
	// that happens to start like a result is not one.
	for _, format := range CommonCaseFormats {
		if cases := parseCasesWith(format, lines); len(cases) > 0 {
			return cases
		}
	}
	return nil
}

func parseCasesWith(format CaseFormat, lines []string) []Result {
	results := []Result{}
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		switch {
		case format.FailPrefix != "" && strings.HasPrefix(line, format.FailPrefix):
			results = append(results, Result{
				Case:   Case{Name: strings.TrimSpace(strings.TrimPrefix(line, format.FailPrefix))},
				Status: StatusFail,
				Output: strings.TrimSpace(line),
			})
		case format.PassPrefix != "" && strings.HasPrefix(line, format.PassPrefix):
			results = append(results, Result{
				Case:   Case{Name: strings.TrimSpace(strings.TrimPrefix(line, format.PassPrefix))},
				Status: StatusPass,
			})
		default:
			if len(results) > 0 && line == "" {
				// The block of results has ended, once past its first blank line.
				continue
			}
		}
	}
	return reverse(results)
}

func reverse(results []Result) []Result {
	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}
	return results
}

/* -------------------------------------------------------------------------- */
/*  Taking a tree down                                                         */
/* -------------------------------------------------------------------------- */

// setProcessGroup puts a step in a process group of its own.
//
// A shell that runs a command is the ordinary case rather than an unusual one —
// a step is very often a shell — and a shell's child inherits everything the shell
// had, including the pipe the step's output is read through. Killing the shell
// alone therefore leaves the child running and the pipe open, and the wait for the
// shell's output never finishes.
func setProcessGroup(process *exec.Cmd) {
	if process.SysProcAttr == nil {
		process.SysProcAttr = &syscall.SysProcAttr{}
	}
	process.SysProcAttr.Setpgid = true
}

// killTree takes down the step and everything it started.
func killTree(process *exec.Cmd) {
	if process.Process == nil {
		return
	}
	// The group, because the direct child is rarely the only process involved. A
	// child that has already been reaped cannot be signalled at all, and the group
	// can.
	if group, err := syscall.Getpgid(process.Process.Pid); err == nil {
		if err := syscall.Kill(-group, syscall.SIGKILL); err == nil {
			return
		}
	}
	_ = process.Process.Kill()
}
