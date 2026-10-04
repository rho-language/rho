//
// @file report.go
// @brief One run: the compilation database, the engines, and what came back.
//
// This is the file that decides what a run *is*, and there are only three decisions
// in it, each of which has a wrong answer that looks fine:
//
// What gets read. The compilation database first if a build wrote one, and derived
// from the manifest if nothing did — and the run says which, every time, in the
// output and in the SARIF file. Not because the distinction is subtle but because a
// finding from an analyzer that was given a guessed command line is a weaker claim
// than one from an analyzer that was given the real one, and a report that flattens
// the two is a report that will be believed once too often.
//
// Which engines run. Whatever was asked for, and every one of them rather than
// stopping at the first failure. A machine whose clang-tidy is not installed still
// gets cppcheck's findings and the two built-in engines', because a run that
// aborted on one missing analyzer would find nothing on every machine that is
// part-way through being set up.
//
// What a run's exit code means. Findings at or above a threshold fail it; an engine
// that did not work does not — unless every engine failed, because a pipeline that
// cannot tell "your code is clean" from "nothing was installed" will eventually
// report the second one as the first.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/targetspec"
)

// Options is what a run is asked for.
type Options struct {
	// Root is the project directory. Empty means this one.
	Root string
	// Target is the board, as its target spec names it. It is what chooses the
	// compiler in a derived database and which of several CMake build directories is
	// read, and both matter: they are the flags the analyzers are told.
	Target string
	// Manifest is the project's own. Read by the caller rather than here, because
	// the caller has to read it anyway to work out what the project is.
	Manifest *deps.Manifest
	// TargetSpec is the board's resolved spec, which carries the compiler and the
	// board's own defines. Nil for a project that targets this machine.
	TargetSpec *targetspec.Target
	// Engines are the engine names to run. Empty means every engine available.
	Engines []string
	// Rules are the checks to run, when the caller named any. Empty means all of
	// them, per engine.
	Rules []string
	// Database is the compilation database to read, as the settings name it. Empty
	// means the places this component looks.
	Database string
	// Exclude are the path fragments that are never read.
	Exclude []string
	// Outside is `exclude` or `include` for a source outside the project's own
	// directory.
	Outside string
	// FailOn is the severity at which the run fails.
	FailOn Severity
	// Write is where to put a derived compilation database, when a run is asked to
	// write one.
	//
	// Writing is opt-in because it changes the project rather than only reading it,
	// and a `rho lint` in a person's working tree should not create files. A run
	// still *derives* one in memory whether this is set or not: an analyzer given no
	// database finds nothing, and a run that found nothing because it had nowhere to
	// point is not a run that worked.
	Write string
	// Overwrite allows writing over a database a build already wrote.
	Overwrite bool
	// ExtraArgs are arguments passed to every engine.
	ExtraArgs []string
	// TreePlugins runs the plugins this project's own tree carries.
	//
	// Off by default, and the default is the answer rather than an omission: a plugin
	// is a program, and one found in a tree came from whoever wrote that tree — which
	// for a repository that was just cloned is a stranger. An installed plugin was
	// published by somebody the user chose to install.
	//
	// This is the flag a person passes to work on the plugins in `modules/plugins`,
	// which is the same decision rho-doc and rho-target make and the same flag.
	TreePlugins bool
}

// Result is what a run found.
type Result struct {
	// Diagnostics is every engine's findings, sorted and with the duplicates taken
	// out, each attributed to the engine that found it.
	Diagnostics []Diagnostic
	// Counts is what that list holds.
	Counts Counts
	// Runs is each engine's own findings in engine order, which is what SARIF wants:
	// one run per tool.
	Runs []Run
	// Engines is what each engine did, including the plugins that could not be used.
	Engines []Outcome
	// Database is the compilation database the run read or derived.
	Database Database
	// Units are the sources that were read, after the exclusions.
	Units []Unit
	// LeftOut is how many sources the exclusions dropped.
	LeftOut int
	// Failed is whether the run failed, which is the threshold and the engines that
	// did not work together rather than either alone.
	Failed bool
	// Reason says why, for the run's own last line.
	Reason string
}

// Outcome is what one engine did.
//
// The three cases are kept apart because a report that cannot tell them is worse
// than useless: an engine that found nothing, an engine that was pointed at nothing,
// and an engine that did not work are three different answers to "is this project
// clean", and only the first of them is yes.
type Outcome struct {
	// Description is what the engine is.
	Description Description
	// Findings is how many it reported.
	Findings int
	// Failed is whether it did not work, which is not the same as it found
	// something.
	Failed bool
	// Message is why it did not, when it did not.
	Message string
	// Ran is whether it was given anything to read at all.
	Ran bool
	// Sources is how many sources it was pointed at, which is what a report says
	// beside how many it found.
	//
	// Counted by the engine rather than recomputed by whoever writes the report,
	// because the engine narrowed the files to the languages it reads and the report
	// does not know which those were. A run whose engine read one source and found
	// nothing is a different answer from one whose engine read four hundred, and the
	// second number is what makes the first legible.
	Sources int
}

// Analyse is every engine over one compilation database.
//
// The order of the steps is the order a reader expects them in and is not rearranged
// for speed: resolve the board, resolve the database, resolve the sources, choose the
// engines, run them, merge. Each of those can fail with a message that is the whole
// answer, and each is done before the one that would make the message harder to read.
func Analyse(options Options) (*Result, error) {
	root := strings.TrimSpace(options.Root)
	if root == "" {
		root = "."
	}
	root = absolute(root)

	board := BoardFor(options.Manifest, options.Target)
	out := &Result{}

	database, err := resolveDatabase(root, options, board)
	if err != nil {
		return nil, err
	}
	out.Database = database

	units := sourcesFor(root, database)
	exclusions := Exclusions{Root: root, Fragments: options.Exclude, Outside: options.Outside}
	out.Units = exclusions.Filter(units)
	out.LeftOut = len(units) - len(out.Units)

	source := PluginSource(root)
	source.TrustTree = options.TreePlugins
	available := Discover(source)
	out.Engines = append(out.Engines, outcomesFor(available)...)

	specs, missing, err := available.Choose(options.Engines, nil, board)
	if err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		// Said before anything runs, and said as a note rather than a failure: the
		// engines that *are* here still run, and a person reading this wants to know
		// which half of the run they are looking at.
		out.Engines = append(out.Engines, Outcome{Message: fmt.Sprintf(
			"this machine has no engine called %s; this run is using %s",
			strings.Join(missing, ", "), strings.Join(available.Names(), ", "))})
	}

	request := Request{
		Root:            root,
		Database:        database.Path,
		DatabaseDerived: database.Derived,
		Units:           out.Units,
		Files:           FilesOf(out.Units, allLanguages(specs)),
		Rules:           options.Rules,
		ExtraArgs:       options.ExtraArgs,
	}
	for name := range GeneratedModules(root) {
		request.Generated = append(request.Generated, name)
	}
	for _, entry := range database.Entries {
		request.Defines = append(request.Defines, entry.Defines()...)
	}

	perEngine := make([][]Diagnostic, 0, len(specs))
	for _, spec := range specs {
		outcome, found := runOne(spec, request)
		out.Engines = append(out.Engines, outcome)
		perEngine = append(perEngine, found)
		out.Runs = append(out.Runs, Run{
			Tool:       toolFor(outcome.Description, found),
			Invocation: invocationFor(outcome),
			Results:    found,
		})
	}

	out.Diagnostics = Merge(perEngine)
	out.Counts = Of(out.Diagnostics)
	out.Failed, out.Reason = decide(options.FailOn, out)
	return out, nil
}

// resolveDatabase is the compilation database a run reads, and writes one on purpose
// when it is asked to.
func resolveDatabase(root string, options Options, board string) (Database, error) {
	path, where := Where(root, options.Database, board)
	if path != "" {
		if where != "" && strings.HasSuffix(where, "and not there") {
			return Database{}, fmt.Errorf("the compilation database is %s, %s", path, where)
		}
		database, err := ReadDatabase(path)
		if err != nil {
			return Database{}, err
		}
		database.From = where
		return database, nil
	}

	derived, err := Derive(DeriveRequest{
		Root:     root,
		Manifest: options.Manifest,
		Target:   options.TargetSpec,
	})
	if err != nil {
		return Database{}, err
	}
	derived.Path = derivedPath(root, options)
	if strings.TrimSpace(options.Write) == "" {
		return derived, nil
	}
	if err := WriteDatabase(derived.Path, derived, options.Overwrite); err != nil {
		return Database{}, err
	}
	return derived, nil
}

// derivedPath is where a derived database is written.
func derivedPath(root string, options Options) string {
	named := strings.TrimSpace(options.Write)
	if named == "" {
		return ""
	}
	if filepath.IsAbs(named) {
		return filepath.Clean(named)
	}
	return filepath.Join(root, filepath.FromSlash(named))
}

// sourcesFor is every source a run could read: the ones the database describes, and
// the project's own Rho sources.
//
// The Rho sources are not in the database and never will be, because nothing compiles
// a `.rho` file — the file the compiler reads is the one this toolchain translated
// out of it. So a run that wanted the Rho engine to have anything to read would have
// to be handed it, and the ordinary case would find no Rho sources at all.
func sourcesFor(root string, database Database) []Unit {
	units := database.UnitsOf()
	for _, path := range RhoSources(root) {
		units = append(units, Unit{File: path, Language: LanguageRho, From: "the project's own sources"})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].File < units[j].File })
	return units
}

// outcomesFor are the notes a run carries about the machine's plugins.
//
// Shown before any finding, because they are about the machine and everything else
// is about the code: a plugin whose analyzer is not installed is a fact somebody can
// act on, and it is easier to act on when it is the first thing in the output rather
// than the last.
func outcomesFor(available Engines) []Outcome {
	out := []Outcome{}
	for _, problem := range available.Problems {
		out = append(out, Outcome{Failed: true, Message: problem})
	}
	if len(available.Sources) > 0 {
		out = append(out, Outcome{Message: "engines asked from: " + strings.Join(available.Sources, ", ")})
	}
	return out
}

// runOne is one engine, its findings, and what went wrong with it.
func runOne(spec Spec, request Request) (Outcome, []Diagnostic) {
	description := describe(spec, request)
	sources := len(FilesOf(request.Units, description.Languages))
	if sources == 0 {
		return Outcome{Description: description}, nil
	}
	engine, err := build(spec, request)
	if err != nil {
		return Outcome{Description: description, Failed: true, Message: err.Error()}, nil
	}
	description = engine.Describe()
	found, err := engine.Run(request)
	if err != nil {
		return Outcome{
			Description: description, Sources: sources,
			Failed: true, Message: err.Error(),
		}, nil
	}
	return Outcome{
		Description: description, Sources: sources,
		Findings: len(found), Ran: true,
	}, found
}

// describe is what a spec says about its engine, resolved and expanded.
func describe(spec Spec, request Request) Description {
	out := Description{
		Name:      spec.Name,
		Kind:      spec.Kind,
		Languages: spec.ReadLanguages(),
		Source:    SourceBuiltin,
		Comment:   spec.Comment,
		Version:   spec.Version,
		Tool:      spec.Tool,
	}
	if spec.Kind == KindProgram {
		program, arguments := spec.Expansion(request)
		out.Program = program
		out.Arguments = arguments
		out.Format = spec.ReadFormat()
		out.Source = SourcePlugin
		out.Rules = spec.Rules
	}
	return out
}

// build is the engine a spec describes.
func build(spec Spec, request Request) (Engine, error) {
	if spec.Kind == KindProgram {
		return NewProgram(spec, request)
	}
	if engine, ok := NewDriver(spec.Name); ok {
		return engine, nil
	}
	return nil, fmt.Errorf(
		"the engine %q is not one this component carries and names no program to run", spec.Name)
}

// allLanguages is every language the chosen engines read, which is what a request's
// file list holds.
func allLanguages(specs []Spec) []string {
	wanted := map[string]bool{}
	for _, spec := range specs {
		for _, language := range spec.ReadLanguages() {
			wanted[language] = true
		}
	}
	out := make([]string, 0, len(wanted))
	for language := range wanted {
		out = append(out, language)
	}
	sort.Strings(out)
	return out
}

// decide is whether a run failed, and why in one sentence.
//
// Three inputs, weighed in this order, and the order is the design. An engine that
// did not work, when no engine did, is checked before the threshold is even read: a
// run where nothing ran has found nothing, and reporting that as a clean tree is the
// one wrong answer this file exists to prevent. One engine failing out of four is not
// a failure — the other three ran and their findings are real.
func decide(threshold Severity, out *Result) (bool, string) {
	attempted, succeeded := 0, 0
	for _, outcome := range out.Engines {
		if outcome.Description.Name == "" {
			continue
		}
		attempted++
		if !outcome.Failed {
			succeeded++
		}
	}
	if attempted > 0 && succeeded == 0 {
		return true, fmt.Sprintf("none of the %s could run, so nothing here has been checked",
			Plural(attempted, "engine"))
	}
	// Counted here rather than read off the result, because the threshold and the
	// findings are the two inputs to this decision and a caller that has both should
	// not have to also remember to keep a third field consistent with them.
	counts := Of(out.Diagnostics)
	if counts.AtLeast(threshold) {
		return true, fmt.Sprintf("%s at or above %s", Plural(counts.Total(), "finding"), threshold)
	}
	for _, outcome := range out.Engines {
		if outcome.Description.Name != "" && outcome.Ran {
			continue
		}
		if outcome.Description.Name != "" && !outcome.Failed {
			return false, fmt.Sprintf("nothing at or above %s, though %s was given nothing to read",
				threshold, outcome.Description.Name)
		}
	}
	return false, "nothing at or above " + string(threshold)
}

// toolFor is a description as the driver of a SARIF run.
func toolFor(description Description, found []Diagnostic) Tool {
	properties := map[string]string{PropertySource: description.Source}
	if description.Format != "" {
		properties[PropertyFormat] = description.Format
	}
	if description.Program != "" {
		properties[PropertyProgram] = description.Program
	}
	tool := Tool{
		Name:           description.Name,
		Version:        description.Version,
		InformationURI: description.Tool,
		Rules:          description.Rules,
		Properties:     properties,
	}
	if len(tool.Rules) == 0 && len(found) > 0 {
		// A run's rules are whatever its findings mention when the engine says
		// nothing about them, so a consumer can group and suppress by name even from
		// an engine that publishes no rule list of its own.
		tool.Rules = rulesMentioned(found)
	}
	return tool
}

// invocationFor is what an engine did, as a SARIF run records it.
func invocationFor(outcome Outcome) Invocation {
	return Invocation{
		CommandLine: commandLine(outcome.Description),
		// Zero whatever the findings were. An analyzer that found something and
		// exited non-zero has not failed, and a SARIF file that recorded that as a
		// failure would tell every consumer the tool is broken every time it works.
		ExitCode:   exitFor(outcome),
		Successful: !outcome.Failed,
		Message:    outcome.Message,
	}
}

func exitFor(outcome Outcome) int {
	if outcome.Failed {
		return 1
	}
	return 0
}

// rulesMentioned are the rules a set of findings names, with the worst severity each
// was seen at.
func rulesMentioned(found []Diagnostic) []Rule {
	worst := map[string]Severity{}
	for _, one := range found {
		known, ok := worst[one.Rule]
		if !ok || one.Severity.Rank() < known.Rank() {
			worst[one.Rule] = one.Severity
		}
	}
	out := make([]Rule, 0, len(worst))
	for id, severity := range worst {
		out = append(out, Rule{ID: id, Severity: severity})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// commandLine is what an engine was run with, as one line.
func commandLine(description Description) string {
	if description.Program == "" {
		return ""
	}
	return strings.Join(append([]string{description.Program}, description.Arguments...), " ")
}

// WriteText is a run's findings as a person reads them.
//
// The gcc spelling for the findings themselves and a summary underneath, because the
// two answer different questions: the lines say what is wrong and the summary says
// whether the run means it. A report with no summary makes a person count the lines,
// and a report with no finding lines makes them scroll.
func WriteText(w io.Writer, out *Result) error {
	for _, one := range out.Diagnostics {
		if _, err := fmt.Fprintln(w, one.String()); err != nil {
			return err
		}
	}
	if out.Counts.Total() == 0 {
		_, err := fmt.Fprintf(w, "No findings. %s\n", Sentence(Summarise(out)))
		return err
	}
	_, err := fmt.Fprintf(w, "%s in %s.\n", out.Counts.SeverityText(), filesCount(out))
	return err
}

// WriteSummary is the notes a run carries, which a report prints above its findings
// when there are any.
//
// Separate from WriteText because a pipeline that wants only the findings should not
// have to filter the notes out of them, and because the two are answered to different
// readers: the notes are about the machine, and the findings are about the code.
func WriteSummary(w io.Writer, out *Result) error {
	if out.Database.Derived && out.Database.Path != "" {
		if _, err := fmt.Fprintf(w, "%s: the compilation database was derived from %s\n",
			note("note"), out.Database.From); err != nil {
			return err
		}
	}
	if out.LeftOut > 0 {
		if _, err := fmt.Fprintf(w, "%s: %s left out, being excluded or outside this project\n",
			note("note"), Plural(out.LeftOut, "source")); err != nil {
			return err
		}
	}
	for _, outcome := range out.Engines {
		if outcome.Description.Name == "" {
			continue
		}
		switch {
		case outcome.Failed:
			if _, err := fmt.Fprintf(w, "%s: the engine %s: %s\n",
				note("warning"), outcome.Description.Name, outcome.Message); err != nil {
				return err
			}
		case outcome.Ran:
			if _, err := fmt.Fprintf(w, "%s: %s read %s, found %s\n",
				note("engine"), outcome.Description.Name,
				Plural(outcome.Sources, "source"), Plural(outcome.Findings, "finding")); err != nil {
				return err
			}
		default:
			if _, err := fmt.Fprintf(w, "%s: %s reads %s and there was none here\n",
				note("engine"), outcome.Description.Name,
				strings.Join(outcome.Description.Languages, " and ")); err != nil {
				return err
			}
		}
	}
	return nil
}

// note is the prefix a line of a run's notes carries.
//
// Bare, because every caller writes `"%s: "` after it. A prefix that already ended in
// a colon would print `engine:: rho`, which is the sort of thing that makes a report
// look like it was assembled rather than written.
func note(kind string) string { return kind }

func filesCount(out *Result) string {
	files := map[string]bool{}
	for _, one := range out.Diagnostics {
		if one.File != "" {
			files[one.File] = true
		}
	}
	return Plural(len(files), "file")
}

// Summarise is the sentence after a run with nothing in it.
//
// Deliberately longer than a finding count, because "no findings" needs more
// explaining than "3 warnings" and almost every case behind it is the run not having
// looked at what somebody assumed: the engines that were pointed at nothing, the
// sources the exclusions dropped, and the database being derived rather than read. A
// pipeline that got a silent success from a run with none of those in its output has
// been told nothing.
func Summarise(out *Result) string {
	parts := []string{}
	if out.Database.Derived {
		parts = append(parts, fmt.Sprintf(
			"the compilation database was derived from %s rather than read from a build, so the "+
				"flags these findings rest on are the manifest's and the target spec's",
			out.Database.From))
	} else if out.Database.Path != "" {
		parts = append(parts, "read from "+out.Database.Path)
	}
	if out.LeftOut > 0 {
		parts = append(parts, fmt.Sprintf("%s left out as excluded or outside this project",
			Plural(out.LeftOut, "source")))
	}
	if len(out.Units) == 0 {
		parts = append(parts, "there was nothing to read")
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%s ran over %s", Plural(len(out.Engines), "engine"), filesCount(out))
	}
	return strings.Join(parts, "; ") + "."
}

// Sentence is a summary as a sentence of its own, capitalised.
//
// A one-letter helper for one job, and the job is worth doing in one place: a
// report that reads "No findings. the compilation database was derived" is correct
// and reads as two halves of one line that somebody wrote badly, which is a poor
// first impression of the one line that tells a person whether to trust the rest.
//
// The *first* character, and not the first letter: "2 warnings" leads with a digit,
// and capitalising past it produces "2 Warnings", which is not a thing anybody
// writes.
func Sentence(summary string) string {
	if summary == "" {
		return summary
	}
	first := summary[0]
	if first < 'a' || first > 'z' {
		return summary
	}
	return string(first-'a'+'A') + summary[1:]
}
