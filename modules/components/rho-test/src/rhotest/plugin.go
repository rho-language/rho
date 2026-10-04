//
// @file plugin.go
// @brief The plugins that extend a test run.
//
// A test run is the one piece of a project's work that everybody has an opinion
// about: a CI system wants JUnit XML, a dashboard wants a JSON payload, a person
// wants a line on a terminal. None of that belongs in the runner, which knows
// how to run a test and nothing about how a result is reported. So the runner
// offers two hooks and lets a plugin answer them:
//
//   - discover, over the test files the run found, so a plugin that knows a
//     project's own test layout can add to them
//   - run, over the results of a finished run, so a plugin can report it in the
//     form a system reads
//
// Finding a plugin and running it are the toolchain's, in rho/src/plugins, so a
// plugin is written the same way whether it extends a test run or a
// documentation build.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhotest

import (
	"encoding/json"
	"fmt"

	"rho/src/plugins"
)

/* -------------------------------------------------------------------------- */
/*  What this component offers                                                */
/* -------------------------------------------------------------------------- */

// ComponentName is this component's own name, which a plugin has to say in order
// to be loaded by it. A component knows its own name; that is the one thing it
// does not have to be told.
const ComponentName = "rho-test"

// The hooks a run goes through, and so the hooks a plugin may answer. They are
// declared in the component's manifest as well, because that is what a plugin is
// validated against; the test beside this file checks the two agree.
const (
	// HookDiscover is the hook a plugin answers with the test files a run
	// should also run.
	HookDiscover = "discover"
	// HookRun is the hook a plugin answers with the results of a finished run.
	HookRun = "run"
)

// OfferedHooks are the hooks this runner runs.
var OfferedHooks = []string{HookDiscover, HookRun}

// offers reports whether this runner runs a hook at all. A run asks for nothing
// it does not run, whatever a plugin claims.
func offers(hook string) bool {
	for _, offered := range OfferedHooks {
		if offered == hook {
			return true
		}
	}
	return false
}

// Source is where this runner looks for the plugins it runs.
type Source = plugins.Source

// Plugin is a plugin attached to this component.
type Plugin = plugins.Plugin

// DiscoverPlugins reads the plugins that extend this runner.
func DiscoverPlugins(source Source) []Plugin {
	return source.Discover(ComponentName)
}

/* -------------------------------------------------------------------------- */
/*  What a plugin is asked                                                    */
/* -------------------------------------------------------------------------- */

// DiscoverRequest is what a plugin is asked about the tests a run found.
//
// A file is named by its path, and a test by its function name, file and line:
// the three things a report needs to point a reader at the code that failed.
//
// The declared suites are here too, which is the whole of what a plugin that
// runs somebody else's tests needs. A gtest suite is not a file this runner can
// read and build; it is a declaration, and the plugin that knows gtest is the one
// that can turn it into steps. It is asked over the declaration rather than left
// to walk the project, because a plugin that discovers by walking a directory
// runs whatever it finds rather than what the project said it has.
type DiscoverRequest struct {
	// Root is the project the run is over.
	Root string
	// Target is the backend the tests would run on.
	Target string
	// Files are the test files that were found, for the kinds this runner reads
	// itself. A plugin that only answers for a framework's own kinds ignores it.
	Files []DiscoveredFile
	// Suites are the declared suites a plugin may resolve, after selection. Each
	// carries the name it will be reported under and the kind that decides who
	// knows how to run it.
	Suites []DeclaredSuite
	// Manifest is where the declarations came from, for a plugin that reports a
	// problem against the file rather than against a suite.
	Manifest string
}

// DeclaredSuite is one declared suite as a plugin is asked about it.
//
// It repeats the fields of a Declared rather than sending that type, because a
// plugin is a separate program written against a version of this protocol and
// has no business knowing what the runner's internals are called.
type DeclaredSuite struct {
	// Name is what the suite will be reported as.
	Name string
	// Kind is what decides who runs it. A plugin answers for the kinds it knows
	// and leaves the rest alone, so several plugins may be asked about one run.
	Kind string
	// Tags are what the suite was tagged with.
	Tags []string
	// Axis is the value of each axis this suite was expanded from, by axis name.
	// A smoke suite built for one board reads its board here rather than having
	// the name parsed back apart.
	Axis map[string]string
	// Options is everything the manifest said about this suite that the runner
	// does not itself read.
	//
	// This is how a plugin's own settings reach it. The runner knows the keys every
	// kind shares and cannot know the ones a kind invented, so it hands them over
	// untouched: a suite whose kind is `gtest` carries whatever its `gtest:` mapping
	// said, and the plugin reads it as the plugin's shape. Without this, a manifest
	// could not describe a suite for a plugin the runner has never heard of, which
	// is the same as not being extensible.
	Options map[string]any
}

// DiscoveredFile is one test file and the cases in it.
type DiscoveredFile struct {
	Path  string
	Tests []Case
}

// DiscoverResponse is what a plugin answers the discover hook with.
//
// Both fields are optional and mean different things. Suites are ones the plugin
// resolved and can run: steps, and whatever it will report about them. Files are
// ones it added to what the runner found, for the kinds the runner reads itself —
// a plugin that generates a Rho test file adds it there.
//
// The distinction is why a plugin can now add a test it knows how to build, which
// was the thing it could not do before: a file was named, and every named file
// went down the runner's own Rho path regardless of who had named it.
type DiscoverResponse struct {
	Files       []DiscoveredFile
	Suites      []ResolvedSuite
	Diagnostics []plugins.Diagnostic
}

// ResolvedSuite is a runnable suite a plugin produced for a declaration.
type ResolvedSuite struct {
	// Declared is the declaration this answers, by name. A plugin that answers for
	// a suite nobody declared is dropped rather than run, so a plugin cannot add
	// tests the project did not ask for.
	Declared string
	// Name is what the suite will be reported as. Empty means the declared name.
	Name string
	// Steps are how to run it, in order.
	Steps []PluginStep
	// Cases are what it is expected to contain, for a report that can name a
	// missing case rather than a suite that passed.
	Cases []PluginCase
	// Skip is why it will not be run, when it will not.
	Skip string
	// Diagnostics is what the plugin wants the run to report about the suite.
	Diagnostics []plugins.Diagnostic
}

// PluginStep is one step of a plugin's suite.
//
// Env is how a plugin that retrieved something says where it put it: the step
// that fetched GoogleTest hands the path to the step that configures the build
// through the environment, rather than the plugin writing the answer into the
// project's files or the project having to know the path in advance.
type PluginStep struct {
	Env     []string `json:"env,omitempty"`
	Name    string
	Dir     string
	Command []string
	// Produces names what the step leaves behind, which a later step reads. It is
	// how a framework's XML report gets from the run that wrote it to the plugin
	// that parses it, without either of them guessing at the other's file.
	Produces string
	Optional bool
}

// PluginCase is one case a plugin expects a suite to contain.
type PluginCase struct {
	Name string
	File string
	Line int
}

// RunRequest is what a plugin is asked about a finished run.
type RunRequest struct {
	// Root is the project that was tested.
	Root string
	// Target is the backend the tests ran on.
	Target string
	// Results are what the run produced, in the order the suites were run.
	//
	// Each result names the suite it came from and says whether it passed, failed
	// or was skipped. A reporter that only knows about passing and failing has to
	// guess what to do with the third, and every guess it can make puts a skipped
	// test somewhere it did not happen.
	Results []Result
	// Suites are the suites the run considered, so a report can say a suite was
	// skipped even though it produced no cases at all — which is what a suite
	// whose toolchain is not installed looks like.
	Suites []Suite
	// Passed and Total are the run's own count, so a plugin need not walk the
	// results to report a summary. Skipped is here for the same reason: it is a
	// count of its own, and folding it into the total is how a run that tested
	// nothing gets reported as a run that tested everything successfully.
	Passed  int
	Failed  int
	Skipped int
	Total   int
}

// RunResponse is what a plugin answers the run hook with: text it wants printed
// after the report, and anything it wants reported.
type RunResponse struct {
	// Report is what the plugin writes, which is its reason for existing: a
	// document a system reads, or a line a person reads.
	Report string
	// Notes are lines added to the runner's own output, which is how a plugin
	// says something about the run without replacing the report.
	Notes []string
	// Diagnostics is what the plugin wants the run to report.
	Diagnostics []plugins.Diagnostic
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                   */
/* -------------------------------------------------------------------------- */

// Plugins are the plugins a run is handed, or nil for a run without them.
type Plugins struct {
	found []Plugin
}

// WithPlugins returns a set of plugins for a run, which is how a caller passes
// the plugins it discovered to the runner.
func WithPlugins(found []Plugin) Plugins { return Plugins{found: found} }

// Len is how many plugins a run will answer to.
func (p Plugins) Len() int { return len(p.found) }

// DiscoverContext is what a run asks its plugins about.
type DiscoverContext struct {
	// Root is the project the run is over.
	Root string
	// Target is the backend the tests would run on.
	Target string
	// Files are the test files the runner found itself.
	Files []DiscoveredFile
	// Suites are the declared suites, after selection and expansion.
	Suites []DeclaredSuite
	// Manifest is where the declarations were read from.
	Manifest string
}

// Discover runs the discover hook and returns what the plugins resolved: the
// files they added, and the suites they can run.
//
// A plugin that fails is reported and what it would have contributed is left
// out. A broken extension must not cost a project the tests it has.
func (p Plugins) Discover(context DiscoverContext) DiscoverOutcome {
	outcome := DiscoverOutcome{Files: context.Files}
	if len(p.found) == 0 || !offers(HookDiscover) {
		return outcome
	}
	answer := plugins.RunHooks(p.found, ComponentName, HookDiscover, DiscoverRequest{
		Root:     context.Root,
		Target:   context.Target,
		Files:    context.Files,
		Suites:   context.Suites,
		Manifest: context.Manifest,
	})
	outcome.Diagnostics = append(outcome.Diagnostics, answer.Diagnostics...)

	seen := map[string]bool{}
	for _, file := range context.Files {
		seen[file.Path] = true
	}
	// What was declared, so a suite answered for a declaration that was not made
	// is dropped instead of run.
	declared := map[string]bool{}
	for _, suite := range context.Suites {
		declared[suite.Name] = true
	}

	for _, response := range decodePayloads[DiscoverResponse](answer.Payload) {
		for _, file := range response.Files {
			if file.Path == "" || seen[file.Path] {
				// A file the run already has is not added twice, and a plugin naming
				// a file with no path is a mistake reported rather than a file that
				// cannot be read.
				if file.Path == "" {
					outcome.Diagnostics = append(outcome.Diagnostics, plugins.Diagnostic{
						Level: "warning", Message: "a plugin answered the discover hook with a file that has no path"})
				}
				continue
			}
			seen[file.Path] = true
			outcome.Files = append(outcome.Files, file)
		}
		for _, suite := range response.Suites {
			outcome.Diagnostics = append(outcome.Diagnostics, suite.Diagnostics...)
			if !declared[suite.Declared] {
				// A plugin may run what a project declared and nothing else. One that
				// adds tests the manifest does not mention is a plugin deciding what
				// a project's tests are, which is the project's to decide.
				outcome.Diagnostics = append(outcome.Diagnostics, plugins.Diagnostic{
					Level:   "warning",
					Message: fmt.Sprintf("a plugin answered for suite %q, which the manifest does not declare", suite.Declared)})
				continue
			}
			outcome.Resolved = append(outcome.Resolved, suite)
		}
	}
	return outcome
}

// DiscoverOutcome is what the plugins said about what should run.
type DiscoverOutcome struct {
	// Files are the test files the run should cover: the ones it found, plus
	// whatever the plugins added.
	Files []DiscoveredFile
	// Resolved are the suites a plugin knows how to run.
	Resolved []ResolvedSuite
	// Diagnostics is what the plugins reported about themselves.
	Diagnostics []plugins.Diagnostic
}

// Run reports the results of a finished run to the plugins and returns what they
// want said: the reports they wrote, the notes they added, and what they want
// the run to report.
//
// A plugin that fails is reported and the rest still run: one broken extension
// must not cost a project the reports the others wrote.
func (p Plugins) Run(root, target string, suites []Suite, results []Result) RunOutcome {
	outcome := RunOutcome{}
	if len(p.found) == 0 || !offers(HookRun) {
		return outcome
	}
	counts := Summarize(suites, results)
	answer := plugins.RunHooks(p.found, ComponentName, HookRun, RunRequest{
		Root:    root,
		Target:  target,
		Suites:  suites,
		Results: results,
		Passed:  counts.Passed,
		Failed:  counts.Failed,
		Skipped: counts.Skipped,
		Total:   counts.Cases,
	})
	for _, response := range decodePayloads[RunResponse](answer.Payload) {
		if response.Report != "" {
			outcome.Reports = append(outcome.Reports, response.Report)
		}
		outcome.Notes = append(outcome.Notes, response.Notes...)
		outcome.Diagnostics = append(outcome.Diagnostics, response.Diagnostics...)
	}
	outcome.Diagnostics = append(outcome.Diagnostics, answer.Diagnostics...)
	return outcome
}

// RunOutcome is what the plugins said about a finished run.
type RunOutcome struct {
	// Reports are the documents plugins wrote, in the order the plugins
	// answered. A report is a plugin's reason for existing: a JUnit document, a
	// JSON payload, a summary for a dashboard.
	Reports []string
	// Notes are lines the plugins added to the run's own output.
	Notes []string
	// Diagnostics is what the plugins reported, about themselves or about the
	// run they were given.
	Diagnostics []plugins.Diagnostic
}

// decodePayloads reads what the plugins answered, one payload per plugin that
// answered at all. A payload that is not the shape a component expects is
// reported rather than dropped in silence: a plugin written against another
// version should say so.
func decodePayloads[T any](payloads []json.RawMessage) []T {
	out := make([]T, 0, len(payloads))
	for _, raw := range payloads {
		var value T
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}
		out = append(out, value)
	}
	return out
}
