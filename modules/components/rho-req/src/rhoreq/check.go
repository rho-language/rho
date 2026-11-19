//
// @file check.go
// @brief Whether the requirements, the decisions and the tags agree.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"fmt"
	"sort"
	"strings"
)

// CheckOptions are what `rho req check` was asked.
type CheckOptions struct {
	// Base is a git revision; approvals made after it are checked against the
	// maintainers.
	Base string
}

// Check validates a workspace and returns the problems in the repositories it
// reports on.
func Check(w *Workspace, opts CheckOptions) Issues {
	all := append(Issues{}, w.Issues...)
	add := func(issue Issue) { all = append(all, issue) }

	// Ids unique across the family.
	ids := make([]string, 0, len(w.byID))
	for id := range w.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		found := w.byID[id]
		if id == "" || len(found) < 2 {
			continue
		}
		for _, req := range found[1:] {
			first := found[0]
			add(Issue{Severity: SeverityError, Code: "id.duplicate", Repo: req.Repo, File: req.File, Line: req.Line, ID: id,
				Message: fmt.Sprintf("%s is also declared in %s/%s:%d", id, first.Repo, first.File, first.Line)})
		}
	}

	for _, req := range w.AllRequirements() {
		if req.ID == "" {
			continue
		}
		levels := w.LevelsOf(req.Repo)
		own := LevelIndex(levels, req.Level)
		for _, target := range req.Satisfies {
			if target == req.ID {
				add(Issue{Severity: SeverityError, Code: "satisfies.self", Repo: req.Repo, File: req.File, Line: req.Line, ID: req.ID, Message: req.ID + " satisfies itself"})
				continue
			}
			parent := w.Requirement(target)
			if parent == nil {
				add(Issue{Severity: SeverityError, Code: "satisfies.unresolved", Repo: req.Repo, File: req.File, Line: req.Line, ID: req.ID,
					Message: fmt.Sprintf("%s satisfies %s, which no repository of the workspace declares", req.ID, target)})
				continue
			}
			theirs := LevelIndex(levels, parent.Level)
			if own >= 0 && (theirs < 0 || theirs >= own) {
				add(Issue{Severity: SeverityError, Code: "satisfies.level", Repo: req.Repo, File: req.File, Line: req.Line, ID: req.ID,
					Message: fmt.Sprintf("%s (%s) satisfies %s (%s); a requirement satisfies only a higher level", req.ID, req.Level, target, parent.Level)})
			}
			if Closed(parent.Status) && !Closed(req.Status) {
				add(Issue{Severity: SeverityWarning, Code: "satisfies.closed", Repo: req.Repo, File: req.File, Line: req.Line, ID: req.ID,
					Message: fmt.Sprintf("%s satisfies %s, which is %s", req.ID, target, parent.Status)})
			}
		}
		for _, ref := range req.DecidedBy {
			if !adrPattern.MatchString(ref) {
				add(Issue{Severity: SeverityError, Code: "decided-by.format", Repo: req.Repo, File: req.File, Line: req.Line, ID: req.ID,
					Message: fmt.Sprintf("%q is not ADR-NNNN or <repo>:ADR-NNNN", ref)})
				continue
			}
			if w.ADR(req.Repo, ref) == nil {
				add(Issue{Severity: SeverityError, Code: "decided-by.unresolved", Repo: req.Repo, File: req.File, Line: req.Line, ID: req.ID,
					Message: fmt.Sprintf("%s is decided by %s, which does not exist", req.ID, ref)})
			}
		}
		if req.Changed == "" {
			add(Issue{Severity: SeverityWarning, Code: "changed.missing", Repo: req.Repo, File: req.File, Line: req.Line, ID: req.ID,
				Message: req.ID + " has no changed: date"})
		}
	}

	// ADRs.
	adrIDs := map[string]*ADR{}
	referenced := map[string]bool{}
	for _, req := range w.AllRequirements() {
		for _, ref := range req.DecidedBy {
			if adr := w.ADR(req.Repo, ref); adr != nil {
				referenced[adr.Ref()] = true
			}
		}
	}
	for _, adr := range w.ADRs() {
		if adr.ID != "" {
			if other := adrIDs[adr.Ref()]; other != nil {
				add(Issue{Severity: SeverityError, Code: "adr.duplicate", Repo: adr.Repo, File: adr.File, ID: adr.ID,
					Message: fmt.Sprintf("%s is also %s", adr.ID, other.File)})
			}
			adrIDs[adr.Ref()] = adr
		}
		for _, id := range adr.Requirements {
			if w.Requirement(id) == nil {
				add(Issue{Severity: SeverityError, Code: "adr.requirements-unresolved", Repo: adr.Repo, File: adr.File, ID: adr.ID,
					Message: fmt.Sprintf("%s names the requirement %s, which does not exist", adr.ID, id)})
			}
		}
		for _, list := range [][]string{adr.Supersedes, adr.SupersededBy} {
			for _, ref := range list {
				if w.ADR(adr.Repo, ref) == nil {
					add(Issue{Severity: SeverityError, Code: "adr.supersedes-unresolved", Repo: adr.Repo, File: adr.File, ID: adr.ID,
						Message: fmt.Sprintf("%s names the ADR %s, which does not exist", adr.ID, ref)})
				}
			}
		}
		if adr.Status == "superseded" && len(adr.SupersededBy) == 0 {
			add(Issue{Severity: SeverityError, Code: "adr.superseded", Repo: adr.Repo, File: adr.File, ID: adr.ID,
				Message: adr.ID + " is superseded but names no superseded_by"})
		}
		if len(adr.Requirements) == 0 && !referenced[adr.Ref()] && adr.Status != "rejected" {
			add(Issue{Severity: SeverityWarning, Code: "adr.orphan", Repo: adr.Repo, File: adr.File, ID: adr.ID,
				Message: adr.ID + " is about no requirement: neither its requirements nor any decided_by names it"})
		}
	}

	// Cycles: through satisfies, and through supersedes.
	for _, cycle := range cycles(w) {
		add(cycle)
	}

	// Tags, and the evidence they give.
	for _, repo := range w.Reported() {
		for _, tag := range repo.Tags() {
			if len(tag.IDs) == 0 {
				add(Issue{Severity: SeverityWarning, Code: "tag.empty", Repo: tag.Repo, File: tag.File, Line: tag.Line, Message: "@" + tag.Kind + " names no requirement id"})
			}
			for _, id := range tag.IDs {
				req := w.Requirement(id)
				switch {
				case req == nil:
					add(Issue{Severity: SeverityError, Code: "tag.unresolved", Repo: tag.Repo, File: tag.File, Line: tag.Line, ID: id,
						Message: fmt.Sprintf("@%s %s names a requirement that does not exist", tag.Kind, id)})
				case Closed(req.Status):
					add(Issue{Severity: SeverityWarning, Code: "tag.closed", Repo: tag.Repo, File: tag.File, Line: tag.Line, ID: id,
						Message: fmt.Sprintf("@%s %s names a requirement that is %s", tag.Kind, id, req.Status)})
				}
			}
			if tag.Kind == TagVerifies && tag.Test != nil && tag.Test.Kind == "script" {
				add(Issue{Severity: SeverityWarning, Code: "tag.no-suite", Repo: tag.Repo, File: tag.File, Line: tag.Line,
					Message: "no suite in tests.yml runs this script, so its @verifies has no result"})
			}
		}
	}
	verifiedBy := map[string]int{}
	for _, repo := range w.Repos {
		for _, tag := range repo.Tags() {
			if tag.Kind == TagVerifies {
				for _, id := range tag.IDs {
					verifiedBy[id]++
				}
			}
		}
	}
	for _, req := range w.AllRequirements() {
		if !Committed(req.Status) || req.Verification != "test" {
			continue
		}
		if verifiedBy[req.ID] == 0 && len(openChildren(w, req.ID)) == 0 {
			add(Issue{Severity: SeverityWarning, Code: "evidence.missing", Repo: req.Repo, File: req.File, Line: req.Line, ID: req.ID,
				Message: fmt.Sprintf("%s is %s but no test verifies it and no lower-level requirement satisfies it", req.ID, req.Status)})
		}
	}

	if opts.Base != "" {
		for _, repo := range w.Reported() {
			all = append(all, CheckApprovals(w, repo, opts.Base)...)
		}
	}

	// Only what the command reports on, and only the family's.
	out := Issues{}
	for _, issue := range all {
		if issue.Repo != "" && (w.repos[issue.Repo] == nil || !w.IsReported(issue.Repo)) {
			continue
		}
		out = append(out, issue)
	}
	out.Sort()
	return out
}

// openChildren are the requirements below one that still ask something.
func openChildren(w *Workspace, id string) []*Requirement {
	out := []*Requirement{}
	for _, child := range w.Children(id) {
		if !Closed(child.Status) {
			out = append(out, child)
		}
	}
	return out
}

// cycles finds loops in satisfies and in supersedes.
func cycles(w *Workspace) Issues {
	out := Issues{}
	edges := map[string][]string{}
	place := map[string]Issue{}
	for _, req := range w.AllRequirements() {
		edges[req.ID] = append(edges[req.ID], req.Satisfies...)
		place[req.ID] = Issue{Repo: req.Repo, File: req.File, Line: req.Line}
	}
	for _, adr := range w.ADRs() {
		key := adr.Ref()
		for _, ref := range adr.Supersedes {
			if other := w.ADR(adr.Repo, ref); other != nil {
				edges[key] = append(edges[key], other.Ref())
			}
		}
		place[key] = Issue{Repo: adr.Repo, File: adr.File}
	}
	state := map[string]int{} // 1 visiting, 2 done
	reported := map[string]bool{}
	var stack []string
	var visit func(node string)
	visit = func(node string) {
		state[node] = 1
		stack = append(stack, node)
		for _, next := range edges[node] {
			switch state[next] {
			case 0:
				visit(next)
			case 1:
				start := 0
				for index, item := range stack {
					if item == next {
						start = index
					}
				}
				loop := append(append([]string{}, stack[start:]...), next)
				key := strings.Join(loop, ">")
				if !reported[key] {
					reported[key] = true
					issue := place[next]
					issue.Severity, issue.Code = SeverityError, "cycle"
					issue.Message = "a cycle: " + strings.Join(loop, " -> ")
					out = append(out, issue)
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = 2
	}
	keys := make([]string, 0, len(edges))
	for key := range edges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if state[key] == 0 {
			visit(key)
		}
	}
	return out
}
