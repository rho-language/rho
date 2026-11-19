//
// @file tsv.go
// @brief `--tsv`: one record per line, for a program that reads lines, not JSON.
//
// A dashboard written in a language without a JSON reader (the RoboHub keeper,
// which is Rho) reads the same answers `--json` gives as tab-separated records.
// Every line starts with the record's kind, so a line that is not a record — a
// log line, an error on standard error that lands in the same file — is told
// apart by its first field and skipped. A field is escaped: a backslash, a tab,
// a line feed and a carriage return become `\\`, `\t`, `\n` and `\r`, so a
// record is always one line and a field never holds a tab. A list is its items
// joined by commas; a yes/no is `yes` or `no`.
//
// The records, by command:
//
//	repo    name dir prefix parent maintainers levels         (list, trace, check, adr list)
//	req     id repo level status verification title satisfies tags file line owner changed
//	statement text / rationale text / decided_by list / evidence state   (show)
//	up      depth id title level status                        (show; what it satisfies)
//	down    depth id title level status                        (show; what satisfies it)
//	adr     repo id status date title requirements supersedes superseded_by file
//	code    repo file line symbol
//	test    id result repo file line test kind report         (show, trace)
//	row     id repo level status verification evidence title satisfies children adrs code tests
//	gap     code blocking id repo file line message
//	issue   severity code repo file line id message
//	summary (trace) blocking release_ready | (check) ok errors warnings requirements
//	created id repo file                                       (new, adr new)
//	updated id file status reopened                            (edit, set-status, adr set-status)
//	superseded old new                                         (adr supersede)
//	error   message                                            (any command that failed)
//
// @project Rho
// @version 0.3
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"io"
	"strconv"
	"strings"
)

var tsvEscaper = strings.NewReplacer(`\`, `\\`, "\t", `\t`, "\n", `\n`, "\r", `\r`)

// TSVEscape is one field as a record carries it.
func TSVEscape(value string) string { return tsvEscaper.Replace(value) }

// TSVUnescape is the inverse of TSVEscape.
func TSVUnescape(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	var b strings.Builder
	for i := 0; i < len(field); i++ {
		if field[i] != '\\' || i+1 == len(field) {
			b.WriteByte(field[i])
			continue
		}
		i++
		switch field[i] {
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		default:
			b.WriteByte(field[i])
		}
	}
	return b.String()
}

// tsvRecord writes one record: its kind, then its fields, escaped.
func tsvRecord(out io.Writer, kind string, fields ...any) {
	parts := make([]string, 0, len(fields)+1)
	parts = append(parts, kind)
	for _, field := range fields {
		var text string
		switch value := field.(type) {
		case string:
			text = value
		case int:
			text = strconv.Itoa(value)
		case bool:
			text = "no"
			if value {
				text = "yes"
			}
		case []string:
			text = strings.Join(value, ",")
		default:
			text = ""
		}
		parts = append(parts, TSVEscape(text))
	}
	io.WriteString(out, strings.Join(parts, "\t")+"\n")
}

func tsvRepos(out io.Writer, w *Workspace) {
	for _, repo := range w.Reported() {
		levels := []string{}
		for _, level := range repo.Levels {
			levels = append(levels, level.Name)
		}
		tsvRecord(out, "repo", repo.Name, repo.Dir, repo.Prefix, repo.Parent, repo.Maintainers, levels)
	}
}

func tsvRequirement(out io.Writer, req *Requirement) {
	tsvRecord(out, "req", req.ID, req.Repo, req.Level, req.Status, req.Verification, req.Title,
		req.Satisfies, req.Tags, req.File, req.Line, req.Owner, req.Changed)
}

func tsvTest(out io.Writer, id string, t TestEvidence) {
	tsvRecord(out, "test", id, t.Result, t.Repo, t.File, t.Line, t.Test, t.Kind, t.Report)
}

func tsvADR(out io.Writer, adr *ADR) {
	tsvRecord(out, "adr", adr.Repo, adr.ID, adr.Status, adr.Date, adr.Title, adr.Requirements, adr.Supersedes, adr.SupersededBy, adr.File)
}

// tsvShow is `rho req show --tsv`.
func tsvShow(out io.Writer, w *Workspace, req *Requirement, row *Row) {
	tsvRequirement(out, req)
	tsvRecord(out, "statement", strings.TrimSpace(req.Statement))
	if req.Rationale != "" {
		tsvRecord(out, "rationale", strings.TrimSpace(req.Rationale))
	}
	tsvRecord(out, "decided_by", req.DecidedBy)
	seen := map[string]bool{req.ID: true}
	var up func(id string, depth int)
	up = func(id string, depth int) {
		current := w.Requirement(id)
		if current == nil {
			return
		}
		for _, parent := range current.Satisfies {
			found := w.Requirement(parent)
			if found == nil {
				tsvRecord(out, "up", depth, parent, "", "", "missing")
				continue
			}
			tsvRecord(out, "up", depth, parent, found.Title, found.Level, found.Status)
			if !seen[parent] {
				seen[parent] = true
				up(parent, depth+1)
			}
		}
	}
	up(req.ID, 0)
	visited := map[string]bool{}
	var down func(id string, depth int)
	down = func(id string, depth int) {
		if visited[id] {
			return
		}
		visited[id] = true
		for _, child := range w.Children(id) {
			tsvRecord(out, "down", depth, child.ID, child.Title, child.Level, child.Status)
			down(child.ID, depth+1)
		}
	}
	down(req.ID, 0)
	for _, ref := range row.ADRs {
		if adr := w.ADR(req.Repo, ref); adr != nil {
			tsvADR(out, adr)
		}
	}
	for _, at := range row.Code {
		tsvRecord(out, "code", at.Repo, at.File, at.Line, at.Symbol)
	}
	for _, t := range row.Tests {
		tsvTest(out, req.ID, t)
	}
	tsvRecord(out, "evidence", row.Evidence)
}

// tsvTrace is `rho req trace --tsv`.
func tsvTrace(out io.Writer, w *Workspace, matrix *Matrix) {
	tsvRepos(out, w)
	for _, row := range matrix.Rows {
		tsvRecord(out, "row", row.ID, row.Repo, row.Level, row.Status, row.Verification, row.Evidence, row.Title,
			row.Satisfies, row.Children, row.ADRs, len(row.Code), len(row.Tests))
		for _, t := range row.Tests {
			tsvTest(out, row.ID, t)
		}
	}
	for _, gap := range matrix.Gaps {
		tsvRecord(out, "gap", gap.Code, gap.Blocking, gap.ID, gap.Repo, gap.File, gap.Line, gap.Message)
	}
	blocking := matrix.Blocking()
	tsvRecord(out, "summary", blocking, blocking == 0)
}

// tsvCheck is `rho req check --tsv`.
func tsvCheck(out io.Writer, w *Workspace, issues Issues, ok bool) {
	tsvRepos(out, w)
	for _, issue := range issues {
		tsvRecord(out, "issue", issue.Severity, issue.Code, issue.Repo, issue.File, issue.Line, issue.ID, issue.Message)
	}
	count := 0
	for _, repo := range w.Reported() {
		count += len(repo.Requirements())
	}
	tsvRecord(out, "summary", ok, issues.Errors(), issues.Warnings(), count)
}
