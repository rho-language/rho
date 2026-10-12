//
// @file runner.go
// @brief A test run, end to end.
//
// The run is four questions in order, and each one has its own answer:
//
//	1. What does this project claim to have?   — the manifest
//	2. Which of those did this run ask for?    — the selection
//	3. Who knows how to run each of them?      — a built-in kind, or a plugin
//	4. What came of it?                        — the execution engine
//
// They were four questions in one function before, which is why a project written
// in C could not be tested: the third question had one answer and it assumed Rho.
// Splitting them is the rebase. Everything below is arrangement; the substance is
// in manifest.go, plan.go, execute.go and plugin.go.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhotest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"rho/src/deps"
	"rho/src/plugins"
)

/* -------------------------------------------------------------------------- */
/*  What a run was asked for                                                   */
/* -------------------------------------------------------------------------- */

// DefaultTarget is the backend a Rho test runs on unless another is asked for.
const DefaultTarget = "c"

// Options configure a test run.
type Options struct {
	// Root is the project the run is over. Empty resolves from Path.
	Root string
	// Path is a test file or a directory, for a run that names its tests rather
	// than reading a manifest. The two ways of asking are kept apart on purpose:
	// a project with a manifest is described by it, and a directory of Rho test
	// files is a run with no project to describe.
	Path string
	// Selection is what to run. Empty runs everything the manifest declares.
	Selection Selection
	// Target is the backend Rho tests run on.
	Target string
	// RhoBinary is the rho to build with. Empty falls back to RHO_BIN, then PATH.
	RhoBinary string
	// Plugins are the plugins the run hands its work to. A run without them is a
	// plain run, and says which kinds it had no runner for rather than pretending
	// those suites do not exist.
	Plugins Plugins
	// ReportDir is where a plugin's report is written. Empty writes it beside the
	// project's build directory, which is where a build already keeps what a run
	// produced.
	ReportDir string
	// Verbose prints each step as it is taken.
	Verbose bool
	// Echo prints a suite's output whether it passed or failed.
	Echo bool
	// DryRun reports what would run without running any of it.
	DryRun bool
	// List reports what the manifest declares and exits.
	List bool
	// Runner takes the steps. ExecRunner when nil.
	Runner Runner
	// Out is where the run reports. os.Stdout when nil.
	Out io.Writer
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                   */
/* -------------------------------------------------------------------------- */

// Run executes a test run and returns a process exit code: 0 when nothing failed.
func Run(options Options) int {
	report := RunSuite(options)
	if report.ExitCode() != 0 {
		return report.ExitCode()
	}
	return 0
}

// Report is a completed run, in the shape a caller needs to exit on it, print it
// or hand it to a plugin.
type Report struct {
	// Manifest is what the project declared.
	Manifest Manifest
	// Suites are what the run considered, in the order it ran them.
	Suites []Suite
	// Results are the case results.
	Results []Result
	// Summary is the count of all of it.
	Summary Summary
	// Diagnostics are things that were wrong but did not fail a suite.
	Diagnostics []string
	// Reports are the documents plugins wrote.
	Reports []string
	// Notes are lines plugins added to the run's own output.
	Notes []string
	// Seconds is how long the run took.
	Seconds float64
	// Selection is what the run was asked for.
	Selection Selection
	// Err is a failure of the run itself — a manifest that could not be read, a
	// project with none — as distinct from a test that failed. The two have
	// different exits in a person’s head even when the exit code is the same.
	Err error
}

// ExitCode is what the process should return.
func (r Report) ExitCode() int {
	if r.Err != nil {
		return 1
	}
	if !r.Summary.OK() {
		return 1
	}
	return 0
}

/* -------------------------------------------------------------------------- */
/*  The four questions                                                         */
/* -------------------------------------------------------------------------- */

// RunSuite does what Run does and hands back everything, which is what a caller
// needs when it wants to report the run itself rather than only its exit code.
func RunSuite(options Options) Report {
	out := options.Out
	if out == nil {
		out = os.Stdout
	}
	if options.Target == "" {
		options.Target = DefaultTarget
	}

	report := Report{Selection: options.Selection}

	// (1) What does this project claim to have?
	manifest, err := loadManifestFor(options)
	if err != nil {
		report.Err = err
		fmt.Fprintln(out, "rho test:", err)
		return report
	}
	report.Manifest = manifest

	if options.List {
		printManifest(out, manifest, options.Selection)
		return report
	}

	// (2) Which of those did this run ask for?
	declared := Expand(manifest, options.Selection)
	if len(declared) == 0 {
		fmt.Fprintf(out, "No suites matched %s.\n", options.Selection.Describes())
		if len(manifest.Declared) > 0 {
			printManifest(out, manifest, Selection{})
		}
		return report
	}

	if options.DryRun {
		for _, line := range DescribePlan(manifest, options.Selection) {
			fmt.Fprintln(out, line)
		}
		report.Suites = declaredSuites(declared, projectRootOf(options, manifest))
		return report
	}

	root := projectRootOf(options, manifest)

	// (3) Who knows how to run each of them?
	suites, problems := resolve(declared, root, manifest, options)
	report.Suites = suites
	for _, problem := range problems {
		report.Diagnostics = append(report.Diagnostics, problem.Error())
	}

	// (4) What came of it?
	executed := Execute(suites, ExecuteOptions{
		Root:    root,
		Runner:  options.Runner,
		Verbose: options.Verbose,
		Echo:    options.Echo,
	})
	report.Results = executed.Results
	report.Summary = executed.Summary
	report.Diagnostics = append(report.Diagnostics, executed.Diagnostics...)
	report.Seconds = executed.Seconds

	printReport(out, report, options)

	// A plugin is told what the run actually did rather than what it was asked to
	// do, which is the only thing a reporter can honestly write down.
	outcome := options.Plugins.Run(root, options.Target, report.Suites, report.Results)
	report.Reports = append(report.Reports, outcome.Reports...)
	report.Notes = append(report.Notes, outcome.Notes...)
	for _, diagnostic := range outcome.Diagnostics {
		report.Diagnostics = append(report.Diagnostics, diagnostic.String())
	}
	writeReports(out, options, root, outcome)
	return report
}

// loadManifestFor reads the project's declaration, or falls back to the tests it
// can find by itself.
func loadManifestFor(options Options) (Manifest, error) {
	if options.Root == "" {
		options.Root = DefaultRoot(options.Path)
	}
	manifest, err := LoadProjectManifest(options.Root)
	if err == nil {
		return manifest, nil
	}
	// A project with no manifest is not an error, provided it has Rho test files:
	// those are a description too, and the oldest one this runner has. A project
	// with neither is an error, because "nothing to run" and "nothing declared"
	// are different things.
	paths, findErr := Discover(orDefault(options.Path, options.Root))
	if findErr != nil || len(paths) == 0 {
		return Manifest{}, err
	}
	declared := []Declared{}
	absRoot, rootErr := filepath.Abs(options.Root)
	for _, path := range paths {
		// The suite keeps the file's path relative to the project, not just its
		// base name, because the runner resolves a declared Rho suite to the file
		// it names: a test under `tests/` is `tests/math`, and dropping the
		// directory would look for it beside the manifest and report it missing.
		// The discovery walks the path as given, which may be relative while the
		// root is absolute, so both are resolved before one is taken from the other.
		name := strings.TrimSuffix(filepath.Base(path), TestFileSuffix)
		if absPath, absErr := filepath.Abs(path); rootErr == nil && absErr == nil {
			if rel, relErr := filepath.Rel(absRoot, absPath); relErr == nil && !strings.HasPrefix(rel, "..") {
				name = strings.TrimSuffix(filepath.ToSlash(rel), TestFileSuffix)
			}
		}
		declared = append(declared, Declared{Name: name, Kind: KindRho, Tags: []string{KindRho}})
	}
	return Manifest{Name: filepath.Base(options.Root), Declared: declared}, nil
}

// resolve answers the third question: a built-in kind, or a plugin.
//
// A plugin is asked over every declaration at once rather than one at a time,
// because a plugin that has to build a model of the project to answer for one
// suite should be able to answer for all of them in one call — and because a
// plugin's answer is a statement about what it can run, not about one suite.
func resolve(declared []Declared, root string, manifest Manifest, options Options) ([]Suite, []error) {
	resolvers := BuiltinResolvers()

	// What a plugin can run, asked once.
	outcome := options.Plugins.Discover(DiscoverContext{
		Root:     root,
		Target:   options.Target,
		Files:    DescribeFiles([]string{}),
		Suites:   asDeclaredSuites(declared),
		Manifest: manifest.Source,
	})
	for _, diagnostic := range outcome.Diagnostics {
		fmt.Fprintln(os.Stderr, "rho test:", diagnostic)
	}
	byName := map[string]ResolvedSuite{}
	for _, suite := range outcome.Resolved {
		byName[suiteName(suite)] = suite
	}

	// Everything a plugin claimed becomes a resolver of its own kinds, so that
	// resolution stays one mechanism rather than two.
	if len(byName) > 0 {
		kinds := map[string]bool{}
		for _, entry := range declared {
			if _, ok := byName[entry.Name]; ok {
				kinds[entry.Kind] = true
			}
		}
		for kind := range kinds {
			kind := kind
			resolvers = append(resolvers, ResolverFunc{
				Kind: kind,
				Run: func(entry Declared, root string) (Suite, error) {
					resolved, ok := byName[entry.Name]
					if !ok {
						return Suite{}, fmt.Errorf("no plugin offered a runner for this %s suite", entry.Kind)
					}
					return fromResolved(resolved, entry, root), nil
				},
			})
		}
	}
	return ResolveAll(declared, root, resolvers...)
}

// fromResolved turns what a plugin answered into a runnable suite.
func fromResolved(resolved ResolvedSuite, entry Declared, root string) Suite {
	suite := Suite{
		Name: entry.Name,
		Kind: entry.Kind,
		Tags: entry.Tags,
		Root: root,
		Skip: resolved.Skip,
	}
	if resolved.Name != "" {
		suite.Name = resolved.Name
	}
	for _, step := range resolved.Steps {
		suite.Steps = append(suite.Steps, Step{
			Name:     step.Name,
			Dir:      step.Dir,
			Command:  step.Command,
			Env:      step.Env,
			Produces: step.Produces,
			Optional: step.Optional,
		})
	}
	for _, testCase := range resolved.Cases {
		suite.Cases = append(suite.Cases, Case{Name: testCase.Name, File: testCase.File, Line: testCase.Line})
	}
	return suite
}

func suiteName(resolved ResolvedSuite) string {
	if resolved.Name != "" {
		return resolved.Name
	}
	return resolved.Declared
}

// asDeclaredSuites is what a plugin is asked about the declarations: the fields
// of a Declared that a separate program should know about.
func asDeclaredSuites(declared []Declared) []DeclaredSuite {
	out := make([]DeclaredSuite, 0, len(declared))
	for _, entry := range declared {
		axes := map[string]string{}
		for _, axis := range entry.MatrixAxes() {
			axes[axis] = AxisValue(entry.Name, axis)
		}
		out = append(out, DeclaredSuite{
			Name:    entry.Name,
			Kind:    entry.Kind,
			Tags:    entry.Tags,
			Axis:    axes,
			Options: entry.Options,
		})
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Roots                                                                      */
/* -------------------------------------------------------------------------- */

// DefaultRoot is where a run is over when it was not told: the project that owns
// the path it was given.
func DefaultRoot(path string) string {
	if path == "" {
		working, err := os.Getwd()
		if err != nil {
			return "."
		}
		return working
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if info, err := os.Stat(absolute); err == nil && !info.IsDir() {
		return ProjectRoot(absolute)
	}
	return absolute
}

// ProjectRoot walks up from a path to the project that owns it, which is the
// directory holding a manifest.
func ProjectRoot(path string) string {
	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}
	absolute, err := filepath.Abs(dir)
	if err == nil {
		dir = absolute
	}
	for {
		for _, name := range []string{"module.yml", "rho.mod"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

func projectRootOf(options Options, manifest Manifest) string {
	if options.Root != "" {
		return options.Root
	}
	if options.Path != "" {
		return DefaultRoot(options.Path)
	}
	return DefaultRoot("")
}

func orDefault(path, fallback string) string {
	if path != "" {
		return path
	}
	return fallback
}

func declaredSuites(declared []Declared, root string) []Suite {
	out := make([]Suite, 0, len(declared))
	for _, entry := range declared {
		out = append(out, Suite{Name: entry.Name, Kind: entry.Kind, Tags: entry.Tags, Root: root, Skip: entry.Skip})
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Reporting                                                                  */
/* -------------------------------------------------------------------------- */

func printManifest(out io.Writer, manifest Manifest, selection Selection) {
	fmt.Fprintln(out, "project:", nameOr(manifest.Name, "(unnamed)"))
	if manifest.Source != "" {
		fmt.Fprintln(out, "manifest:", manifest.Source)
	}
	fmt.Fprintln(out, "selected:", selection.Describes())
	for _, suite := range Expand(manifest, selection) {
		labels := ""
		if len(suite.Tags) > 0 {
			labels = "  {" + strings.Join(SortedTags(suite.Tags), ",") + "}"
		}
		state := "run"
		if suite.Skip != "" {
			state = "skip: " + suite.Skip
		}
		fmt.Fprintf(out, "  %-40s %-10s %s%s\n", suite.Name, suite.Kind, labels, state)
	}
}

func printReport(out io.Writer, report Report, options Options) {
	for _, note := range report.Notes {
		fmt.Fprintln(out, note)
	}
	passed, failed, skipped := 0, 0, 0
	for _, result := range report.Results {
		switch {
		case result.Status == StatusPass:
			passed++
			if options.Verbose {
				fmt.Fprintf(out, "[PASS] %s\n", caseLabel(result))
			}
		case result.Status == StatusSkip:
			skipped++
			fmt.Fprintf(out, "[SKIP] %s  (%s)\n", caseLabel(result), result.Reason)
		default:
			failed++
			fmt.Fprintf(out, "[FAIL] %s\n", caseLabel(result))
			if result.Output != "" {
				fmt.Fprintf(out, "       %s\n", tailOf(result.Output, 16))
			}
		}
	}
	summary := report.Summary
	fmt.Fprintf(out, "\n%d suites, %d cases: %d passed, %d failed", summary.Suites, summary.Cases, passed, failed)
	if skipped > 0 {
		fmt.Fprintf(out, ", %d skipped", skipped)
	}
	if summary.FailedSuites > 0 {
		fmt.Fprintf(out, "\n%d of %d suites failed", summary.FailedSuites, summary.Suites)
	}
	if summary.SkippedSuites > 0 {
		fmt.Fprintf(out, "\n%d of %d suites skipped", summary.SkippedSuites, summary.Suites)
	}
	if report.Seconds > 0 {
		fmt.Fprintf(out, "\n%.1fs", report.Seconds)
	}
	fmt.Fprintln(out)
	for _, diagnostic := range report.Diagnostics {
		fmt.Fprintln(os.Stderr, "rho test:", diagnostic)
	}
}

func caseLabel(result Result) string {
	if result.Suite == "" {
		return result.Case.Name
	}
	return result.Suite + ": " + result.Case.Name
}

// writeReports puts what plugins wrote where a system will look for it.
//
// A report is a document a machine reads, so it is written to a file and the path
// printed: printed, it is a wall of text in the middle of a test run.
func writeReports(out io.Writer, options Options, root string, outcome RunOutcome) {
	for _, diagnostic := range outcome.Diagnostics {
		fmt.Fprintln(os.Stderr, "rho test:", diagnostic)
	}
	if len(outcome.Reports) == 0 {
		return
	}
	dir := options.ReportDir
	if dir == "" {
		dir = filepath.Join(root, "build", "test-reports")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "rho test: cannot write a report: %v\n", err)
		return
	}
	for index, report := range outcome.Reports {
		name := filepath.Join(dir, fmt.Sprintf("report-%d.txt", index+1))
		if err := os.WriteFile(name, []byte(report), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "rho test: cannot write %s: %v\n", name, err)
			continue
		}
		fmt.Fprintln(out, "report:", name)
	}
}

// tailOf is what a failure's output is cut down to.
//
// The end, not the beginning. A compiler names the file, the line and the reason
// at the end of what it printed, after everything it compiled successfully; a
// build log's first lines are the toolchain's version banner and the configure
// output. So a report that showed the head of a failing build showed the reader
// the least useful part of it, and the most useful part of a failure — which one
// of them it was, and why — was below the cut.
func tailOf(text string, limit int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) <= limit {
		return strings.Join(lines, "\n       ")
	}
	dropped := len(lines) - limit
	return fmt.Sprintf("... %d earlier lines\n       %s", dropped, strings.Join(lines[len(lines)-limit:], "\n       "))
}

/* -------------------------------------------------------------------------- */
/*  Plugins                                                                    */
/* -------------------------------------------------------------------------- */

// RhoHome is where an installation keeps what it installed, which is where a run
// looks for the plugins that extend it.
func RhoHome() string {
	return deps.RhoHome()
}

var _ = plugins.ProtocolVersion
