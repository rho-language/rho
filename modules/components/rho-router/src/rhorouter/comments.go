//
// @file comments.go
// @brief Attaching a declaration's comment to the declaration.
//
// The compiler's lexer discards comments, and rightly so: a comment is not a
// value and a program does not run on one. A documentation tool runs on
// nothing else, though, so a route that cannot describe itself produces a
// document of blank entries.
//
// The comments are therefore read from the source text, by position: the run of
// lines immediately above a declaration, stopping at a blank line. That is the
// convention the whole tree already writes in — a declaration's comment is
// directly above it, and a blank line ends it — so following it needs no
// grammar and no lexer change, and it costs the compiler nothing.
//
// The first line is the summary and the rest is the description, which is the
// split a documentation tool already knows. A route comment that runs on for
// several paragraphs reads as a paragraph, which is what it is.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhorouter

import "strings"

// commentIndex holds the comment runs a source carries, keyed by the line each
// one ends on, which is the line a declaration starts on.
type commentIndex struct {
	// runs maps a one-based line number to the comment block ending on it.
	runs map[int][]string
	// lines is the source, split once so a position is a lookup.
	lines []string
}

// newCommentIndex reads every comment run in a source.
//
// A run is recorded against the line that ends it, which is the line the
// declaration following it is written on: the run on lines 10 to 13 belongs to
// the declaration on line 14. Recording it against the last comment line would
// mean finding the declaration's line and stepping back, and a declaration
// written with a blank line after its comment would then be attached to
// nothing.
func newCommentIndex(source string) *commentIndex {
	index := &commentIndex{runs: map[int][]string{}, lines: strings.Split(source, "\n")}
	var run []string
	for i, line := range index.lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			run = append(run, strings.TrimSpace(strings.TrimPrefix(trimmed, "//")))
			continue
		}
		// Anything else ends the run. A blank line ends it too: a comment
		// separated from a declaration by a blank line is a note about the
		// section, not the declaration's own description, and attaching it
		// would put prose where a reader expects a heading.
		index.close(run, i+1)
		run = nil
	}
	// A file that ends inside a comment still has one, belonging to nothing.
	index.close(run, len(index.lines)+1)
	return index
}

// close records a finished run against the line it ended on.
func (c *commentIndex) close(run []string, endLine int) {
	if len(run) == 0 {
		return
	}
	c.runs[endLine] = trimBlankEdges(run)
}

// trimBlankEdges drops the blank lines a wrapped comment leaves at its edges,
// which are layout rather than prose.
func trimBlankEdges(run []string) []string {
	start, end := 0, len(run)
	for start < end && run[start] == "" {
		start++
	}
	for end > start && run[end-1] == "" {
		end--
	}
	return run[start:end]
}

// above returns the comment block ending on a line, as one string.
func (c *commentIndex) above(line int) string {
	run, found := c.runs[line]
	if !found {
		return ""
	}
	return strings.Join(run, "\n")
}

// splitSummary divides a comment into the line that heads it and the prose that
// follows, which is the shape a documentation entry takes.
func splitSummary(comment string) (string, string) {
	trimmed := strings.TrimSpace(comment)
	if trimmed == "" {
		return "", ""
	}
	lines := strings.Split(trimmed, "\n")
	summary := strings.TrimSpace(lines[0])
	if len(lines) == 1 {
		// A one-line comment is the heading, not a heading repeated. Repeating
		// it puts the same sentence where a reader expects the prose under it.
		return summary, ""
	}
	return summary, strings.TrimSpace(strings.Join(lines[1:], "\n"))
}

/* -------------------------------------------------------------------------- */
/*  Finding a declaration                                                      */
/* -------------------------------------------------------------------------- */

// The AST records no source positions, so a declaration is found by its own
// opening words. A source that declares the same name twice is a duplicate
// definition the compiler refuses anyway, so the first match is the one a build
// would have used.

// lineOfFunction returns the one-based line a function is declared on.
func (c *commentIndex) lineOfFunction(name string) int {
	needle := "fn " + name + "("
	for i, line := range c.lines {
		if strings.Contains(line, needle) {
			return i + 1
		}
	}
	return 0
}

// lineOfRoute returns the one-based line a route is declared on, which the
// method and the path identify together: a module may serve the same path twice
// under two methods, and those are two declarations on two lines.
func (c *commentIndex) lineOfRoute(method, path string) int {
	needle := "route " + method + " " + quote(path)
	for i, line := range c.lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, needle) || strings.Contains(trimmed, needle) {
			return i + 1
		}
	}
	return 0
}

// quote spells a path the way a source spells it, so a route can be found in the
// text it was written in rather than in a re-quoted guess at it.
func quote(path string) string {
	if strings.ContainsAny(path, "\"") {
		return path
	}
	return "\"" + path + "\""
}
