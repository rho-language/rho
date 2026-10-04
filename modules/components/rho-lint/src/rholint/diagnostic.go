//
// @file diagnostic.go
// @brief What a finding is, the same whichever analyzer found it.
//
// This is the whole of what an analyzer's output is read into. clang-tidy writes
// `path:line:col: warning: text [-Wrule]`, cppcheck writes XML with a severity
// spelling of its own, a bespoke rule engine writes JSON of an inventing, and a
// CI dashboard wants something it can put in a table. One shape, read from all
// of them and written back out as SARIF, is what makes the analyzers swapable —
// and swapability is the reason to run more than one: an engine added later
// costs one adapter here rather than a change to every consumer of the output.
//
// Two decisions are load-bearing and both are about not losing things:
//
//   a finding says which engine found it. Two analyzers agreeing is worth one
//   line; two analyzers disagreeing is worth both, and a report that dropped the
//   attribution would show the same finding twice with no way to tell whether
//   that was a bug or a disagreement.
//
//   a finding may have no place. cppcheck reports a project-wide "unused
//   function" against the file it was configured in rather than the file the
//   function is in, and a message about the whole project has no line. Answering
//   with an empty location is what lets that finding through; refusing to parse it
//   would silently lose the one finding a build like that most wants.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Severity is how much a finding matters.
//
// Three, named after what an editor shows in its problems panel rather than after
// what an analyzer calls them: what the analyzer calls `error`, `warning`,
// `style` or `performance` is all one answer to the same question, and every
// analyzer's own spelling has to be mapped onto these three somewhere. This is
// that somewhere.
type Severity string

// The three answers to "how much does this matter".
const (
	// SeverityNote is something worth knowing that does not need fixing. A stale
	// comment, an unused import.
	SeverityNote Severity = "note"
	// SeverityWarning is a defect that will not stop the build and should be fixed.
	SeverityWarning Severity = "warning"
	// SeverityError is a defect the build ought to stop on.
	SeverityError Severity = "error"
	// SeverityNone is a finding recorded as present but not worth reporting, which
	// SARIF has and a severity list of three does not.
	//
	// Carried rather than dropped because an analyzer that says "this exists" with
	// no severity is answering a question — a suppressed diagnostic, a rule that
	// matched nothing — and dropping it makes the run look like it never asked.
	SeverityNone Severity = "none"
)

// Severities are the three a finding is normally reported at, most severe first.
//
// Ordered rather than a map because the run's exit code is a threshold and a
// threshold needs a line to compare against.
var Severities = []Severity{SeverityError, SeverityWarning, SeverityNote}

// Rank is this severity's position in Severities, counting from the most severe.
//
// The most severe is 0 so that a threshold is a `rank < threshold` rather than a
// list-index comparison somebody has to get the polarity of right. Anything not in
// the list ranks below all three, which is what makes `SeverityNone` the bottom.
func (s Severity) Rank() int {
	for index, known := range Severities {
		if known == s {
			return index
		}
	}
	return len(Severities)
}

// Known reports whether this is one of the three a finding is reported at.
//
// Not a validation gate: an analyzer's spelling that does not map is reported as
// a warning about the analyzer rather than as a failed finding, because losing a
// real defect to a mapping table is the worse of the two failures.
func (s Severity) Known() bool { return s.Rank() < len(Severities) }

// AtLeast reports whether this finding is severe enough to cross a threshold.
//
// The comparison is against a threshold the caller names, so `fail_on: warning`
// crosses on a warning and `fail_on: never` crosses on nothing. `never` is not a
// severity, which is why it is checked here rather than being the bottom of the
// list: a threshold of `none` meaning "fail on findings recorded as present" is
// never what anybody means.
func (s Severity) AtLeast(threshold Severity) bool {
	if threshold == ThresholdNever {
		return false
	}
	return s.Rank() <= threshold.Rank()
}

// ThresholdNever is the threshold at which a run never fails.
//
// A spelling rather than a severity, because `error` is the bottom of what a
// person means by a threshold only until they want none — and then `none`, which is
// itself a real SARIF level, is the obvious thing to type and would mean "fail on
// findings that are not worth reporting".
const ThresholdNever = "never"

// Diagnostic is one finding, the same whichever analyzer produced it.
//
// A finding is a place and a sentence and a severity and the name of the thing
// that noticed. Everything else an analyzer says — its own rule taxonomy, a
// confidence score, a fix-it hint — is either dropped here or kept in `Rule`,
// because a report that renders differently per analyzer is the thing this shape
// exists to prevent.
type Diagnostic struct {
	// Rule is what the finding is called, which is what a suppression is written
	// against and what `--filter` matches on.
	//
	// Carries the engine's own naming, qualified: `clang-tidy-bugprone-...`,
	// `cppcheck-unusedVariable`, `rho-shadowed-binding`. The qualification is not
	// decoration. `unusedVariable` and `unusedFunction` are different checks in
	// different analyzers, and a project that suppresses one of them has said
	// nothing about the other; an unqualified name would let it.
	Rule string `json:"rule"`
	// Message is what a person reads. The analyzer's own sentence, kept verbatim,
	// because a rewritten sentence is one nobody can search for in the analyzer's
	// own documentation.
	Message string `json:"message"`
	// Severity is how much it matters.
	Severity Severity `json:"severity"`
	// File is where it is, absolute when the analyzer gave an absolute path and
	// relative to the project root when it did not. Normalized to relative here
	// because a report that says the same finding twice under two paths — once
	// absolute, once relative — reads as two findings, and a SARIF `uri` that
	// differs by machine is a report code scanning cannot attribute.
	//
	// Empty when the analyzer named no file, which is a real answer from cppcheck
	// and not a failure.
	File string `json:"file,omitempty"`
	// Line and Column are one-based, which is what every analyzer prints and what
	// every editor jumps to. Converted on the way in rather than in every reader,
	// because a zero-based column is the kind of thing that is off by one in a
	// tool nobody reads the source of.
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
	// EndLine and EndColumn are where the finding stops. Zero when the analyzer did
	// not say, and a run that reports a zero-length region shows the single
	// character the finding starts at.
	EndLine   int `json:"end_line,omitempty"`
	EndColumn int `json:"end_column,omitempty"`
	// Engine is what found it, which is what makes two analyzers disagreeing
	// legible.
	Engine string `json:"engine"`
}

// Key identifies a finding, for de-duplicating and for ordering.
//
// Rule, message and place, and not the severity: two analyzers that report the
// same defect at the same line with the same sentence are reporting one defect,
// and a report that showed it twice would make it look like two. The severity is
// left out because the whole of that case is the two disagreeing.
func (d Diagnostic) Key() string {
	return strings.Join([]string{d.Rule, d.File, strconv.Itoa(d.Line), strconv.Itoa(d.Column), d.Message}, "\x00")
}

// Where is the finding's place as one line, for a report that prints one.
func (d Diagnostic) Where() string {
	switch {
	case d.File == "":
		return "(no file)"
	case d.Line == 0:
		return d.File
	case d.Column == 0:
		return d.File + ":" + strconv.Itoa(d.Line)
	default:
		return d.File + ":" + strconv.Itoa(d.Line) + ":" + strconv.Itoa(d.Column)
	}
}

// String is one finding as a report prints it, in the gcc spelling.
//
// The gcc spelling rather than this component's own, because every toolchain
// prints findings this way and a person reads them in pipelines that have been
// highlighting that shape for thirty years.
func (d Diagnostic) String() string {
	out := d.Where() + ": " + string(d.Severity) + ": " + d.Message
	if d.Rule != "" {
		out += " [" + d.Rule + "]"
	}
	return out
}

// Absolute is the finding's place as an absolute path, or "" when it named none.
//
// Answered against a project root rather than by resolving the current directory,
// because the run's root is where the paths in the compilation database are
// relative to and a finding resolved against anything else points at a file that
// is not there.
func (d Diagnostic) Absolute(root string) string {
	if d.File == "" {
		return ""
	}
	if filepath.IsAbs(d.File) {
		return filepath.Clean(d.File)
	}
	return filepath.Clean(filepath.Join(root, d.File))
}

/* -------------------------------------------------------------------------- */
/*  Ordering and merging                                                       */
/* -------------------------------------------------------------------------- */

// Sort puts findings in the order a report shows them: by place, then by severity
// with the worst first, then by rule, then by the sentence.
//
// Place first because that is the order a person reads in and the order that makes
// two runs of the same tree diffable. Severity before rule because within one line
// the worst thing on it is the thing being read. Total, with the sentence last, so
// that a report of the same tree twice is byte-identical: a pipeline that diffs
// two runs needs a total order to diff.
func Sort(found []Diagnostic) {
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Severity != b.Severity {
			return a.Severity.Rank() < b.Severity.Rank()
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
}

// Merge is every engine's findings in one list, sorted and with the duplicates
// taken out.
//
// Duplicates are taken out by Key, which means two analyzers that reported the
// same defect with the same wording produce one finding — attributed to the first
// engine to report it, because the engine list is in a fixed order and so is the
// answer. Two analyzers reporting the same place with different wording are two
// findings, because they are two claims and somebody has to decide which to act
// on; collapsing those would be discarding the disagreement that is the reason to
// run two analyzers at all.
func Merge(perEngine [][]Diagnostic) []Diagnostic {
	merged := []Diagnostic{}
	seen := map[string]bool{}
	for _, found := range perEngine {
		for _, one := range found {
			key := one.Key()
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, one)
		}
	}
	Sort(merged)
	return merged
}

// Counts is what a run found, by severity.
type Counts struct {
	Errors   int
	Warnings int
	Notes    int
	// Others is every finding whose severity is not one of the three, which is
	// counted rather than dropped so that a run whose analyzers all speak a
	// spelling this component has not met still says how many it read.
	Others int
}

// Of is what a list of findings holds.
func Of(found []Diagnostic) Counts {
	counts := Counts{}
	for _, one := range found {
		switch one.Severity {
		case SeverityError:
			counts.Errors++
		case SeverityWarning:
			counts.Warnings++
		case SeverityNote:
			counts.Notes++
		default:
			counts.Others++
		}
	}
	return counts
}

// AtLeast reports whether any finding in this list crosses a threshold.
func (c Counts) AtLeast(threshold Severity) bool {
	switch {
	case threshold == ThresholdNever:
		return false
	case threshold == SeverityError:
		return c.Errors > 0
	case threshold == SeverityWarning:
		return c.Errors > 0 || c.Warnings > 0
	default:
		return c.Errors+c.Warnings+c.Notes+c.Others > 0
	}
}

// Total is every finding in the list.
func (c Counts) Total() int { return c.Errors + c.Warnings + c.Notes + c.Others }

// Plural is a count and its noun, which is one place so that a report cannot say
// "1 warnings".
func Plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(count) + " " + noun + "s"
}

// SeverityText is one severity as the phrase a report uses about it.
func (c Counts) SeverityText() string {
	parts := []string{}
	if c.Errors > 0 {
		parts = append(parts, Plural(c.Errors, "error"))
	}
	if c.Warnings > 0 {
		parts = append(parts, Plural(c.Warnings, "warning"))
	}
	if c.Notes > 0 {
		parts = append(parts, Plural(c.Notes, "note"))
	}
	if c.Others > 0 {
		parts = append(parts, Plural(c.Others, "finding"))
	}
	return strings.Join(parts, ", ")
}
