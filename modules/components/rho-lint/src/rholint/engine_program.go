//
// @file engine_program.go
// @brief An engine that is another program, run and read from.
//
// This is how a static analyzer this component has never heard of becomes part of a
// run. A linter that already exists is already a program with its own flags and its
// own output format, so a plugin contributes a *description* of it — a path, some
// arguments, and which of this component's output formats to read — and this file
// does the rest: start it, collect what it wrote, turn that into diagnostics, and
// say what happened when it could not be read.
//
// Which is the whole reason the format is a field on the spec rather than something
// each plugin re-implements. Every analyzer's output is normalized *here*, once, and
// the normalization is where the mistakes that matter live: a line number that is
// zero-based, a severity that means the opposite of what it looks like, a finding
// that gets dropped because its file was not one of the ones this run asked about.
// A plugin that gets this wrong by not doing it cannot, and a plugin that gets it
// wrong by writing a different format name is refused.
//
// Three details are worth stating because each is a bug this design is shaped
// around:
//
// A non-zero exit is not a failure. clang-tidy exits 1 when it finds something and
// cppcheck exits 1 when it finds something, which is most of what they do. Treating
// that as an error would turn every productive run into a failed one, so the exit
// code decides whether the *engine* worked and never whether it found anything.
//
// Output that cannot be read is a failure, not an empty result. An analyzer that
// printed something this component has no reader for would otherwise be reported as
// having found nothing, and a lint run that reports a clean tree because it could not
// read its own output is the worst failure this tool has.
//
// Standard error is kept and reported. "error: no compilation commands found" is the
// entire answer to why a run found nothing, and it arrives on the stream a program
// writes it to rather than on the one its findings come out of.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Program is an engine that is another executable.
//
// It implements the same Engine interface as the two drivers and knows nothing about
// static analysis: it starts a program, reads what the program wrote, and produces
// the same diagnostics every other engine produces. That is what lets an analyzer
// nobody has heard of join a run without anything above it changing.
type Program struct {
	spec    Spec
	run     Request
	timeout time.Duration
}

// DefaultTimeout is how long an engine is given before the run carries on without
// it.
//
// Long enough for a full-project analysis on a cross-compiled tree — which is the
// slow case, because clang-tidy parses a vendor's HAL — and bounded because an
// analyzer that never finishes must not hold a pipeline open forever. A tool that
// takes longer than this is reported as timed out rather than silently producing
// nothing.
const DefaultTimeout = 10 * time.Minute

// NewProgram is an engine that is the program a spec names.
func NewProgram(spec Spec, run Request) (Engine, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return &Program{spec: spec, run: run, timeout: DefaultTimeout}, nil
}

// Describe is this engine as a report shows it.
//
// The program and arguments are resolved and expanded here rather than at run time,
// because a listing is where somebody compares what a run *would* do against what
// they expected — and a run that resolved them differently would be a mystery.
func (p *Program) Describe() Description {
	program, arguments := p.spec.Expansion(p.run)
	return Description{
		Name:      p.spec.Name,
		Kind:      KindProgram,
		Languages: p.spec.ReadLanguages(),
		Program:   program,
		Arguments: arguments,
		Format:    p.spec.ReadFormat(),
		Source:    SourcePlugin,
		Comment:   p.spec.Comment,
		Rules:     p.spec.Rules,
		Version:   p.spec.Version,
		Tool:      p.spec.Tool,
	}
}

// Run starts the program and reads its findings.
func (p *Program) Run(run Request) ([]Diagnostic, error) {
	run = mergeRequests(p.run, run)
	program, arguments := p.spec.Expansion(run)

	resolved, err := ResolveProgram(program, p.spec.Root)
	if err != nil {
		return nil, fmt.Errorf("the engine %q: %w", p.spec.Name, err)
	}

	process := exec.Command(resolved, arguments...)
	// In the project directory, because every path an analyzer reports and every
	// path it was given is relative to it, and a program started somewhere else
	// resolves those against somewhere else.
	process.Dir = run.Root
	var out, complaints bytes.Buffer
	process.Stdout = &out
	process.Stderr = &complaints

	if err := process.Start(); err != nil {
		return nil, fmt.Errorf("the engine %q could not be started: %w", p.spec.Name, err)
	}

	done := make(chan error, 1)
	go func() { done <- process.Wait() }()

	var waitErr error
	timedOut := false
	select {
	case waitErr = <-done:
	case <-time.After(p.timeout):
		timedOut = true
		// A linter parsing a vendor's HAL can genuinely take minutes, so this is
		// the last resort and not a politeness timeout. The process is killed rather
		// than left, because a run that carries on leaves an analyzer competing with
		// whatever the pipeline does next.
		if process.Process != nil {
			_ = process.Process.Kill()
		}
		waitErr = <-done
	}

	if timedOut {
		return nil, fmt.Errorf("the engine %q did not finish within %s", p.spec.Name, p.timeout)
	}

	found, err := p.read(out.Bytes())
	if err != nil {
		// The analyzer's own complaints are part of the answer and are never lost:
		// "no compilation commands found" on stderr with a parse failure on stdout is
		// one problem, not two, and the second message alone would send somebody
		// looking for a malformed report.
		return nil, fmt.Errorf("the engine %q could not be read: %w%s",
			p.spec.Name, err, sayMore(complaints.String()))
	}

	// A non-zero exit with output this component could read is not a failure: it is
	// an analyzer that found something. A non-zero exit with *nothing* readable is
	// a failure, because an analyzer that ran and found nothing says so.
	if waitErr != nil && len(found) == 0 {
		return nil, fmt.Errorf("the engine %q exited with %s and reported nothing%s",
			p.spec.Name, waitErr, sayMore(complaints.String()))
	}
	return found, nil
}

// sayMore is an analyzer's own output as a sentence to append to a message.
//
// Prefixed rather than concatenated so that a multi-line diagnostic stack from a
// crashed analyzer reads as a diagnostic from that analyzer and not as part of this
// component's sentence.
func sayMore(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	return "\n  it said: " + strings.ReplaceAll(trimmed, "\n", "\n         ")
}

// read turns what a program wrote into diagnostics.
func (p *Program) read(output []byte) ([]Diagnostic, error) {
	switch p.spec.ReadFormat() {
	case FormatGCC:
		return ReadGCC(output, p.spec, p.run.Root), nil
	case FormatCPPCheck:
		return ReadCPPCheck(output, p.spec, p.run.Root)
	case FormatSARIF:
		return ReadSARIF(output)
	case FormatJSON:
		return ReadJSON(output, p.spec, p.run.Root)
	default:
		return nil, fmt.Errorf("%q is not a format this component reads", p.spec.ReadFormat())
	}
}

// mergeRequests is the spec's own request with the caller's over it.
//
// A plugin's spec is bound to a run when the engine is created — the run was known
// then, and the spec's expansion has to be stable — while a caller may still narrow
// it. The two are merged rather than one replacing the other so that a caller
// narrowing the files does not also drop the root and the database the spec was
// built against.
func mergeRequests(bound Request, run Request) Request {
	out := bound
	if run.Root != "" {
		out.Root = run.Root
	}
	if run.Database != "" {
		out.Database = run.Database
	}
	if run.DatabaseDerived {
		out.DatabaseDerived = true
	}
	if len(run.Files) > 0 {
		out.Files = run.Files
	}
	if len(run.Units) > 0 {
		out.Units = run.Units
	}
	if len(run.Rules) > 0 {
		out.Rules = run.Rules
	}
	out.Defines = append(append([]string{}, bound.Defines...), run.Defines...)
	return out
}

/* -------------------------------------------------------------------------- */
/*  Reading output                                                             */
/* -------------------------------------------------------------------------- */

// ReadGCC is findings in the gcc spelling: `path:line:col: severity: text [-rule]`.
//
// The format of clang, of clang-tidy with no `--output` flag, and of most C tooling
// that prints diagnostics at all. Parsed as a whole-line pattern rather than by
// splitting on colons because the parts themselves contain colons — a C++ scope
// resolution, a Windows drive letter, and the `-W` name in the trailing bracket.
//
// The column is optional because GCC omits it for diagnostics with no column, and
// requiring it would drop every such finding. The rule is optional because a plain
// compiler warning has none.
func ReadGCC(output []byte, spec Spec, root string) []Diagnostic {
	pattern := regexp.MustCompile(
		`^(?P<file>[^:\n]+(?:[:/][^:\n]*)*?):(?P<line>\d+)(?::(?P<column>\d+))?:\s*` +
			`(?P<severity>fatal error|error|warning|note|remark):\s*(?P<message>.*)$`)

	found := []Diagnostic{}
	for _, line := range splitLines(output) {
		parts := pattern.FindStringSubmatch(line)
		if parts == nil {
			continue
		}
		severity := gccSeverity(parts[pattern.SubexpIndex("severity")])
		if severity == SeverityNone {
			// `remark` is gcc's word for a note that is not a diagnostic. It is kept
			// as a note rather than dropped: it is what `-fdiagnostics-show-option`
			// prints to say which flag turned a warning off, and it is the answer to
			// "why is this not being reported".
			severity = SeverityNote
		}
		message := parts[pattern.SubexpIndex("message")]
		// The bracket is cut off by *position*, not by re-joining the rule's name:
		// `trailingRule` strips the leading `-` off a `-W` spelling, so trimming
		// `"[" + rule + "]"` would not match and the bracket would stay in the
		// sentence of every diagnostic clang printed.
		rule, suffix := trailingRule(message)
		if suffix > 0 {
			message = strings.TrimSpace(message[:suffix])
		}
		found = append(found, normalise(Diagnostic{
			Rule:     qualify(spec, rule),
			Message:  message,
			Severity: severity,
			File:     Relative(parts[pattern.SubexpIndex("file")], root),
			Line:     atoi(parts[pattern.SubexpIndex("line")]),
			Column:   atoi(parts[pattern.SubexpIndex("column")]),
			Engine:   spec.Name,
		}))
	}
	return found
}

// gccSeverity is a gcc severity word as one of the three here.
//
// `fatal error` maps to an error rather than to a severity of its own: it means the
// compilation stopped, which is the same thing to a reader as an error that will
// stop the build.
func gccSeverity(word string) Severity {
	switch strings.ToLower(strings.TrimSpace(word)) {
	case "fatal error", "error":
		return SeverityError
	case "warning":
		return SeverityWarning
	default:
		return SeverityNone
	}
}

// trailingRule is the `[-Wsomething]` at the end of a gcc diagnostic, and where in
// the message it begins.
//
// The offset is returned as well as the name because the name is normalised — the
// leading `-` of a `-W` spelling is the flag, not the check's name, and keeping it
// would put `clang-tidy--Wunused-variable` in a report, which no existing
// suppression matches and nobody can type — and a caller that trimmed by the
// normalised name would leave the bracket in the sentence.
//
// A return of 0 means there is no rule, and also that the caller must not cut
// anything off the message.
func trailingRule(message string) (string, int) {
	if !strings.HasSuffix(message, "]") {
		return "", 0
	}
	open := strings.LastIndex(message, "[")
	if open < 0 {
		return "", 0
	}
	candidate := message[open+1 : len(message)-1]
	// Only a `-W` name or a bare identifier, and only when it is the whole bracket
	// group: a message ending in `[3]` is an array subscript in prose, not a rule.
	if strings.TrimSpace(candidate) == "" {
		return "", 0
	}
	for _, character := range candidate {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '-', character == '_', character == '.', character == '+':
		default:
			return "", 0
		}
	}
	if dash, ok := strings.CutPrefix(candidate, "-"); ok {
		candidate = dash
	}
	return candidate, open
}

// qualify is an analyzer's rule name as a finding records it.
//
// Prefixed with the engine's name unless the spec says otherwise, and the prefix is
// not decoration: `unusedVariable` and `unusedFunction` are different checks in
// different analyzers, and a project that suppresses one of them has said nothing
// about the other. A project that genuinely wants them merged can say so in its
// plugin's manifest, which is a decision somebody makes on purpose rather than one
// this component makes for everybody.
func qualify(spec Spec, rule string) string {
	if rule == "" {
		return ""
	}
	if !spec.Qualifies() || strings.HasPrefix(rule, spec.Name+"-") {
		return rule
	}
	return spec.Name + "-" + rule
}

// ReadCPPCheck is findings in cppcheck's own XML report.
//
// A struct rather than a map, because cppcheck's severities are spelled its own way
// and the mapping has to be written down rather than looked up. The two that need a
// decision are:
//
//	`information` is a note. cppcheck uses it for "unused function" and for the
//	"style" checks, which are worth seeing and not worth failing a build over.
//
//	`debug` and `internal` are dropped. They are the analyzer talking to itself
//	about its own analysis, not a statement about the code, and a run that reported
//	them would drown a person in a page of nothing.
//
// The `<error>` element's `file` may name a file the run never asked about — the
// project-wide checks report against the configuration rather than against a source
// — and it is kept, with the file as given, rather than dropped: an unused-function
// finding is the one most worth acting on and it is the one most often reported this
// way.
func ReadCPPCheck(output []byte, spec Spec, root string) ([]Diagnostic, error) {
	document := cppcheckReport{}
	if err := xmlUnmarshal(output, &document); err != nil {
		return nil, fmt.Errorf("this is not a cppcheck report: %w", err)
	}
	found := []Diagnostic{}
	for _, item := range document.findings() {
		severity := cppcheckSeverity(item.Severity)
		if severity == SeverityNone {
			continue
		}
		// The first `<location>` is where the finding is, and the `<call>`s are where
		// it came from — which for a finding with no location of its own is the only
		// place it is. A project-wide check reports against the configuration rather
		// than against a source, and an unused function is reported that way.
		location := item.Locations
		if len(location) == 0 {
			location = item.Calls
		}
		place := cppcheckLocation{}
		if len(location) > 0 {
			place = location[0]
		}
		found = append(found, normalise(Diagnostic{
			Rule:     qualify(spec, cppcheckRule(item.ID)),
			Message:  cppcheckMessage(item),
			Severity: severity,
			File:     Relative(place.File, root),
			Line:     place.Number(),
			Column:   place.ColumnNumber(),
			Engine:   spec.Name,
		}))
	}
	return found, nil
}

// cppcheckSeverity is a cppcheck severity as one of the three here.
func cppcheckSeverity(severity string) Severity {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error":
		return SeverityError
	case "warning", "style", "performance", "portability":
		return SeverityWarning
	case "information":
		return SeverityNote
	default:
		// `debug`, `internal`, and anything a future cppcheck adds.
		return SeverityNone
	}
}

// cppcheckRule is cppcheck's `id` as a name that survives being put next to another
// analyzer's.
//
// cppcheck spells them in camel case — `unusedVariable`, `nullPointer` — which is
// fine until two analyzers are in one report and one of them says
// `unusedVariable` about something else. The name is kept as cppcheck writes it
// because that is what its own documentation calls it, and `qualify` is what makes
// it unambiguous.
func cppcheckRule(id string) string { return strings.TrimSpace(id) }

// cppcheckMessage is the sentence, with cppcheck's own prefix removed.
//
// `[src/foo.c:12]` in front of every message is cppcheck saying which line the
// message is about, which the finding already says in its own location. Left in, it
// would appear in the middle of a sentence in a report that reads as prose.
func cppcheckMessage(item cppcheckError) string {
	message := strings.TrimSpace(item.Message)
	if open := strings.Index(message, "["); open > 0 {
		message = strings.TrimSpace(message[:open])
	}
	if message == "" {
		message = "reported without a message"
	}
	if item.Inconclusive != "" {
		// Said, rather than dropped. An inconclusive finding is one a person may
		// want to look at and one a pipeline must not gate on, and the only way to
		// express that in one shape is in the sentence a person reads.
		message = "inconclusive: " + message
	}
	return message
}

// ReadJSON is findings in this component's own shape, for a project that has already
// normalized its analyzers.
//
// A flat list, not a `{diagnostics: []}` envelope, because the envelope would be one
// more shape to read and the list is what a caller has. A file with an envelope is
// read anyway: an analyzer that writes `{"diagnostics": [...]}` is doing something
// reasonable and a reader that refused it would send somebody to add a wrapper
// program for no gain.
func ReadJSON(output []byte, spec Spec, root string) ([]Diagnostic, error) {
	found := []Diagnostic{}
	if err := json.Unmarshal(output, &found); err == nil {
		return qualifyAll(found, spec, root), nil
	}
	// A pointer, so that a file with no `diagnostics` key at all is refused rather
	// than read as an empty run. `{"config": {...}}` unmarshals into an empty slice
	// without complaint, and reporting that as "this engine found nothing" is exactly
	// the failure a lint tool must not have.
	envelope := struct {
		Diagnostics *[]Diagnostic `json:"diagnostics"`
	}{}
	if err := json.Unmarshal(output, &envelope); err != nil || envelope.Diagnostics == nil {
		if err == nil {
			err = fmt.Errorf("it has no `diagnostics` in it")
		}
		return nil, fmt.Errorf("this is not a list of findings in this component's shape: %w", err)
	}
	return qualifyAll(*envelope.Diagnostics, spec, root), nil
}

// qualifyAll attributes a read list to an engine and relativises it.
func qualifyAll(found []Diagnostic, spec Spec, root string) []Diagnostic {
	out := make([]Diagnostic, 0, len(found))
	for _, one := range found {
		if one.Engine == "" {
			one.Engine = spec.Name
		}
		one.Rule = qualify(spec, one.Rule)
		if one.Severity == "" {
			one.Severity = SeverityWarning
		}
		out = append(out, normalise(one))
	}
	return out
}

// normalise is a finding as it is recorded: relative where it can be, with a
// severity it can be compared against.
//
// Both normalisations are here rather than in each reader because a finding has to
// be comparable with the next one however it arrived, and doing it once at the edge
// is the only way to be sure it happened to all of them.
func normalise(one Diagnostic) Diagnostic {
	one.Severity = Severity(strings.ToLower(strings.TrimSpace(string(one.Severity))))
	if !one.Severity.Known() && one.Severity != SeverityNone && one.Severity != "" {
		// An unrecognized severity becomes a warning rather than being dropped. The
		// alternative is losing a real defect to a spelling, and a finding wrongly
		// reported is a thing a person can see and correct.
		one.Severity = SeverityWarning
	}
	if one.Line < 0 {
		one.Line = 0
	}
	if one.Column < 0 {
		one.Column = 0
	}
	if one.EndLine > 0 && one.EndColumn == 0 {
		one.EndColumn = 0
	}
	return one
}

/* -------------------------------------------------------------------------- */
/*  Small helpers                                                              */
/* -------------------------------------------------------------------------- */

// splitLines is the output split into lines, with the trailing newline dropped and
// CRLF handled.
//
// Every analyzer that prints diagnostics on a Windows machine prints CRLF, and a
// regex that ends at `$` matches before `\r` but a message read out of it ends with
// one — so a rule name parsed out of the end of a line comes back with a carriage
// return on it and matches nothing.
func splitLines(output []byte) []string {
	text := strings.ReplaceAll(string(output), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// xmlUnmarshal reads XML, so that this component has one reader of it rather than
// three.
func xmlUnmarshal(data []byte, into any) error { return xml.Unmarshal(data, into) }

func atoi(value string) int {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return number
}

// ResolveProgram is the program an engine names, found beside the plugin that
// named it or on PATH.
//
// In that order, and the order is the design rather than a detail. A plugin is a
// module with a directory of its own, and a name is unambiguous inside it: a plugin
// in `modules/plugins` carries its wrapper beside it, and one installed from a
// registry carries it in the installed package. PATH is second because it is the
// machine's answer rather than the plugin's, and a project with two versions of an
// analyzer installed should get the one its plugin shipped with.
//
// An absolute path is taken as given, which is what a spec names when the analyzer
// was fetched into the Rho home rather than installed system-wide — the case
// `rho keeper` sets up when it prefers a download over a package manager.
func ResolveProgram(program string, roots ...string) (string, error) {
	name := strings.TrimSpace(program)
	if name == "" {
		return "", fmt.Errorf("no program is named")
	}
	if filepath.IsAbs(name) {
		if executable(name) {
			return name, nil
		}
		if resolved, err := exec.LookPath(name); err == nil {
			return resolved, nil
		}
		return "", fmt.Errorf("%s is not there and is not on PATH either", name)
	}

	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		carried := filepath.Join(root, filepath.FromSlash(name))
		if executable(carried) {
			return carried, nil
		}
	}
	if resolved, err := exec.LookPath(name); err == nil {
		return resolved, nil
	}
	return "", fmt.Errorf("%s is not carried by the plugin and is not on PATH\n"+
		"  `rho doctor` reports on it, and `rho keeper` installs it",
		filepath.Base(name))
}

// executable is a path that is a file and can be run.
func executable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}
