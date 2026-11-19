//
// @file trace.go
// @brief Each requirement, traced to what satisfies it, decides it, implements it and verifies it.
//
// A requirement's evidence is its own verifying tests and the evidence of the
// open requirements below it that satisfy it:
//
//   - failing  — a verifying test failed, or a child's evidence is failing
//   - not run  — there are tests, none failed, and some have no result
//   - passing  — there is evidence, and all of it passed
//   - none     — no verifying test and no open child
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
)

// Evidence states.
const (
	EvidencePassing = "passing"
	EvidenceFailing = "failing"
	EvidenceNotRun  = "not run"
	EvidenceNone    = "none"
)

// Location is a place in a repository.
type Location struct {
	Repo   string `json:"repo"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol,omitempty"`
}

// TestEvidence is one verifying test and its latest result.
type TestEvidence struct {
	Location
	Test   string `json:"test"`
	Kind   string `json:"kind"`
	Result string `json:"result"`
	Report string `json:"report,omitempty"`
}

// Row is one requirement of the matrix.
type Row struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Level        string         `json:"level"`
	Status       string         `json:"status"`
	Verification string         `json:"verification"`
	Repo         string         `json:"repo"`
	Satisfies    []string       `json:"satisfies"`
	Children     []string       `json:"children"`
	ADRs         []string       `json:"adrs"`
	Code         []Location     `json:"code"`
	Tests        []TestEvidence `json:"tests"`
	Evidence     string         `json:"evidence"`
}

// Gap is something the trace found missing.
type Gap struct {
	Code    string `json:"code"`
	ID      string `json:"id,omitempty"`
	Repo    string `json:"repo,omitempty"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
	// Blocking gaps fail `trace --release`.
	Blocking bool `json:"blocking"`
}

// Matrix is the trace.
type Matrix struct {
	Rows []Row `json:"requirements"`
	Gaps []Gap `json:"gaps"`
}

// Blocking counts the gaps that fail a release.
func (m *Matrix) Blocking() int {
	count := 0
	for _, gap := range m.Gaps {
		if gap.Blocking {
			count++
		}
	}
	return count
}

// Row is the row of a requirement, or nil.
func (m *Matrix) Row(id string) *Row {
	for index := range m.Rows {
		if m.Rows[index].ID == id {
			return &m.Rows[index]
		}
	}
	return nil
}

// Trace builds the matrix for the requirements of the reported repositories,
// with evidence drawn from the whole family.
func Trace(w *Workspace) *Matrix {
	code := map[string][]Location{}
	tests := map[string][]TestEvidence{}
	matrix := &Matrix{Rows: []Row{}, Gaps: []Gap{}}
	for _, repo := range w.Repos {
		results := repo.Results()
		for _, tag := range repo.Tags() {
			at := Location{Repo: tag.Repo, File: tag.File, Line: tag.Line, Symbol: tag.Symbol}
			for _, id := range tag.IDs {
				if w.Requirement(id) == nil && w.IsReported(repo.Name) {
					matrix.Gaps = append(matrix.Gaps, Gap{Code: "dangling-tag", ID: id, Repo: tag.Repo, File: tag.File, Line: tag.Line,
						Message: fmt.Sprintf("@%s %s points at no requirement", tag.Kind, id), Blocking: true})
				}
				if tag.Kind == TagReq {
					code[id] = append(code[id], at)
					continue
				}
				evidence := TestEvidence{Location: at, Result: ResultNotRun}
				if tag.Test != nil {
					evidence.Test, evidence.Kind = tag.Test.Name, tag.Test.Kind
					evidence.Result, evidence.Report = results.Lookup(tag.Test, tag.File)
				}
				tests[id] = append(tests[id], evidence)
			}
		}
	}

	memo := map[string]string{}
	var evidenceOf func(id string, seen map[string]bool) string
	evidenceOf = func(id string, seen map[string]bool) string {
		if found, ok := memo[id]; ok {
			return found
		}
		if seen[id] {
			return EvidenceNone
		}
		seen[id] = true
		states := []string{}
		for _, t := range tests[id] {
			switch t.Result {
			case ResultPass:
				states = append(states, EvidencePassing)
			case ResultFail:
				states = append(states, EvidenceFailing)
			default:
				states = append(states, EvidenceNotRun)
			}
		}
		for _, child := range openChildren(w, id) {
			if state := evidenceOf(child.ID, seen); state != EvidenceNone {
				states = append(states, state)
			} else {
				states = append(states, EvidenceNotRun)
			}
		}
		result := combine(states)
		memo[id] = result
		return result
	}

	referenced := map[string]bool{}
	for _, req := range w.AllRequirements() {
		for _, ref := range req.DecidedBy {
			if adr := w.ADR(req.Repo, ref); adr != nil {
				referenced[adr.Ref()] = true
			}
		}
	}

	for _, repo := range w.Reported() {
		for _, req := range repo.Requirements() {
			row := Row{ID: req.ID, Title: req.Title, Level: req.Level, Status: req.Status, Verification: req.Verification, Repo: req.Repo,
				Satisfies: req.Satisfies, Children: []string{}, ADRs: []string{}, Code: nonNilLoc(code[req.ID]), Tests: nonNilTests(tests[req.ID])}
			for _, child := range w.Children(req.ID) {
				row.Children = append(row.Children, child.ID)
			}
			row.ADRs = adrsOf(w, req)
			row.Evidence = evidenceOf(req.ID, map[string]bool{})
			matrix.Rows = append(matrix.Rows, row)
			matrix.Gaps = append(matrix.Gaps, gapsOf(req, row)...)
		}
		for _, adr := range repo.ADRs {
			if len(adr.Requirements) == 0 && !referenced[adr.Ref()] && adr.Status != "rejected" {
				matrix.Gaps = append(matrix.Gaps, Gap{Code: "orphan-adr", ID: adr.ID, Repo: adr.Repo, File: adr.File,
					Message: adr.ID + " satisfies no requirement"})
			}
		}
	}
	sort.SliceStable(matrix.Gaps, func(a, b int) bool {
		x, y := matrix.Gaps[a], matrix.Gaps[b]
		if x.Repo != y.Repo {
			return x.Repo < y.Repo
		}
		if x.File != y.File {
			return x.File < y.File
		}
		return x.Line < y.Line
	})
	return matrix
}

func combine(states []string) string {
	if len(states) == 0 {
		return EvidenceNone
	}
	result := EvidencePassing
	for _, state := range states {
		if state == EvidenceFailing {
			return EvidenceFailing
		}
		if state != EvidencePassing {
			result = EvidenceNotRun
		}
	}
	return result
}

// adrsOf are the ADRs about a requirement: those it is decided by, and those that
// name it.
func adrsOf(w *Workspace, req *Requirement) []string {
	out := []string{}
	for _, ref := range req.DecidedBy {
		if adr := w.ADR(req.Repo, ref); adr != nil {
			out = append(out, RefFrom(req.Repo, adr))
		}
	}
	for _, adr := range w.ADRs() {
		if contains(adr.Requirements, req.ID) && !contains(out, RefFrom(req.Repo, adr)) {
			out = append(out, RefFrom(req.Repo, adr))
		}
	}
	return out
}

func gapsOf(req *Requirement, row Row) []Gap {
	gap := func(code string, blocking bool, format string, args ...any) Gap {
		return Gap{Code: code, ID: req.ID, Repo: req.Repo, File: req.File, Line: req.Line, Message: fmt.Sprintf(format, args...), Blocking: blocking}
	}
	out := []Gap{}
	if !Committed(req.Status) {
		return out
	}
	byHuman := req.Verification != "test" && req.Status == StatusVerified
	if req.Status == StatusVerified && req.Verification == "test" && row.Evidence != EvidencePassing {
		return append(out, gap("verified-unproven", true, "%s is verified but its evidence is %s", req.ID, row.Evidence))
	}
	switch row.Evidence {
	case EvidenceNone:
		if req.Verification == "test" {
			out = append(out, gap("untested", true, "%s is %s and has no verifying test and no lower-level requirement", req.ID, req.Status))
		} else if !byHuman {
			out = append(out, gap("unverified", true, "%s is verified by %s and is not yet verified", req.ID, req.Verification))
		}
	case EvidenceFailing:
		out = append(out, gap("failing", true, "%s is %s and its evidence is failing", req.ID, req.Status))
	case EvidenceNotRun:
		if !byHuman {
			out = append(out, gap("not-run", true, "%s is %s and some of its tests have no result", req.ID, req.Status))
		}
	}
	return out
}

func nonNilLoc(list []Location) []Location {
	if list == nil {
		return []Location{}
	}
	return list
}

func nonNilTests(list []TestEvidence) []TestEvidence {
	if list == nil {
		return []TestEvidence{}
	}
	return list
}
