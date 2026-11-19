//
// @file cli.go
// @brief The command line: one subcommand per question or change.
//
// Flags are read wherever they are written, before or after a path, with the
// toolchain's own interspersed parse (`rho/src/cli`), so `rho req check . --json`
// and `rho req check --json .` are the same command.
//
// Exit status: 0 when the answer is clean, 1 when it found problems (or an edit
// failed), 2 when the command line was wrong.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/cli"
)

// Usage is `rho req --help`.
const Usage = `usage: rho req <command> [flags]

  Requirements, architecture decisions, and the trace from them to the code that
  implements them (@req) and the tests that verify them (@verifies).

commands:
  check [dir]          validate files, ids, links, tags; --base REF checks approvals
  list                 the requirements, filtered by --level, --status, --repo
  show ID              one requirement with its upward and downward trace
  trace [dir]          the matrix: children, ADRs, code, tests and their results
  render               the whole as Markdown (build/requirements/requirements.md)
  new                  allocate the next id and append a requirement
  edit ID              --set k=v, --add k=v, --remove k[=v]; keeps the file's layout
  set-status ID STATUS move a requirement through its lifecycle
  adr new|list|supersede|set-status
  init [dir]           scaffold docs/requirements and docs/architecture/adr

common flags:
  --workspace          every repository of the family, not only this one
  --json               machine-readable output
  --tsv                the same as tab-separated records, one per line
  --dir DIR            the repository to work in (default: the current directory)

Run 'rho req <command> --help' for a command's flags. The file formats and the
process are documented in the rho-req module's docs/index.md.
`

// Main runs a command line and returns the exit status.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, Usage)
		return 2
	}
	command, rest := args[0], args[1:]
	run, ok := commands[command]
	if !ok {
		switch command {
		case "-h", "-help", "--help", "help":
			if len(rest) > 0 {
				if _, known := commands[rest[0]]; known {
					return Main([]string{rest[0], "--help"}, stdout, stderr)
				}
			}
			fmt.Fprint(stdout, Usage)
			return 0
		}
		fmt.Fprintf(stderr, "rho req: unknown command %q\n\n%s", command, Usage)
		return 2
	}
	return run(&env{name: command, args: rest, out: stdout, err: stderr})
}

type env struct {
	name     string
	args     []string
	out, err io.Writer
	fs       *flag.FlagSet
	jsonOut  bool
	tsvOut   bool
	wsFlag   bool
	dir      string
	usage    string
}

// flags makes the subcommand's flag set with the common flags.
func (e *env) flags(usage string) *flag.FlagSet {
	e.usage = usage
	e.fs = flag.NewFlagSet("rho req "+e.name, flag.ContinueOnError)
	e.fs.SetOutput(io.Discard)
	e.fs.BoolVar(&e.jsonOut, "json", false, "")
	e.fs.BoolVar(&e.tsvOut, "tsv", false, "")
	e.fs.BoolVar(&e.wsFlag, "workspace", false, "")
	e.fs.StringVar(&e.dir, "dir", "", "")
	return e.fs
}

// parse reads the flags; ok is false when the command should stop with status.
func (e *env) parse() (positional []string, status int, ok bool) {
	if cli.WantsHelp(e.args) {
		fmt.Fprint(e.out, e.usage)
		return nil, 0, false
	}
	positional, _, err := cli.ParseInterspersed(e.fs, e.args)
	if err != nil {
		fmt.Fprintf(e.err, "rho req %s: %v\n\n%s", e.name, err, e.usage)
		return nil, 2, false
	}
	return positional, 0, true
}

func (e *env) fail(format string, args ...any) int {
	message := fmt.Sprintf(format, args...)
	fmt.Fprintf(e.err, "rho req %s: %s\n", e.name, message)
	if e.tsvOut {
		tsvRecord(e.out, "error", message)
	}
	return 1
}

func (e *env) load(dir string) (*Workspace, error) {
	if dir == "" {
		dir = e.dir
	}
	if dir == "" {
		dir = "."
	}
	return Load(dir, Options{Workspace: e.wsFlag})
}

func (e *env) emitJSON(value any) int {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return e.fail("%v", err)
	}
	e.out.Write(append(encoded, '\n'))
	return 0
}

// multi is a flag that may be given more than once.
type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

var commands = map[string]func(*env) int{}

func init() {
	commands["check"] = cmdCheck
	commands["list"] = cmdList
	commands["show"] = cmdShow
	commands["trace"] = cmdTrace
	commands["render"] = cmdRender
	commands["new"] = cmdNew
	commands["edit"] = cmdEdit
	commands["set-status"] = cmdSetStatus
	commands["adr"] = cmdADR
	commands["init"] = cmdInit
}

func first(list []string) string {
	if len(list) > 0 {
		return list[0]
	}
	return ""
}

/* -------------------------------------------------------------------------- */
/*  check                                                                     */
/* -------------------------------------------------------------------------- */

// CheckReport is `rho req check --json`.
type CheckReport struct {
	OK       bool     `json:"ok"`
	Repos    []string `json:"repos"`
	Errors   int      `json:"errors"`
	Warnings int      `json:"warnings"`
	Issues   Issues   `json:"issues"`
}

func cmdCheck(e *env) int {
	fs := e.flags(`usage: rho req check [dir] [--workspace] [--base REF] [--strict] [--json|--tsv]

  Validates the requirements files, the ADRs, the levels, and the @req and
  @verifies tags: schema, id format and prefix, uniqueness across the workspace,
  satisfies (only to a higher level), decided_by, ADR links, tags, cycles.

  --base REF   every approval (a requirement becoming approved, implemented or
               verified, an ADR becoming accepted) in REF..HEAD must be committed
               by someone in MAINTAINERS.yml. A guard: commit authors can be faked;
               branch protection and CODEOWNERS are the authority.
  --strict     warnings fail the check too

exit status: 0 clean, 1 errors (or warnings with --strict), 2 usage
`)
	base := fs.String("base", "", "")
	strict := fs.Bool("strict", false, "")
	positional, status, ok := e.parse()
	if !ok {
		return status
	}
	w, err := e.load(first(positional))
	if err != nil {
		return e.fail("%v", err)
	}
	issues := Check(w, CheckOptions{Base: *base})
	failed := issues.Errors() > 0 || (*strict && issues.Warnings() > 0)
	if e.tsvOut {
		tsvCheck(e.out, w, issues, !failed)
	} else if e.jsonOut {
		report := CheckReport{OK: !failed, Errors: issues.Errors(), Warnings: issues.Warnings(), Issues: issues}
		if report.Issues == nil {
			report.Issues = Issues{}
		}
		for _, repo := range w.Reported() {
			report.Repos = append(report.Repos, repo.Name)
		}
		e.emitJSON(report)
	} else {
		for _, issue := range issues {
			fmt.Fprintln(e.out, issue.String())
		}
		count := 0
		for _, repo := range w.Reported() {
			count += len(repo.Requirements())
		}
		fmt.Fprintf(e.out, "%d requirements, %d errors, %d warnings\n", count, issues.Errors(), issues.Warnings())
	}
	if failed {
		return 1
	}
	return 0
}

/* -------------------------------------------------------------------------- */
/*  list and show                                                             */
/* -------------------------------------------------------------------------- */

func cmdList(e *env) int {
	fs := e.flags(`usage: rho req list [dir] [--level L] [--status S] [--repo R] [--tag T] [--workspace] [--json|--tsv]

  The requirements of this repository (of every repository with --workspace;
  --repo implies it), in file order.
`)
	level := fs.String("level", "", "")
	stat := fs.String("status", "", "")
	repoName := fs.String("repo", "", "")
	tag := fs.String("tag", "", "")
	positional, status, ok := e.parse()
	if !ok {
		return status
	}
	if *repoName != "" {
		e.wsFlag = true
	}
	w, err := e.load(first(positional))
	if err != nil {
		return e.fail("%v", err)
	}
	out := []*Requirement{}
	for _, repo := range w.Reported() {
		if *repoName != "" && repo.Name != *repoName {
			continue
		}
		for _, req := range repo.Requirements() {
			if (*level == "" || req.Level == *level) && (*stat == "" || req.Status == *stat) && (*tag == "" || contains(req.Tags, *tag)) {
				out = append(out, req)
			}
		}
	}
	if e.tsvOut {
		tsvRepos(e.out, w)
		for _, req := range out {
			tsvRequirement(e.out, req)
		}
		return 0
	}
	if e.jsonOut {
		return e.emitJSON(out)
	}
	fmt.Fprint(e.out, ListText(out, e.wsFlag))
	return 0
}

// ShowReport is `rho req show --json`.
type ShowReport struct {
	*Requirement
	Upward   []string       `json:"upward"`
	Children []string       `json:"children"`
	ADRs     []*ADR         `json:"adrs"`
	Code     []Location     `json:"code"`
	Tests    []TestEvidence `json:"tests"`
	Evidence string         `json:"evidence"`
}

func cmdShow(e *env) int {
	e.flags(`usage: rho req show ID [--dir DIR] [--json|--tsv]

  One requirement: its text, what it satisfies (upward, transitively), what
  satisfies it (downward, as a tree), its ADRs, its @req code and its @verifies
  tests with their latest results. The id is looked up across the workspace.
`)
	positional, status, ok := e.parse()
	if !ok {
		return status
	}
	if len(positional) != 1 {
		fmt.Fprint(e.err, e.usage)
		return 2
	}
	id := positional[0]
	e.wsFlag = true
	w, err := e.load("")
	if err != nil {
		return e.fail("%v", err)
	}
	req := w.Requirement(id)
	if req == nil {
		return e.fail("%s does not exist in this workspace", id)
	}
	row := Trace(w).Row(id)
	if e.tsvOut {
		tsvShow(e.out, w, req, row)
		return 0
	}
	if e.jsonOut {
		report := ShowReport{Requirement: req, Upward: []string{}, Children: row.Children, ADRs: []*ADR{}, Code: row.Code, Tests: row.Tests, Evidence: row.Evidence}
		seen := map[string]bool{}
		var up func(id string)
		up = func(id string) {
			if r := w.Requirement(id); r != nil {
				for _, parent := range r.Satisfies {
					if !seen[parent] {
						seen[parent] = true
						report.Upward = append(report.Upward, parent)
						up(parent)
					}
				}
			}
		}
		up(id)
		for _, ref := range row.ADRs {
			if adr := w.ADR(req.Repo, ref); adr != nil {
				report.ADRs = append(report.ADRs, adr)
			}
		}
		return e.emitJSON(report)
	}
	fmt.Fprint(e.out, ShowText(w, req, row))
	return 0
}

/* -------------------------------------------------------------------------- */
/*  trace and render                                                          */
/* -------------------------------------------------------------------------- */

// TraceReport is `rho req trace --json`.
type TraceReport struct {
	*Matrix
	Blocking int  `json:"blocking"`
	Release  bool `json:"release_ready"`
}

func cmdTrace(e *env) int {
	e.flags(`usage: rho req trace [dir] [--workspace] [--json|--tsv] [--release]

  Each requirement against its children (lower levels that satisfy it), its
  ADRs, its @req code, and its @verifies tests with their latest results, read
  from build/test-reports/ (JUnit XML, rho test text, TAP, go test -v).

  --release    exit 1 when an approved, implemented or verified requirement lacks
               passing verification (the release gate)
`)
	release := e.fs.Bool("release", false, "")
	positional, status, ok := e.parse()
	if !ok {
		return status
	}
	w, err := e.load(first(positional))
	if err != nil {
		return e.fail("%v", err)
	}
	matrix := Trace(w)
	blocking := matrix.Blocking()
	if e.tsvOut {
		tsvTrace(e.out, w, matrix)
	} else if e.jsonOut {
		e.emitJSON(TraceReport{Matrix: matrix, Blocking: blocking, Release: blocking == 0})
	} else {
		fmt.Fprint(e.out, TraceText(matrix))
		if *release {
			if blocking > 0 {
				fmt.Fprintf(e.out, "\nnot releasable: %d gaps block a release\n", blocking)
			} else {
				fmt.Fprintln(e.out, "\nreleasable: every committed requirement has passing verification")
			}
		}
	}
	if *release && blocking > 0 {
		return 1
	}
	return 0
}

func cmdRender(e *env) int {
	e.flags(`usage: rho req render [dir] [--workspace] [--out FILE|DIR|-] [--level L]

  The requirements as Markdown: a section per level, every requirement's text,
  the ADR index, and the trace matrix with its gaps. Generated on demand, not
  committed. Written to build/requirements/requirements.md under the repository
  unless --out names a file, a directory, or - for standard output.
`)
	outPath := e.fs.String("out", "", "")
	level := e.fs.String("level", "", "")
	positional, status, ok := e.parse()
	if !ok {
		return status
	}
	w, err := e.load(first(positional))
	if err != nil {
		return e.fail("%v", err)
	}
	text := Render(w, Trace(w), *level)
	target := *outPath
	switch {
	case target == "-":
		fmt.Fprint(e.out, text)
		return 0
	case target == "":
		target = filepath.Join(w.Focus.Dir, "build", "requirements", "requirements.md")
	case strings.HasSuffix(target, "/") || isDir(target) || filepath.Ext(target) == "":
		target = filepath.Join(target, "requirements.md")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return e.fail("%v", err)
	}
	if err := os.WriteFile(target, []byte(text), 0644); err != nil {
		return e.fail("%v", err)
	}
	fmt.Fprintln(e.out, target)
	return 0
}

/* -------------------------------------------------------------------------- */
/*  new, edit, set-status                                                     */
/* -------------------------------------------------------------------------- */

func cmdNew(e *env) int {
	fs := e.flags(`usage: rho req new --title T --statement S [--level L] [--prefix P] [--file F] [flags]

  Allocates the next free id for the prefix across the workspace, appends the
  requirement to the file for that level and prefix (creating it with a header
  when there is none), and prints the id.

  --level L          the level (taken from --file when that exists)
  --prefix P         the id prefix (default: SYS at the top level, else the
                     repository's requirements.prefix)
  --file F           the file, relative to docs/requirements/ (default: the first
                     file of that level and prefix, else <level>-<prefix>.yml)
  --rationale R, --owner O, --satisfies A,B, --decided-by ADR-1, --tags a,b
  --status S         default draft;  --verification V   default test
`)
	request := NewRequest{}
	fs.StringVar(&request.Level, "level", "", "")
	fs.StringVar(&request.Prefix, "prefix", "", "")
	fs.StringVar(&request.File, "file", "", "")
	fs.StringVar(&request.Title, "title", "", "")
	fs.StringVar(&request.Statement, "statement", "", "")
	fs.StringVar(&request.Rationale, "rationale", "", "")
	fs.StringVar(&request.Owner, "owner", "", "")
	fs.StringVar(&request.Status, "status", "", "")
	fs.StringVar(&request.Verification, "verification", "", "")
	satisfies := fs.String("satisfies", "", "")
	decided := fs.String("decided-by", "", "")
	tags := fs.String("tags", "", "")
	positional, status, ok := e.parse()
	if !ok {
		return status
	}
	request.Satisfies, request.DecidedBy, request.Tags = splitList(*satisfies), splitList(*decided), splitList(*tags)
	w, err := e.load(first(positional))
	if err != nil {
		return e.fail("%v", err)
	}
	id, file, err := New(w, request)
	if err != nil {
		return e.fail("%v", err)
	}
	if e.tsvOut {
		repo := ""
		if created := w.Requirement(id); created != nil {
			repo = created.Repo
		} else if w.Focus != nil {
			repo = w.Focus.Name
		}
		tsvRecord(e.out, "created", id, repo, file)
		return 0
	}
	if e.jsonOut {
		return e.emitJSON(map[string]string{"id": id, "file": file})
	}
	fmt.Fprintln(e.out, id)
	fmt.Fprintf(e.err, "added to %s\n", file)
	return 0
}

func parseOps(op string, values []string) ([]EditOp, error) {
	out := []EditOp{}
	for _, value := range values {
		key, val, hasValue := strings.Cut(value, "=")
		if !hasValue && op != "remove" {
			return nil, fmt.Errorf("--%s needs key=value, got %q", op, value)
		}
		out = append(out, EditOp{Op: op, Key: strings.TrimSpace(key), Value: val})
	}
	return out, nil
}

func cmdEdit(e *env) int {
	fs := e.flags(`usage: rho req edit ID [--set key=value]... [--add key=value]... [--remove key[=value]]... [--keep-status]

  Changes one requirement in place: only the lines of the values that change are
  touched, so comments, blank lines, key order and the other requirements stay
  as they were. changed: is set to today.

  --set key=value      a scalar; for satisfies, decided_by and tags the whole list
  --add key=value      add to a list (comma-separated values allowed)
  --remove key=value   take out of a list;  --remove key  removes an optional key
  --keep-status        a statement change on an approved (or later) requirement
                       sends it back to proposed unless this is given
  --force              allow a status change the lifecycle does not
`)
	var sets, adds, removes multi
	fs.Var(&sets, "set", "")
	fs.Var(&adds, "add", "")
	fs.Var(&removes, "remove", "")
	keep := fs.Bool("keep-status", false, "")
	force := fs.Bool("force", false, "")
	positional, status, ok := e.parse()
	if !ok {
		return status
	}
	if len(positional) != 1 {
		fmt.Fprint(e.err, e.usage)
		return 2
	}
	ops := []EditOp{}
	for _, group := range []struct {
		op     string
		values []string
	}{{"set", sets}, {"add", adds}, {"remove", removes}} {
		parsed, err := parseOps(group.op, group.values)
		if err != nil {
			fmt.Fprintf(e.err, "rho req edit: %v\n", err)
			return 2
		}
		ops = append(ops, parsed...)
	}
	if len(ops) == 0 {
		fmt.Fprintln(e.err, "rho req edit: nothing to change (--set, --add or --remove)")
		return 2
	}
	e.wsFlag = true
	w, err := e.load("")
	if err != nil {
		return e.fail("%v", err)
	}
	result, err := Edit(w, positional[0], ops, EditOptions{KeepStatus: *keep, Force: *force})
	return e.reportEdit(result, err)
}

func (e *env) reportEdit(result EditResult, err error) int {
	if err != nil {
		return e.fail("%v", err)
	}
	if e.tsvOut {
		tsvRecord(e.out, "updated", result.ID, result.File, result.Status, result.Reopened)
		return 0
	}
	if e.jsonOut {
		return e.emitJSON(map[string]any{"id": result.ID, "file": result.File, "status": result.Status, "reopened": result.Reopened})
	}
	fmt.Fprintf(e.out, "%s updated in %s (status %s)\n", result.ID, result.File, result.Status)
	if result.Reopened {
		fmt.Fprintln(e.out, "the statement changed, so it is proposed again and needs a maintainer's approval (--keep-status keeps it)")
	}
	return 0
}

func cmdSetStatus(e *env) int {
	e.flags(`usage: rho req set-status ID STATUS [--force]

  draft -> proposed -> approved -> implemented -> verified; back to proposed for a
  re-review; any status -> rejected or retired; rejected -> draft.
  --force allows any other change.
`)
	force := e.fs.Bool("force", false, "")
	positional, status, ok := e.parse()
	if !ok {
		return status
	}
	if len(positional) != 2 {
		fmt.Fprint(e.err, e.usage)
		return 2
	}
	e.wsFlag = true
	w, err := e.load("")
	if err != nil {
		return e.fail("%v", err)
	}
	result, err := SetStatus(w, positional[0], positional[1], EditOptions{Force: *force})
	return e.reportEdit(result, err)
}

/* -------------------------------------------------------------------------- */
/*  adr                                                                       */
/* -------------------------------------------------------------------------- */

const adrUsage = `usage: rho req adr <new|list|supersede> [flags]

  new --title T [--requirements A,B] [--status S]
                         write the next docs/architecture/adr/NNNN-slug.md from
                         0000-template.md (or a built-in template); prints the id
  list [--workspace] [--json|--tsv]
  supersede OLD NEW      OLD becomes superseded, superseded_by NEW; NEW supersedes
                         OLD. Another repository's ADR is written <repo>:ADR-NNNN.
  set-status ADR STATUS  proposed, accepted or rejected; superseded is what
                         supersede does. Only the status line changes.

  --dir DIR names the repository an ADR is written in or looked up from.
`

func cmdADR(e *env) int {
	if len(e.args) == 0 || cli.WantsHelp(e.args[:1]) || e.args[0] == "help" {
		fmt.Fprint(e.out, adrUsage)
		if len(e.args) == 0 {
			return 2
		}
		return 0
	}
	sub := e.args[0]
	e.args = e.args[1:]
	e.name = "adr " + sub
	switch sub {
	case "new":
		fs := e.flags(adrUsage)
		title := fs.String("title", "", "")
		reqs := fs.String("requirements", "", "")
		stat := fs.String("status", "", "")
		positional, status, ok := e.parse()
		if !ok {
			return status
		}
		w, err := e.load(first(positional))
		if err != nil {
			return e.fail("%v", err)
		}
		id, file, err := NewADR(w, *title, splitList(*reqs), *stat)
		if err != nil {
			return e.fail("%v", err)
		}
		if e.tsvOut {
			tsvRecord(e.out, "created", id, w.Focus.Name, file)
			return 0
		}
		if e.jsonOut {
			return e.emitJSON(map[string]string{"id": id, "file": file})
		}
		fmt.Fprintln(e.out, id)
		fmt.Fprintf(e.err, "written to %s\n", file)
		return 0
	case "list":
		e.flags(adrUsage)
		positional, status, ok := e.parse()
		if !ok {
			return status
		}
		w, err := e.load(first(positional))
		if err != nil {
			return e.fail("%v", err)
		}
		adrs := []*ADR{}
		for _, repo := range w.Reported() {
			adrs = append(adrs, repo.ADRs...)
		}
		if e.tsvOut {
			tsvRepos(e.out, w)
			for _, adr := range adrs {
				tsvADR(e.out, adr)
			}
			return 0
		}
		if e.jsonOut {
			return e.emitJSON(adrs)
		}
		for _, adr := range adrs {
			label := adr.ID
			if e.wsFlag {
				label = adr.Ref()
			}
			fmt.Fprintf(e.out, "%-24s %-11s %s  %s\n", label, adr.Status, adr.Date, adr.Title)
		}
		return 0
	case "supersede":
		e.flags(adrUsage)
		positional, status, ok := e.parse()
		if !ok {
			return status
		}
		if len(positional) != 2 {
			fmt.Fprint(e.err, adrUsage)
			return 2
		}
		e.wsFlag = true
		w, err := e.load("")
		if err != nil {
			return e.fail("%v", err)
		}
		if err := Supersede(w, positional[0], positional[1]); err != nil {
			return e.fail("%v", err)
		}
		if e.tsvOut {
			tsvRecord(e.out, "superseded", positional[0], positional[1])
			return 0
		}
		fmt.Fprintf(e.out, "%s is superseded by %s\n", positional[0], positional[1])
		return 0
	case "set-status":
		e.flags(adrUsage)
		positional, status, ok := e.parse()
		if !ok {
			return status
		}
		if len(positional) != 2 {
			fmt.Fprint(e.err, adrUsage)
			return 2
		}
		e.wsFlag = true
		w, err := e.load("")
		if err != nil {
			return e.fail("%v", err)
		}
		adr, err := SetADRStatus(w, positional[0], positional[1])
		if err != nil {
			return e.fail("%v", err)
		}
		if e.tsvOut {
			tsvRecord(e.out, "updated", adr.Ref(), adr.File, adr.Status, false)
			return 0
		}
		if e.jsonOut {
			return e.emitJSON(adr)
		}
		fmt.Fprintf(e.out, "%s is %s\n", adr.Ref(), adr.Status)
		return 0
	}
	fmt.Fprintf(e.err, "rho req adr: unknown command %q\n\n%s", sub, adrUsage)
	return 2
}

/* -------------------------------------------------------------------------- */
/*  init                                                                      */
/* -------------------------------------------------------------------------- */

func cmdInit(e *env) int {
	fs := e.flags(`usage: rho req init [dir] [--level L --prefix P] [--parent REPO] [--root]

  Scaffolds docs/requirements/ and docs/architecture/adr/0000-template.md, and
  adds a requirements: block (prefix, parent) to an existing module.yml. Nothing
  that exists is overwritten; docs/doc and other pages are not touched.

  --root           this repository defines the levels and who approves: also
                   writes docs/requirements/levels.yml and MAINTAINERS.yml
  --level, --prefix  also create the first requirements file for them
  --parent REPO    the repository that holds levels.yml and the system level
`)
	request := InitRequest{}
	fs.StringVar(&request.Level, "level", "", "")
	fs.StringVar(&request.Prefix, "prefix", "", "")
	fs.StringVar(&request.Parent, "parent", "", "")
	fs.BoolVar(&request.Root, "root", false, "")
	positional, status, ok := e.parse()
	if !ok {
		return status
	}
	request.Dir = first(positional)
	if request.Dir == "" {
		request.Dir = e.dir
	}
	if request.Dir == "" {
		request.Dir = "."
	}
	if request.Root && request.Parent != "" {
		fmt.Fprintln(e.err, "rho req init: a --root repository has no --parent")
		return 2
	}
	created, kept, err := Init(request)
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			return e.fail("%v", pathErr)
		}
		return e.fail("%v", err)
	}
	sort.Strings(created)
	if e.jsonOut {
		if created == nil {
			created = []string{}
		}
		if kept == nil {
			kept = []string{}
		}
		return e.emitJSON(map[string][]string{"created": created, "kept": kept})
	}
	for _, path := range created {
		fmt.Fprintln(e.out, "created", path)
	}
	for _, path := range kept {
		fmt.Fprintln(e.out, "kept   ", path)
	}
	return 0
}
