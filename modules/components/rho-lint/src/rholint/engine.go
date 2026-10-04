//
// @file engine.go
// @brief What a lint run runs on: the contract an engine fills.
//
// An engine is not a checker. It is a thing that is handed a compilation database
// and answers with findings, and this component never looks inside one — which is
// the whole of the difference between a plugin and a fork.
//
// clang-tidy is not this component's checker. It is a checker this component knows
// how to run, and cppcheck is another, and both are better than anything written
// here. So an engine describes a *program* rather than an implementation, because
// a static analyzer this component has never heard of already exists as a program
// with its own flags and its own idea of a finding. Wrapping one in an API of ours
// would be a second implementation of something that works, and the one that would
// rot first.
//
// There are two kinds, and they differ only in where they run:
//
//   a driver   in this process, reading the sources itself. The two engines this
//              component carries are of this kind: `rho`, over the Rho sources
//              through the compiler's own parser, and `c`, over the C.
//   a program  another executable, in its own process, with the database handed to
//              it. These are what plugins contribute, because an analyzer that
//              already exists is already a program.
//
// A plugin may only contribute the second. An in-process driver would have to be
// compiled into this component, which is the thing being avoided, so a spec that
// claims to be one is refused rather than silently run as something else — the same
// rule rho-dbg applies to a debugger it cannot drive in process.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The hooks a plugin answers.
const (
	// HookEngine is the hook a plugin answers with the analyzers it provides.
	HookEngine = "engine"
	// HookReport is the hook a plugin answers with how a run's findings are
	// rendered.
	HookReport = "report"
)

// ComponentName is this component's own name, which is what a plugin has to say in
// order to be loaded by it.
const ComponentName = "rho-lint"

// The two kinds of engine.
const (
	// KindDriver is an engine in this process.
	KindDriver = "driver"
	// KindProgram is another program run and read from.
	KindProgram = "program"
)

// The formats a program's findings are read from.
//
// The list is closed on purpose. A format is a promise about a program this
// component will be handed output from and cannot see, and the promise is that
// every finding in it is read. An unknown format spelled in a manifest would be an
// analyzer whose findings silently vanish, which is the one failure a lint tool
// must not have: a finding that is not reported is indistinguishable from a defect
// that is not there.
const (
	// FormatGCC is `path:line:col: severity: message [-ruleright]`, which is what
	// clang and clang-tidy print and what most C tooling prints by default.
	FormatGCC = "gcc"
	// FormatCPPCheck is cppcheck's own XML report, read for the findings a C
	// analyzer reports that has no place: the unused function, the variable that
	// is read only on one path.
	FormatCPPCheck = "cppcheck"
	// FormatSARIF is SARIF 2.1.0, which most analyzers can already produce and
	// which this component reads by calling its own reader.
	FormatSARIF = "sarif"
	// FormatJSON is this component's own diagnostic shape, for a project that has
	// already normalized its analyzers and would rather not do it twice.
	FormatJSON = "json"
)

// Formats are the ones a program's output may be read from.
var Formats = []string{FormatGCC, FormatCPPCheck, FormatSARIF, FormatJSON}

// The two kinds of reporter.
const (
	// KindText is the human-readable report this component writes itself.
	KindText = "text"
	// KindSARIF is the machine-readable one, and the one a pipeline and a code
	// scanning service both read.
	KindSARIF = "sarif"
	// KindPlugin is a renderer a plugin provided.
	KindPlugin = "plugin"
)

// KnownFormat reports whether a format is one that can be read.
func KnownFormat(format string) bool {
	for _, known := range Formats {
		if known == format {
			return true
		}
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  The contract                                                               */
/* -------------------------------------------------------------------------- */

// Engine is something a run hands a compilation database to and reads findings
// back from.
//
// The interface is this small on purpose. Everything an engine needs is in one
// request, everything it has to say is a list of diagnostics, and the run above it
// knows nothing about how either was produced — which is what lets the last
// analyzer anybody writes be a program rather than a change to this component.
type Engine interface {
	// Describe says what this engine is, for a diagnostic a person reads when a
	// run finds nothing.
	Describe() Description

	// Run analyses what the request names and answers with what it found.
	Run(Request) ([]Diagnostic, error)
}

// Description is what an engine is, as a report and a diagnostic show it.
type Description struct {
	// Name is how the engine is chosen and what a finding is attributed to.
	Name string `json:"name"`
	// Kind is `driver` in this process or `program` run from another one.
	Kind string `json:"kind"`
	// Languages are the source languages it reads, so a report can say what it was
	// pointed at rather than only what it found.
	Languages []string `json:"languages,omitempty"`
	// Program is the executable, for an engine that is one.
	Program string `json:"program,omitempty"`
	// Arguments are what it is started with, after substitution. Reported because
	// "clang-tidy was run without a compilation database" is a finding about the
	// run rather than about the code, and the arguments are how anybody would see
	// it.
	Arguments []string `json:"arguments,omitempty"`
	// Format is how its output is read.
	Format string `json:"format,omitempty"`
	// Source says where it came from: `builtin` or the plugin that provided it.
	Source string `json:"source,omitempty"`
	// Comment is what the engine is for, which is what `rho lint engines` shows.
	Comment string `json:"comment,omitempty"`
	// Rules are the checks it carries, where it says.
	Rules []Rule `json:"rules,omitempty"`
	// Version is what is known about it, which for a plugin is what its spec says
	// and for a program on PATH is nothing.
	Version string `json:"version,omitempty"`
	// Tool is where the analyzer is documented, which is what a report links to.
	Tool string `json:"tool,omitempty"`
}

// The two places an engine can come from.
const (
	// SourceBuiltin is an engine this component carries.
	SourceBuiltin = "builtin"
	// SourcePlugin is an engine a plugin provided.
	SourcePlugin = "plugin"
)

// String is a description as one line, for a log and for the `engines` listing.
func (d Description) String() string {
	if d.Program == "" {
		return d.Name
	}
	return d.Name + " (" + d.Program + ")"
}

// Request is what an engine is asked to analyse.
//
// One request for the whole run rather than one per source, because that is how
// every analyzer is driven: clang-tidy is handed a database and works out which
// files to read, and calling it once per source would re-parse the database once
// per source for no gain.
type Request struct {
	// Root is the project directory every path is relative to.
	Root string `json:"root"`
	// Database is the compilation database's path, or "" when there is none.
	//
	// Empty is a real answer rather than a failure. A project with no generated C
	// has no database, and the engines that read Rho sources do not need one — so a
	// run must be able to say "here are the files" and go on.
	Database string `json:"database,omitempty"`
	// DatabaseDerived says the database was worked out from the manifest rather
	// than read from a build, and every finding from this run rests on it.
	DatabaseDerived bool `json:"database_derived,omitempty"`
	// Files are the sources to read, absolute, filtered by what the run excludes.
	Files []string `json:"files,omitempty"`
	// Units are the files with the language each is in, which is how an engine
	// that reads one language is given only that language.
	Units []Unit `json:"-"`
	// Defines are the macros a build of this project has, which a rule engine
	// without a compiler may want to know about.
	Defines []string `json:"defines,omitempty"`
	// Generated are the names of the modules this project's build generates, which a
	// source importing one uses without naming anything in it.
	Generated []string `json:"generated,omitempty"`
	// Rules are the checks to run, when the caller named any. Empty means all of
	// them.
	Rules []string `json:"rules,omitempty"`
	// ExtraArgs are arguments a caller passed through to every engine.
	ExtraArgs []string `json:"-"`
}

/* -------------------------------------------------------------------------- */
//*  Engines as plugins                                                         */
/* -------------------------------------------------------------------------- */

// Spec is one analyzer a plugin provides.
//
// A program rather than an implementation, for the reason at the top of this file.
type Spec struct {
	// Name is how the engine is chosen, and what every finding is attributed to.
	Name string `json:"name"`
	// Kind is `program`. It is the only kind a plugin may provide: an in-process
	// driver would have to be compiled into this component.
	Kind string `json:"kind"`
	// Languages are the source languages this engine reads. A run gives an engine
	// only the files in the languages it names, which is what keeps a C analyzer
	// from being handed a `.rho` file it cannot read.
	//
	// Empty means `c`, because that is what a static analyzer of generated C is
	// and an engine that named none has said nothing else.
	Languages []string `json:"languages,omitempty"`
	// Program is the executable. `${database}`, `${root}` and `${files}` in it are
	// filled in from the run, which is what lets one spec serve two projects.
	Program string `json:"program"`
	// Args are its arguments, with the same substitution.
	Args []string `json:"args,omitempty"`
	// Format is how its output is read. Empty means `gcc`, which is what clang and
	// clang-tidy print and what most C tooling prints by default.
	Format string `json:"format,omitempty"`
	// Targets are the boards this engine is for. Empty means it is for any.
	Targets []string `json:"targets,omitempty"`
	// Version is what a report and a SARIF file say about the analyzer. Declared
	// rather than run `--version` for, because a run that shells out to ask every
	// engine its version is a run that fails when one of them hangs.
	Version string `json:"version,omitempty"`
	// Tool is where the analyzer is documented, which is what a SARIF `helpUri` and
	// a report's link are.
	Tool string `json:"tool,omitempty"`
	// Rules are the checks it carries. Not required: an engine that says nothing
	// about its rules gets them assembled from what it found, which is enough for
	// a consumer to group and suppress by.
	Rules []Rule `json:"rules,omitempty"`
	// Comment is what this engine is for, shown by `rho lint engines`.
	Comment string `json:"comment,omitempty"`
	// Root is the directory the plugin lives in, filled in when the spec is read
	// from a plugin and not part of what a plugin sends.
	//
	// It is where the program is looked for before PATH, because a plugin is a
	// module with a directory of its own and a name is unambiguous inside it. A
	// plugin carrying its own wrapper and a project with two versions of an analyzer
	// installed is the case this exists for.
	Root string `json:"-"`
	// Qualified says a finding's rule name carries this engine's name in front of
	// it. On by default, because `unusedVariable` in one analyzer and
	// `unusedVariable` in another are two different checks and a project that
	// suppresses one has said nothing about the other.
	Qualified *bool `json:"qualified,omitempty"`
}

// Qualifies reports whether this engine's findings carry its name in the rule.
//
// A pointer rather than a bool because absent and false are different here, and
// only one of them is a claim: absent means "the safe default", false means
// "somebody decided these names are already unique".
func (s Spec) Qualifies() bool {
	if s.Qualified == nil {
		return true
	}
	return *s.Qualified
}

// Serves reports whether this engine is the one for a board.
//
// A named board is a claim about a board, and an engine that names none is a
// general one — which is what an analyzer of C is, since the flags come from the
// database rather than from the engine.
func (s Spec) Serves(board string) bool {
	if len(s.Targets) == 0 {
		return true
	}
	for _, named := range s.Targets {
		if named == board {
			return true
		}
	}
	return false
}

// ReadFormat is the format this engine's output is read from, defaulted.
func (s Spec) ReadFormat() string {
	format := strings.ToLower(strings.TrimSpace(s.Format))
	if format == "" {
		return FormatGCC
	}
	return format
}

// ReadLanguages are the languages this engine reads, defaulted.
func (s Spec) ReadLanguages() []string {
	if len(s.Languages) == 0 {
		return []string{LanguageC}
	}
	return s.Languages
}

// Expansion is the program and arguments an engine spec turns into for a run.
//
// Three substitutions, and the third is why this is not a string replacement:
//
//	`${database}`  the compilation database's path
//	`${root}`      the project directory
//	`${files}`     the sources, as as many arguments as there are files
//
// `${files}` expands to a *list*, because a linter takes its sources as separate
// arguments and there is no spelling of one argument that means several paths. A
// spec that wants the database only — which is most of them, clang-tidy among them
// — simply does not use it. A spec that tries to build one argument out of it is
// refused by Validate rather than quietly producing a single argument holding
// several paths.
func (s Spec) Expansion(run Request) (string, []string) {
	replace := func(value string) string {
		value = strings.ReplaceAll(value, "${database}", run.Database)
		return strings.ReplaceAll(value, "${root}", run.Root)
	}
	arguments := make([]string, 0, len(s.Args)+len(run.Files))
	for _, argument := range s.Args {
		if argument == FilesPlaceholder {
			arguments = append(arguments, run.Files...)
			continue
		}
		arguments = append(arguments, replace(argument))
	}
	return replace(s.Program), arguments
}

// FilesPlaceholder is the substitution that expands to every source of a run, and
// the whole of what an engine spec may substitute.
const FilesPlaceholder = "${files}"

// Validate is a spec that is usable.
//
// Checked before it is run rather than by whatever goes wrong first, because every
// failure below has a different message and a manifest is read once by somebody
// who is not looking at this code.
func (s Spec) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("an engine names nothing")
	}
	if strings.TrimSpace(s.Program) == "" {
		return fmt.Errorf("the engine %q names no program to run", s.Name)
	}
	if s.Kind != KindProgram && s.Kind != "" {
		return fmt.Errorf("the engine %q is a %q, and only a %q can be provided by a plugin",
			s.Name, s.Kind, KindProgram)
	}
	if !KnownFormat(s.ReadFormat()) {
		return fmt.Errorf("the engine %q writes %q, which cannot be read; this component reads %s",
			s.Name, s.Format, strings.Join(Formats, ", "))
	}
	for _, argument := range s.Args {
		if strings.Contains(argument, FilesPlaceholder) && argument != FilesPlaceholder {
			return fmt.Errorf(
				"the engine %q builds the argument %q out of %s, which stands for a list of files;\n"+
					"  write it as an argument of its own", s.Name, argument, FilesPlaceholder)
		}
	}
	return nil
}

// pluginSpecs reads the engines out of what a plugin answered.
func pluginSpecs(payload []byte) ([]Spec, error) {
	answer := struct {
		Engines []Spec `json:"engines"`
	}{}
	if err := json.Unmarshal(payload, &answer); err != nil {
		return nil, err
	}
	return answer.Engines, nil
}

/* -------------------------------------------------------------------------- */
/*  Reporters as plugins                                                       */
/* -------------------------------------------------------------------------- */

// ReportSpec is one renderer a plugin provides.
type ReportSpec struct {
	// Name is what `--format` is given to choose it.
	Name string `json:"name"`
	// Comment is what it is for, shown by `rho lint --help`.
	Comment string `json:"comment,omitempty"`
}

// ReportRequest is what a plugin is asked to render.
type ReportRequest struct {
	// Root is the project directory the paths are relative to.
	Root string `json:"root"`
	// Format is what the caller asked for.
	Format string `json:"format"`
	// Diagnostics are the run's findings, in the order a report shows them.
	Diagnostics []Diagnostic `json:"diagnostics"`
	// Counts is what the run found, for a renderer that leads with a summary.
	Counts Counts `json:"counts"`
	// Engines are the engines that ran, and how each went — which is what a
	// renderer needs to say "nothing found, and here is what looked".
	Engines []Description `json:"engines"`
	// Database is the compilation database the run read, or "".
	Database string `json:"database,omitempty"`
	// DatabaseDerived says it was worked out from the manifest rather than read
	// from a build.
	DatabaseDerived bool `json:"database_derived,omitempty"`
	// FailOn is the threshold, and Counts says whether it was crossed.
	FailOn string `json:"fail_on"`
	// Failed is whether the threshold was crossed.
	Failed bool `json:"failed"`
}
