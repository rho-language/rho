//
// @file pages.go
// @brief The pages a documentation site shows for a module's requirements and decisions.
//
// For every module of a site that is a repository of the requirements process —
// it keeps docs/requirements/ or docs/architecture/adr/ — this writes, as the
// doc-as-code rho-doc reads:
//
//	<module>/requirements/index   every requirement of the module, by level, with
//	                              its text, status, trace, decisions, code, tests
//	                              and evidence; each requirement is a label
//	<module>/requirements/trace   for the root of a family (it keeps levels.yml):
//	                              every level, every requirement of the family,
//	                              the trace matrix and its gaps
//	<module>/adr/index            the decisions, by number, with their status
//	<module>/adr/NNNN-slug        one decision: its front matter as a header, then
//	                              its text; labelled <repository>:ADR-NNNN
//
// The model is rho-req's: the files are read, the family resolved and the trace
// built by its package, so these pages and `rho req check` read the same files
// the same way. A requirement is linked where it is on the site and named in
// plain text where it is not, so a page never carries a reference that resolves
// to nothing.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package docreq

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rho/rho-req/src/rhoreq"
)

// Module is a module of the site, as rho-doc's request names it.
type Module struct {
	Name    string
	Root    string
	Section string
}

// Page is a page written for a module, as rho-doc's sources answer carries it.
type Page struct {
	Module   string `json:"module"`
	Path     string `json:"path"`
	Title    string `json:"title,omitempty"`
	Markdown string `json:"markdown"`
	Source   string `json:"source,omitempty"`
	Order    int    `json:"order,omitempty"`
	Unlisted bool   `json:"unlisted,omitempty"`
}

// The order of a module's generated pages, after its own.
const (
	orderRequirements = 1
	orderTrace        = 2
	orderADRIndex     = 3
	orderADR          = 100
)

// Addresses inside a module.
const (
	RequirementsPath = "requirements/index"
	TracePath        = "requirements/trace"
	ADRIndexPath     = "adr/index"
	ADRPrefix        = "adr/"
)

// site is what the pages of one build share: which repositories are on the site
// and under which module name.
type site struct {
	// modules maps a repository directory (cleaned, absolute) to its module.
	modules map[string]string
}

// Pages writes the pages for every module of a site that keeps requirements or
// decisions, and the notes a person building the site should see.
func Pages(modules []Module) ([]Page, []string) {
	s := site{modules: map[string]string{}}
	candidates := []Module{}
	for _, module := range modules {
		dir := canonical(module.Root)
		if dir == "" || !hasProcess(dir) || canonical(rhoreq.FindRepoRoot(dir)) != dir {
			continue
		}
		if _, seen := s.modules[dir]; seen {
			continue
		}
		s.modules[dir] = module.Name
		candidates = append(candidates, module)
	}
	pages, notes := []Page{}, []string{}
	for _, module := range candidates {
		w, err := rhoreq.Load(module.Root, rhoreq.Options{})
		if err != nil {
			notes = append(notes, fmt.Sprintf("rho-doc-req: %s: %v", module.Name, err))
			continue
		}
		if errors := w.Issues.Errors(); errors > 0 {
			notes = append(notes, fmt.Sprintf("rho-doc-req: %s: the requirements have %d error(s); the pages show what could be read (run `rho req check`)", module.Name, errors))
		}
		focus := w.Focus
		matrix := rhoreq.Trace(w)
		if len(focus.Requirements()) > 0 {
			pages = append(pages, Page{Module: module.Name, Path: RequirementsPath, Order: orderRequirements,
				Title: module.Name + " requirements", Markdown: s.requirementsPage(w, matrix, module.Name)})
		}
		if focus == w.Root && fileExists(filepath.Join(focus.Dir, filepath.FromSlash(rhoreq.LevelsFile))) {
			whole, err := rhoreq.Load(module.Root, rhoreq.Options{Workspace: true})
			if err == nil && len(whole.AllRequirements()) > 0 {
				pages = append(pages, Page{Module: module.Name, Path: TracePath, Order: orderTrace,
					Title: "Requirements trace", Markdown: s.tracePage(whole, rhoreq.Trace(whole))})
			}
		}
		if len(focus.ADRs) > 0 {
			adrs := append([]*rhoreq.ADR{}, focus.ADRs...)
			sort.SliceStable(adrs, func(a, b int) bool { return adrs[a].ID < adrs[b].ID })
			pages = append(pages, Page{Module: module.Name, Path: ADRIndexPath, Order: orderADRIndex,
				Title: module.Name + " architecture decisions", Markdown: s.adrIndexPage(w, adrs, module.Name)})
			for index, adr := range adrs {
				pages = append(pages, Page{Module: module.Name, Path: ADRPrefix + adrSlug(adr), Order: orderADR + index, Unlisted: true,
					Title: adr.ID + ": " + adr.Title, Source: filepath.Join(focus.Dir, filepath.FromSlash(adr.File)), Markdown: s.adrPage(w, adr)})
			}
		}
	}
	return pages, notes
}

/* -------------------------------------------------------------------------- */
/*  A module's requirements                                                   */
/* -------------------------------------------------------------------------- */

func (s site) requirementsPage(w *rhoreq.Workspace, matrix *rhoreq.Matrix, module string) string {
	var b strings.Builder
	focus := w.Focus
	fmt.Fprintf(&b, "# %s requirements\n\n", module)
	b.WriteString("Generated from `docs/requirements/` when this site was built. Edit the YAML (or use `rho req`), not this page.\n\n")
	withEvidence := familyHasResults(w)
	b.WriteString(evidenceNote(w) + "\n\n")
	reqs := focus.Requirements()
	for _, level := range focus.Levels {
		atLevel := []*rhoreq.Requirement{}
		for _, req := range reqs {
			if req.Level == level.Name {
				atLevel = append(atLevel, req)
			}
		}
		if len(atLevel) == 0 {
			continue
		}
		sort.SliceStable(atLevel, func(a, c int) bool { return atLevel[a].ID < atLevel[c].ID })
		fmt.Fprintf(&b, "## %s (%s)\n\n", level.Title, level.Name)
		if withEvidence {
			b.WriteString("| ID | Title | Status | Verification | Evidence |\n|---|---|---|---|---|\n")
		} else {
			b.WriteString("| ID | Title | Status | Verification |\n|---|---|---|---|\n")
		}
		for _, req := range atLevel {
			row := matrix.Row(req.ID)
			fmt.Fprintf(&b, "| %s | %s | %s | %s |", s.req(w, req.ID), cell(req.Title), req.Status, req.Verification)
			if withEvidence && row != nil {
				fmt.Fprintf(&b, " %s |", row.Evidence)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		for _, req := range atLevel {
			s.requirement(&b, w, req, matrix.Row(req.ID), withEvidence)
		}
	}
	return b.String()
}

// requirement is one requirement in full: a heading that is its label, its text,
// and what it is linked to.
func (s site) requirement(b *strings.Builder, w *rhoreq.Workspace, req *rhoreq.Requirement, row *rhoreq.Row, withEvidence bool) {
	fmt.Fprintf(b, "### %s: %s {#%s}\n\n", req.ID, oneLine(req.Title), req.ID)
	fmt.Fprintf(b, "%s\n\n", strings.TrimSpace(req.Statement))
	if strings.TrimSpace(req.Rationale) != "" {
		fmt.Fprintf(b, "*Rationale.* %s\n\n", strings.TrimSpace(req.Rationale))
	}
	facts := []string{
		"**Status:** " + req.Status,
		"**Verification:** " + req.Verification,
	}
	if len(req.Satisfies) > 0 {
		facts = append(facts, "**Satisfies:** "+s.reqs(w, req.Satisfies))
	}
	if row != nil && len(row.Children) > 0 {
		facts = append(facts, "**Satisfied by:** "+s.reqs(w, row.Children))
	}
	if row != nil && len(row.ADRs) > 0 {
		facts = append(facts, "**Decided by:** "+s.adrs(w, req.Repo, row.ADRs))
	}
	if row != nil && len(row.Code) > 0 {
		places := []string{}
		for _, at := range row.Code {
			places = append(places, s.place(w, req.Repo, at))
		}
		facts = append(facts, "**Implemented in:** "+strings.Join(places, ", "))
	}
	if row != nil && len(row.Tests) > 0 {
		tests := []string{}
		for _, t := range row.Tests {
			name := s.place(w, req.Repo, t.Location)
			if t.Test != "" {
				name = "`" + t.Test + "` (" + name + ")"
			}
			if withEvidence {
				name += ": " + t.Result
			}
			tests = append(tests, name)
		}
		facts = append(facts, "**Verified by:** "+strings.Join(tests, ", "))
	}
	if withEvidence && row != nil {
		facts = append(facts, "**Evidence:** "+row.Evidence)
	}
	if req.Owner != "" {
		facts = append(facts, "**Owner:** "+req.Owner)
	}
	if req.Changed != "" {
		facts = append(facts, "**Changed:** "+req.Changed)
	}
	if len(req.Tags) > 0 {
		facts = append(facts, "**Tags:** "+strings.Join(req.Tags, ", "))
	}
	facts = append(facts, fmt.Sprintf("**Source:** `%s:%d`", req.File, req.Line))
	for _, fact := range facts {
		fmt.Fprintf(b, "- %s\n", fact)
	}
	b.WriteString("\n")
}

/* -------------------------------------------------------------------------- */
/*  The family's trace                                                        */
/* -------------------------------------------------------------------------- */

func (s site) tracePage(w *rhoreq.Workspace, matrix *rhoreq.Matrix) string {
	var b strings.Builder
	b.WriteString("# Requirements trace\n\n")
	names := []string{}
	for _, repo := range w.Repos {
		names = append(names, s.repoLabel(repo))
	}
	fmt.Fprintf(&b, "Every requirement of the %d repositories of this family (%s), traced to what satisfies it, decides it, implements it and verifies it. "+
		"Generated when this site was built, as `rho req trace --workspace` sees it.\n\n", len(w.Repos), strings.Join(names, ", "))

	b.WriteString("## Levels\n\n| Level | Title | Requirements | Repositories |\n|---|---|---|---|\n")
	for _, level := range w.Root.Levels {
		count, repos := 0, []string{}
		for _, repo := range w.Repos {
			here := 0
			for _, req := range repo.Requirements() {
				if req.Level == level.Name {
					here++
				}
			}
			if here > 0 {
				count += here
				repos = append(repos, s.repoLabel(repo))
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", level.Name, cell(level.Title), count, strings.Join(repos, ", "))
	}
	b.WriteString("\n")

	withEvidence := familyHasResults(w)
	all := w.AllRequirements()
	for _, level := range w.Root.Levels {
		atLevel := []*rhoreq.Requirement{}
		for _, req := range all {
			if req.Level == level.Name {
				atLevel = append(atLevel, req)
			}
		}
		if len(atLevel) == 0 {
			continue
		}
		sort.SliceStable(atLevel, func(a, c int) bool { return atLevel[a].ID < atLevel[c].ID })
		fmt.Fprintf(&b, "## %s (%s)\n\n| ID | Title | Repository | Status | Satisfies |\n|---|---|---|---|---|\n", level.Title, level.Name)
		for _, req := range atLevel {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", s.req(w, req.ID), cell(req.Title), s.repoLabel(w.Repo(req.Repo)), req.Status, s.reqs(w, req.Satisfies))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Trace matrix\n\n")
	if !withEvidence {
		b.WriteString("No test results were available when this page was built, so every test shows as not run.\n\n")
	}
	b.WriteString("| ID | Status | Satisfied by | Decisions | Code | Tests | Evidence |\n|---|---|---|---|---|---|---|\n")
	for _, row := range matrix.Rows {
		code := []string{}
		for _, at := range row.Code {
			code = append(code, s.place(w, "", at))
		}
		tests := []string{}
		for _, t := range row.Tests {
			name := t.Test
			if name == "" {
				name = fmt.Sprintf("%s:%d", t.File, t.Line)
			}
			tests = append(tests, fmt.Sprintf("`%s` (%s)", name, t.Result))
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n", s.req(w, row.ID), row.Status, s.reqs(w, row.Children), s.adrsNamed(w, row.Repo, row.ADRs, true),
			strings.Join(code, ", "), strings.Join(tests, ", "), row.Evidence)
	}
	b.WriteString("\n## Gaps\n\n")
	if len(matrix.Gaps) == 0 {
		b.WriteString("None.\n")
		return b.String()
	}
	blocking := matrix.Blocking()
	fmt.Fprintf(&b, "%d gap(s), %d of which block a release.\n\n", len(matrix.Gaps), blocking)
	for _, gap := range matrix.Gaps {
		subject := ""
		switch {
		case gap.Code == "orphan-adr" && gap.ID != "":
			subject = s.adrsNamed(w, gap.Repo, []string{gap.ID}, true) + ": "
		case gap.ID != "" && w.Requirement(gap.ID) != nil:
			subject = s.req(w, gap.ID) + ": "
		}
		marker := ""
		if gap.Blocking {
			marker = " (blocks a release)"
		}
		fmt.Fprintf(&b, "- **%s** %s%s%s\n", gap.Code, subject, escapeText(gap.Message), marker)
	}
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Decisions                                                                 */
/* -------------------------------------------------------------------------- */

func (s site) adrIndexPage(w *rhoreq.Workspace, adrs []*rhoreq.ADR, module string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s architecture decisions\n\n", module)
	b.WriteString("Generated from `docs/architecture/adr/` when this site was built. A page of this module refers to a decision as `ADR-NNNN`, " +
		"any other page as `" + w.Focus.Name + ":ADR-NNNN`, both with the `{ref}` role.\n\n")
	b.WriteString("| Decision | Title | Status | Date | Requirements |\n|---|---|---|---|---|\n")
	for _, adr := range adrs {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", s.adrs(w, adr.Repo, []string{adr.ID}), cell(adr.Title), adr.Status, adr.Date, s.reqs(w, adr.Requirements))
	}
	return b.String()
}

func (s site) adrPage(w *rhoreq.Workspace, adr *rhoreq.ADR) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s {#%s}\n\n", adr.ID, oneLine(adr.Title), adr.Ref())
	facts := []string{"**Status:** " + adr.Status}
	if adr.Date != "" {
		facts = append(facts, "**Date:** "+adr.Date)
	}
	if len(adr.Requirements) > 0 {
		facts = append(facts, "**Requirements:** "+s.reqs(w, adr.Requirements))
	}
	if len(adr.Supersedes) > 0 {
		facts = append(facts, "**Supersedes:** "+s.adrs(w, adr.Repo, adr.Supersedes))
	}
	if len(adr.SupersededBy) > 0 {
		facts = append(facts, "**Superseded by:** "+s.adrs(w, adr.Repo, adr.SupersededBy))
	}
	facts = append(facts, "**Source:** `"+adr.File+"`")
	for _, fact := range facts {
		fmt.Fprintf(&b, "- %s\n", fact)
	}
	b.WriteString("\n")
	body := rhoreq.ADRBody(w, adr)
	// The title is the page's; a body that repeats it as a heading of its own
	// would be a second title.
	if lines := strings.SplitN(body, "\n", 2); strings.HasPrefix(lines[0], "# ") {
		body = ""
		if len(lines) == 2 {
			body = strings.TrimLeft(lines[1], "\n")
		}
	}
	b.WriteString(body)
	return b.String()
}

// adrSlug is a decision's address inside its module: its file's name.
func adrSlug(adr *rhoreq.ADR) string {
	return strings.TrimSuffix(filepath.Base(adr.File), ".md")
}

/* -------------------------------------------------------------------------- */
/*  Links                                                                     */
/* -------------------------------------------------------------------------- */

// onSite is the module a repository is documented as on this site, or "".
func (s site) onSite(w *rhoreq.Workspace, repoName string) string {
	repo := w.Repo(repoName)
	if repo == nil {
		return ""
	}
	return s.modules[canonical(repo.Dir)]
}

// req is a requirement: a reference where it has a page on this site, its id in
// code type where it has none.
func (s site) req(w *rhoreq.Workspace, id string) string {
	req := w.Requirement(id)
	if req == nil || s.onSite(w, req.Repo) == "" {
		return "`" + id + "`"
	}
	return "{ref}`" + id + "`"
}

func (s site) reqs(w *rhoreq.Workspace, ids []string) string {
	out := []string{}
	for _, id := range ids {
		out = append(out, s.req(w, id))
	}
	return strings.Join(out, ", ")
}

// adrs are decisions as a repository names them: a reference to the decision's
// label where it is on this site, the name as written where it is not.
func (s site) adrs(w *rhoreq.Workspace, from string, refs []string) string {
	return s.adrsNamed(w, from, refs, false)
}

// adrsNamed is adrs, naming each decision with its repository when qualified is
// set: on a page about the whole family, `ADR-0001` alone says nothing.
func (s site) adrsNamed(w *rhoreq.Workspace, from string, refs []string, qualified bool) string {
	out := []string{}
	for _, ref := range refs {
		adr := w.ADR(from, ref)
		if adr == nil {
			out = append(out, "`"+ref+"`")
			continue
		}
		name := rhoreq.RefFrom(from, adr)
		if qualified {
			name = adr.Ref()
		}
		if s.onSite(w, adr.Repo) == "" {
			out = append(out, "`"+name+"`")
			continue
		}
		out = append(out, "{ref}`"+name+" <"+adr.Ref()+">`")
	}
	return strings.Join(out, ", ")
}

// place is a file and line, named with its repository when it is not the one the
// page is about.
func (s site) place(w *rhoreq.Workspace, repo string, at rhoreq.Location) string {
	where := at.File
	if at.Repo != "" && at.Repo != repo {
		where = at.Repo + "/" + where
	}
	return fmt.Sprintf("`%s:%d`", where, at.Line)
}

// repoLabel is a repository linked to its requirements page when it has one on
// this site.
func (s site) repoLabel(repo *rhoreq.Repo) string {
	if repo == nil {
		return ""
	}
	module := s.modules[canonical(repo.Dir)]
	if module == "" || len(repo.Requirements()) == 0 {
		return repo.Name
	}
	return "{ref}`" + repo.Name + " <" + module + "/" + RequirementsPath + ">`"
}

/* -------------------------------------------------------------------------- */
/*  Evidence                                                                  */
/* -------------------------------------------------------------------------- */

func hasResults(repo *rhoreq.Repo) bool {
	return len(repo.Results().Cases) > 0
}

// familyHasResults reports whether any repository of the family has test
// results: a requirement's evidence includes its children's, which may be in
// another repository.
func familyHasResults(w *rhoreq.Workspace) bool {
	for _, repo := range w.Repos {
		if hasResults(repo) {
			return true
		}
	}
	return false
}

// evidenceNote says where the evidence on a page came from, or that there is
// none, so a page built without test results does not read as untested.
func evidenceNote(w *rhoreq.Workspace) string {
	if !familyHasResults(w) {
		return "No test results were available when this page was built, so it shows no evidence; `rho req trace` shows it after `rho test`."
	}
	if !hasResults(w.Focus) {
		return "Evidence is from the test results other repositories of the family had when this site was built; this one had none, so its own tests show as not run."
	}
	if at := rhoreq.RunTime(w.Focus.Dir); at != "" {
		return "Evidence is from the test run of " + at + " (`" + rhoreq.ReportsDir + "/" + rhoreq.ResultsFile + "`)."
	}
	return "Evidence is from the reports in `" + rhoreq.ReportsDir + "/` when this site was built."
}

/* -------------------------------------------------------------------------- */
/*  Text                                                                      */
/* -------------------------------------------------------------------------- */

// cell is text that fits in a table cell: one line, no column separator.
func cell(text string) string {
	return strings.ReplaceAll(oneLine(text), "|", "/")
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// escapeText keeps a message from being read as markup.
func escapeText(text string) string {
	return strings.ReplaceAll(text, "`", "'")
}

// hasProcess reports whether a directory keeps requirements or decisions.
func hasProcess(dir string) bool {
	for _, sub := range []string{rhoreq.RequirementsDir, rhoreq.ADRDir} {
		if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(sub))); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// canonical is a directory as one string whatever path reached it: absolute,
// clean, with its symbolic links resolved.
func canonical(dir string) string {
	if dir == "" {
		return ""
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved
	}
	return absolute
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
