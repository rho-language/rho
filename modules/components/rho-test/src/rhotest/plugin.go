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
// SPDX-License-Identifier: MIT
//

package rhotest

import (
	"encoding/json"

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
type DiscoverRequest struct {
	// Root is the project the run is over.
	Root string
	// Target is the backend the tests would run on.
	Target string
	// Files are the test files that were found.
	Files []DiscoveredFile
}

// DiscoveredFile is one test file and the cases in it.
type DiscoveredFile struct {
	Path  string
	Tests []TestCase
}

// DiscoverResponse is what a plugin answers the discover hook with: the files it
// adds, and anything it wants the run to report.
type DiscoverResponse struct {
	Files       []DiscoveredFile
	Diagnostics []plugins.Diagnostic
}

// RunRequest is what a plugin is asked about a finished run.
type RunRequest struct {
	// Root is the project that was tested.
	Root string
	// Target is the backend the tests ran on.
	Target string
	// Results are what the run produced, in the order the files were run.
	Results []TestResult
	// Passed and Total are the run's own count, so a plugin need not walk the
	// results to report a summary.
	Passed int
	Total  int
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

// Discover runs the discover hook and returns the test files the run should
// cover: the ones it found, plus whatever the plugins added.
//
// A plugin that fails is reported and the files it would have added are left
// out. A broken extension must not cost a project the tests it has.
func (p Plugins) Discover(root, target string, files []DiscoveredFile) ([]DiscoveredFile, []plugins.Diagnostic) {
	if len(p.found) == 0 || !offers(HookDiscover) {
		return files, nil
	}
	answer := plugins.RunHooks(p.found, ComponentName, HookDiscover, DiscoverRequest{
		Root:   root,
		Target: target,
		Files:  files,
	})
	added := decodePayloads[DiscoverResponse](answer.Payload)
	diagnostics := answer.Diagnostics
	seen := map[string]bool{}
	for _, file := range files {
		seen[file.Path] = true
	}
	for _, addedFiles := range added {
		for _, file := range addedFiles.Files {
			if file.Path == "" || seen[file.Path] {
				// A file the run already has is not added twice, and a plugin
				// naming a file with no path is a mistake reported rather than
				// a file that cannot be read.
				if file.Path == "" {
					diagnostics = append(diagnostics, plugins.Diagnostic{
						Level: "warning", Message: "a plugin answered the discover hook with a file that has no path"})
				}
				continue
			}
			seen[file.Path] = true
			files = append(files, file)
		}
		diagnostics = append(diagnostics, addedFiles.Diagnostics...)
	}
	return files, diagnostics
}

// Run reports the results of a finished run to the plugins and returns what they
// want said: the reports they wrote, the notes they added, and what they want
// the run to report.
//
// A plugin that fails is reported and the rest still run: one broken extension
// must not cost a project the reports the others wrote.
func (p Plugins) Run(root, target string, results []TestResult) RunOutcome {
	outcome := RunOutcome{}
	if len(p.found) == 0 || !offers(HookRun) {
		return outcome
	}
	passed := 0
	for _, result := range results {
		if result.Passed {
			passed++
		}
	}
	answer := plugins.RunHooks(p.found, ComponentName, HookRun, RunRequest{
		Root:    root,
		Target:  target,
		Results: results,
		Passed:  passed,
		Total:   len(results),
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

