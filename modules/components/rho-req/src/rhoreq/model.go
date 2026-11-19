//
// @file model.go
// @brief Requirements, decisions, levels and the problems found in them.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

// Package rhoreq reads the requirements a repository declares, the architecture
// decisions recorded against them, and the tags in code and tests that point at
// them; it checks that the three agree, traces each requirement to its evidence,
// and makes the edits a person would otherwise make by hand without disturbing
// anything else in the files.
package rhoreq

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Level is one rung of the requirement hierarchy.
type Level struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

// DefaultLevels are the levels when no levels.yml is found.
var DefaultLevels = []Level{
	{Name: "SYS", Title: "System"},
	{Name: "NODE", Title: "Node"},
	{Name: "SUB", Title: "Subsystem"},
	{Name: "UNIT", Title: "Unit"},
}

// LevelIndex is the position of a level, 0 for the highest, or -1.
func LevelIndex(levels []Level, name string) int {
	for index, level := range levels {
		if level.Name == name {
			return index
		}
	}
	return -1
}

// The statuses a requirement moves through.
const (
	StatusDraft       = "draft"
	StatusProposed    = "proposed"
	StatusApproved    = "approved"
	StatusImplemented = "implemented"
	StatusVerified    = "verified"
	StatusRejected    = "rejected"
	StatusRetired     = "retired"
)

// Statuses is every requirement status, in lifecycle order.
var Statuses = []string{StatusDraft, StatusProposed, StatusApproved, StatusImplemented, StatusVerified, StatusRejected, StatusRetired}

// Verifications are the ways a requirement is shown to hold.
var Verifications = []string{"test", "analysis", "inspection", "demonstration"}

// ADRStatuses are the states of an architecture decision record.
var ADRStatuses = []string{"proposed", "accepted", "superseded", "rejected"}

// Committed reports whether a status is one a maintainer had to approve.
func Committed(status string) bool {
	return status == StatusApproved || status == StatusImplemented || status == StatusVerified
}

// Closed reports whether a requirement no longer asks anything.
func Closed(status string) bool {
	return status == StatusRejected || status == StatusRetired
}

// transitions are the status changes `set-status` makes without --force.
var transitions = map[string][]string{
	StatusDraft:       {StatusProposed},
	StatusProposed:    {StatusDraft, StatusApproved},
	StatusApproved:    {StatusProposed, StatusImplemented},
	StatusImplemented: {StatusProposed, StatusApproved, StatusVerified},
	StatusVerified:    {StatusProposed, StatusImplemented},
	StatusRejected:    {StatusDraft},
	StatusRetired:     {},
}

// CanTransition reports whether a requirement may move from one status to another.
// Any status may become rejected or retired.
func CanTransition(from, to string) bool {
	if to == StatusRejected || to == StatusRetired {
		return from != to
	}
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// Requirement is one requirement as a file declares it.
type Requirement struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Statement    string   `json:"statement"`
	Rationale    string   `json:"rationale,omitempty"`
	Satisfies    []string `json:"satisfies"`
	Status       string   `json:"status"`
	Verification string   `json:"verification"`
	DecidedBy    []string `json:"decided_by"`
	Owner        string   `json:"owner,omitempty"`
	Changed      string   `json:"changed,omitempty"`
	Tags         []string `json:"tags"`

	Level string `json:"level"`
	Repo  string `json:"repo"`
	File  string `json:"file"`
	Line  int    `json:"line"`
}

// ADR is an architecture decision record's front matter.
type ADR struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Date         string   `json:"date"`
	Requirements []string `json:"requirements"`
	Supersedes   []string `json:"supersedes"`
	SupersededBy []string `json:"superseded_by"`

	Repo string `json:"repo"`
	File string `json:"file"`
}

// Ref is the ADR's name as a repository other than its own writes it.
func (a *ADR) Ref() string { return a.Repo + ":" + a.ID }

// Severity is how much a problem matters.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Issue is one problem found.
type Issue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Repo     string `json:"repo,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	ID       string `json:"id,omitempty"`
	Message  string `json:"message"`
}

func (i Issue) String() string {
	place := i.File
	if i.Repo != "" && place != "" {
		place = i.Repo + "/" + place
	}
	if i.Line > 0 {
		place = fmt.Sprintf("%s:%d", place, i.Line)
	}
	if place != "" {
		place += ": "
	}
	return fmt.Sprintf("%s: %s%s [%s]", i.Severity, place, i.Message, i.Code)
}

// Issues is a list of problems.
type Issues []Issue

// Errors counts the errors.
func (list Issues) Errors() int {
	count := 0
	for _, issue := range list {
		if issue.Severity == SeverityError {
			count++
		}
	}
	return count
}

// Warnings counts the warnings.
func (list Issues) Warnings() int { return len(list) - list.Errors() }

// Sort orders issues by place.
func (list Issues) Sort() {
	sort.SliceStable(list, func(a, b int) bool {
		x, y := list[a], list[b]
		if x.Repo != y.Repo {
			return x.Repo < y.Repo
		}
		if x.File != y.File {
			return x.File < y.File
		}
		if x.Line != y.Line {
			return x.Line < y.Line
		}
		return x.Code < y.Code
	})
}

// idPattern is a requirement id: a prefix of upper-case words joined by dashes,
// and four digits.
var idPattern = regexp.MustCompile(`^([A-Z][A-Z0-9]*(?:-[A-Z][A-Z0-9]*)*)-([0-9]{4})$`)

// prefixPattern is a file's or a repository's id prefix.
var prefixPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:-[A-Z][A-Z0-9]*)*$`)

// adrPattern is an ADR id, local (`ADR-0009`) or qualified by its repository
// (`robohub:ADR-0009`).
var adrPattern = regexp.MustCompile(`^(?:([A-Za-z0-9][A-Za-z0-9._-]*):)?(ADR-[0-9]{4})$`)

// datePattern is the date format every date in these files uses.
var datePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

// SplitID is an id's prefix and number.
func SplitID(id string) (prefix string, number int, ok bool) {
	match := idPattern.FindStringSubmatch(id)
	if match == nil {
		return "", 0, false
	}
	fmt.Sscanf(match[2], "%d", &number)
	return match[1], number, true
}

// splitList reads `A,B C` as [A B C].
func splitList(text string) []string {
	out := []string{}
	for _, field := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
