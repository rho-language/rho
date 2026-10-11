//
// @file help.go
// @brief What an editor asks for when the cursor is on a keyword.
//
// The language reference is a page, and a person reading a page is not the only
// reader: an editor showing a hover beside `for` wants the `for` part of it and
// a way to the rest. The part is taken from the page rather than written again,
// so the hover and the page cannot say different things — the page is split into
// its headings, and a keyword names the heading that explains it.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Help is one syntax element's place in the reference: the section that explains
// it and the page to read the rest on.
type Help struct {
	// Keyword is the word that was asked about.
	Keyword string `json:"keyword"`
	// Title is the heading of the section that explains it.
	Title string `json:"title"`
	// Markdown is that section, as it is written.
	Markdown string `json:"markdown"`
	// Route is the page within the documentation, for a link to the full view.
	Route string `json:"route"`
}

// syntaxTopics maps a keyword to the heading of the section that explains it.
// The reference groups a few words under one heading — `for`, `while` and `if`
// are all control flow — so the section is what a hover shows rather than a
// sentence written twice.
var syntaxTopics = map[string]string{
	"module": "Source files, and the top of a file", "namespace": "Source files, and the top of a file",
	"feature": "Source files, and the top of a file", "include": "Source files, and the top of a file",
	"import": "Source files, and the top of a file",
	"fn":     "Functions", "extern": "Functions", "main": "Functions",
	"let": "Bindings", "var": "Bindings", "const": "Bindings",
	"type":   "Types",
	"struct": "Structs",
	"if":     "Control flow", "else": "Control flow", "while": "Control flow",
	"for": "Control flow", "match": "Control flow", "break": "Control flow",
	"return": "Control flow", "reverse": "Control flow", "step": "Control flow",
	"route": "Routes",
	"Ptr":   "Pointers",
	"enum":  "Protocol declarations", "message": "Protocol declarations", "rpc": "Protocol declarations",
	"sizeof": "Expressions and operators", "true": "Expressions and operators", "false": "Expressions and operators",
}

// HelpFor returns the help for a keyword, and whether the reference has any.
func HelpFor(word string) (*Help, bool) {
	title, known := syntaxTopics[word]
	if !known {
		return nil, false
	}
	body, found := syntaxSections()[title]
	if !found {
		return nil, false
	}
	return &Help{
		Keyword:  word,
		Title:    title,
		Markdown: body,
		Route:    "language/syntax",
	}, true
}

// HelpKeywords lists the words help is available for, so a reader can see the
// set rather than guess at it.
func HelpKeywords() []string {
	words := make([]string, 0, len(syntaxTopics))
	for word := range syntaxTopics {
		words = append(words, word)
	}
	sort.Strings(words)
	return words
}

// syntaxSections reads the language reference and splits it by its level-two
// headings, giving each section's markdown. The page is read here rather than
// rewritten, so what a hover shows is what the page says.
func syntaxSections() map[string]string {
	path := filepath.Join(docsRoot(), "language", "syntax.md")
	source, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	sections := map[string]string{}
	title := ""
	var body strings.Builder
	flush := func() {
		if title != "" {
			sections[title] = strings.TrimSpace(body.String())
		}
	}
	for _, line := range strings.Split(string(source), "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			title = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			body.Reset()
			continue
		}
		if title != "" {
			body.WriteString(line)
			body.WriteByte('\n')
		}
	}
	flush()
	return sections
}
