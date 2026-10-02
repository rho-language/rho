//
// @file suite.go
// @brief What a test run is made of.
//
// The rebase this file carries out is one of vocabulary. A run used to be a list
// of test files, each of which was a list of cases, and the runner knew — with no
// way to be told otherwise — that every one of them was a Rho file it should
// translate, compile with the host compiler and run as a process.
//
// That is true of exactly one kind of test. A project written in C has tests
// that are C, built by the project's own build system and run by whatever test
// framework the project chose, and a runner that assumes Rho cannot reach them.
// The way it was extended for that was to let a plugin name files, which then
// went down the Rho path anyway.
//
// So the unit is now a suite: a named thing that can be run, that knows its own
// kind, and that a run reports on. What kind it is decides who knows how to run
// it — this runner for the kinds it has, and a plugin for the rest. Nothing else
// about a suite is assumed, which is what lets a gtest suite and a Rho suite sit
// in one run.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

import (
	"sort"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Case                                                                      */
/* -------------------------------------------------------------------------- */

// Case is one test: a name, and the place in the source a reader would go to
// find it.
//
// The place is optional because not every case lives in a file a person wrote.
// A case produced by a build — an image that linked, a program that started — is
// a fact about the build, and a report that pointed at a source line for it would
// be pointing at nothing.
type Case struct {
	// Name is what the run reports. A framework's own name is kept rather than
	// rewritten, because the name a framework prints is what a person's muscle
	// memory and their CI filter are written against.
	Name string
	// File is the source the case came from, when it came from one.
	File string
	// Line is where in that file, when the file is a source a reader can open.
	Line int
}

// Describe is a human-readable location for the case, for a report line.
func (c Case) Describe() string {
	switch {
	case c.File != "" && c.Line > 0:
		return c.File + ":" + itoa(c.Line)
	case c.File != "":
		return c.File
	default:
		return ""
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

/* -------------------------------------------------------------------------- */
/*  Step                                                                      */
/* -------------------------------------------------------------------------- */

// Step is one thing a run does to get a suite's result: build it, list what is
// in it, run it.
//
// Steps rather than one command, because getting a test to run is almost never
// one command. Something is built first, and a build that fails is not the same
// fact as a test that fails, and a run that reports both as "failed" throws away
// the difference between them.
type Step struct {
	// Name is what the run prints when this step is taken, so a failure says
	// which step failed rather than only that something did.
	Name string
	// Dir is the working directory, resolved against the suite's root when
	// relative. Empty is the root.
	Dir string
	// Command is the program and its arguments. A step with no command and no
	// Action is a step that only produces output, which is how a step runs
	// something whose output the next step reads.
	Command []string
	// Action runs the step in this process rather than as a program.
	//
	// It is here because not every step is a program a person could type.
	// Compiling a Rho test file is: strip the marker, write it, build it, compile
	// what comes out, run that. Those are four things, three of them this runner's
	// own, and reducing them to one argv would mean a shell script per test file.
	// Everything else about a step — its order, its output, its failure — is the
	// engine's, so a step that is a program and a step that is an action are
	// treated the same way throughout.
	//
	// Not encoded: it is a Go function, and this struct travels to plugins as JSON.
	// A plugin cannot be handed a function — it is a separate program — which is
	// exactly why a plugin describes its steps as commands instead.
	Action func(suite Suite) (output string, failed bool, err error) `json:"-"`
	// Env is set for the programs this step runs, on top of the process's own.
	//
	// For a step that hands a path to what it has just fetched: a plugin that
	// retrieves GoogleTest cannot leave the build to guess where it went, and
	// cannot write the answer into a project's files either.
	Env []string
	// Produces names what this step leaves behind for the next one to read: an
	// output file, a build directory. A suite that is skipped says what it was
	// waiting for, which is almost always this.
	Produces string
	// Optional is whether the step may fail without failing the suite. A step
	// that discovers whether something is present is optional; one that has to
	// succeed for the run to mean anything is not.
	Optional bool
	// Verdict is what a failing exit status means for this step.
	//
	// A step that fails is normally a failure, and this is for the one that is
	// not: a build exits 77 to say the code did not fail, the machine cannot try it
	// yet. Reading that as a failure is what makes a smoke suite over fifty boards
	// report fifty failures on a machine with one toolchain, and it is why the
	// number is a convention rather than something this runner invented.
	Verdict StepVerdict
}

// StepVerdict is what a step's exit status means.
type StepVerdict string

const (
	// StepVerdictDefault is that a failure failed.
	StepVerdictDefault StepVerdict = ""
	// StepVerdictBuild reads a build's exit status: 0 built, CannotRunHere skipped,
	// anything else failed.
	StepVerdictBuild StepVerdict = "build"
)

// Runs reports whether the step does anything.
func (s Step) Runs() bool { return len(s.Command) > 0 || s.Action != nil }

/* -------------------------------------------------------------------------- */
/*  Suite                                                                     */
/* -------------------------------------------------------------------------- */

// Verdict is how a suite's outcome is read, which differs between the two kinds
// of test there are.
type Verdict string

const (
	// VerdictCases is a suite whose own runner named its cases, and whose output
	// says what each one did. A test framework is like this: it knows the name of
	// every assertion it made, and a run that reports the suite as one failure
	// throws away the only information it had.
	VerdictCases Verdict = "cases"
	// VerdictStatus is a suite that has no per-case output, where whether it
	// succeeded is the whole of what it has to say. A build is like this: it links
	// or it does not, and there is nothing to say about which of a hundred
	// translation units was the one that failed beyond which build step said so.
	//
	// This is why it is stated rather than inferred. A suite that produced no cases
	// could be a test that ran and said nothing — which is a failure, because it
	// tested nothing — or a build that succeeded, which is a pass. Only whoever
	// wrote the suite knows which.
	VerdictStatus Verdict = "status"
)

// Suite is one runnable unit and everything a run needs to say about it.
type Suite struct {
	// Name is what the run reports it as. A suite that came out of a matrix is
	// named after the axes it was expanded from, so "hello_world/nucleo-h753zi"
	// rather than a generated number nobody can connect to a cell.
	Name string
	// Kind is what knows how to run it: KindRho, KindCommand, or a name a plugin
	// answers for.
	Kind string
	// Verdict is how the suite's outcome is read. Empty means VerdictCases.
	Verdict Verdict
	// Tags are what selection matches against. They are free-form by design: a
	// project tags what it finds useful, and the runner does not need to know the
	// vocabulary in advance to filter on it.
	Tags []string
	// Cases are what the suite is expected to contain. A runner does not require
	// a suite to declare them — a framework knows better than a manifest does —
	// but a suite that does is reporting failures it could name without running.
	Cases []Case
	// Steps are how the suite is run, in order.
	Steps []Step
	// Root is the directory a step's relative Dir and a suite's paths resolve
	// against. It is absolute, so a run does not depend on where it was started
	// from.
	Root string
	// Skip is why this suite will not be run, when it will not. A suite that is
	// skipped has said out loud that it did not pass: a smoke test for a board
	// whose toolchain is not installed has not been satisfied, and reporting it as
	// satisfied is the one way a smoke suite lies.
	Skip string
	// Output is anything a run produced from the suite's own steps, kept so a
	// report can show why a suite failed without the run having to have printed
	// it at the time.
	Output string
}

// Skipped reports whether the suite will not be run.
func (s Suite) Skipped() bool { return s.Skip != "" }

// Tagged reports whether the suite carries a tag.
func (s Suite) Tagged(tag string) bool {
	for _, held := range s.Tags {
		if held == tag {
			return true
		}
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  Result                                                                    */
/* -------------------------------------------------------------------------- */

// Status is what a run concluded about a case.
type Status string

const (
	// StatusPass is a case that ran and held.
	StatusPass Status = "pass"
	// StatusFail is a case that ran and did not hold, or a suite that could not
	// be run at all. Both are a failure: a suite that will not build has not
	// passed.
	StatusFail Status = "fail"
	// StatusSkip is a case or suite that was not run, with a reason. Reported
	// separately from a failure because it is a different fact, and counted
	// separately because a suite of skipped suites is not a suite that passed.
	StatusSkip Status = "skip"
)

// Result is what a run concluded about one case of one suite.
type Result struct {
	// Suite is the suite the case belongs to, which is the name the manifest gave
	// it rather than a pointer into a structure that no longer exists by the time
	// a plugin reads this.
	Suite string
	Case  Case
	// Status is what the run concluded.
	Status Status
	// Output is what the case printed, kept for a report that shows a failure's
	// output under it.
	Output string
	// Reason is why a case was skipped.
	Reason string
}

// Passed reports whether the result counts as a pass.
func (r Result) Passed() bool { return r.Status == StatusPass }

// Skipped reports whether the result was not run.
func (r Result) Skipped() bool { return r.Status == StatusSkip }

/* -------------------------------------------------------------------------- */
/*  Summary                                                                   */
/* -------------------------------------------------------------------------- */

// Summary is what a run concluded, in the counts a report and a process exit
// code both need.
type Summary struct {
	Suites  int
	Cases   int
	Passed  int
	Failed  int
	Skipped int
	// FailedSuites and SkippedSuites count suites, which is not the same as
	// counting cases: a suite of a hundred cases that would not build is one
	// failure and not a hundred, and a report that says otherwise is unreadable.
	FailedSuites  int
	SkippedSuites int
}

// OK reports whether the run passed: nothing failed. A run with skipped suites is
// not a failing run — a skipped suite says why it was not run — but it is not a
// complete one either, which is why the counts are reported rather than folded
// into the verdict.
func (s Summary) OK() bool { return s.Failed == 0 }

// Summarize counts a run's results.
//
// Every result is attributed to a suite, and a suite that produced no result at
// all — because it was skipped, or because it produced one case for a hundred
// declared — still counts as a suite that ran. That is what keeps "1 of 27 smoke
// suites passed" from being reported as "1 test passed".
func Summarize(suites []Suite, results []Result) Summary {
	summary := Summary{Suites: len(suites), Cases: len(results)}
	seen := map[string]bool{}
	for _, result := range results {
		switch result.Status {
		case StatusPass:
			summary.Passed++
		case StatusSkip:
			summary.Skipped++
		default:
			summary.Failed++
		}
		if result.Suite != "" && !seen[result.Suite] {
			seen[result.Suite] = true
		}
	}
	// A suite whose cases were all attributed to other suites still counts once.
	for _, result := range results {
		if result.Suite != "" {
			seen[result.Suite] = true
		}
	}
	for _, suite := range suites {
		if suite.Skipped() {
			summary.SkippedSuites++
			continue
		}
		failed := false
		for _, result := range results {
			if result.Suite == suite.Name && result.Status == StatusFail {
				failed = true
				break
			}
		}
		if failed {
			summary.FailedSuites++
		}
	}
	return summary
}

/* -------------------------------------------------------------------------- */
/*  Naming                                                                    */
/* -------------------------------------------------------------------------- */

// AxisSeparator joins the axes of a suite that was expanded from a matrix.
//
// Not a slash, because an axis value is very often a path — a smoke matrix's axes
// are the examples and the boards — and a path has slashes in it. Joining with
// one would make a cell's name ambiguous: `example=examples/c/hello_world` would
// read back as two axes, and the first of them would be `examples`, and a whole
// smoke matrix would build whatever directory happened to share that name.
//
// Chosen for being a character no path holds rather than for being pretty. A name
// that cannot be read back is a name that cannot be acted on.
const AxisSeparator = "!"

// MatrixName is the name a suite expanded from a matrix gets: the axes, joined
// with AxisSeparator, in the order they were declared.
//
// The value rather than the index, so a name in a report is something a person
// can read back to a cell of the matrix and act on. An index is shorter and says
// nothing until you count.
func MatrixName(base string, axis, value string, more ...string) string {
	parts := append([]string{base, axis + "=" + value}, more...)
	return strings.Join(parts, AxisSeparator)
}

// SortedTags returns a suite's tags in a stable order, so two runs of the same
// manifest report the same suite the same way and a diff between two reports is
// about what changed rather than about iteration order.
func SortedTags(tags []string) []string {
	out := append([]string{}, tags...)
	sort.Strings(out)
	return out
}
