//
// @file toc.go
// @brief A documentation plugin that adds a table of contents.
//
// A plugin extends a documentation build from the outside: the generator runs
// it as a process, writes a request to its standard input and reads a response
// from its standard output, both JSON. Nothing is shared but the protocol, so
// a plugin can be written in any language and versioned on its own.
//
// This one answers two hooks. The whole-site hook adds a contents page, because
// a table of contents is an answer about every page and no single page can
// produce it. The per-page hook adds a notice saying the page was generated,
// because a reader should be able to tell a generated page from a written one.
//
// Usage:
//
//	rho-doc-toc < documentation-request.json > response.json
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package toc

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"strings"
)

// ProtocolVersion is the version of the request and response shapes this
// plugin was written against. A generator that speaks another version is
// reported rather than answered with something it would misread.
const ProtocolVersion = 1

// Request is what a documentation build sends: the shared envelope, whose
// payload is the site this build is about.
//
// The envelope is the toolchain's and says which hook is being answered; the
// payload is the component's and says what it is being answered about. A plugin
// that read the envelope's top level would find every field of it empty.
type Request struct {
	Version   int             `json:"version"`
	Hook      string          `json:"hook"`
	Command   string          `json:"command"`
	Component string          `json:"component"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// Site is this component's part of a request: the site being built, and the
// page when one is being asked about.
type Site struct {
	Site    string `json:"site"`
	Project string `json:"project,omitempty"`
	Out     string `json:"out,omitempty"`
	Pages   []Page `json:"pages,omitempty"`
	Page    *Page  `json:"page,omitempty"`
}

// Page is one page as the build describes it.
type Page struct {
	Target  string `json:"target"`
	Title   string `json:"title"`
	Href    string `json:"href"`
	Section string `json:"section"`
	HTML    string `json:"html,omitempty"`
	Plain   string `json:"plain,omitempty"`
}

// Response is what a plugin answers.
type Response struct {
	Version      int               `json:"version,omitempty"`
	Pages        []Page            `json:"pages,omitempty"`
	Replacements map[string]string `json:"replacements,omitempty"`
	Diagnostics  []Diagnostic      `json:"diagnostics,omitempty"`
	Formats      []string          `json:"formats,omitempty"`
}

// Diagnostic is a problem a plugin reports about what it was given.
type Diagnostic struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Hook names the stages a build runs. A generator that does not know a hook
// name simply never calls it, so a plugin may answer hooks this version of the
// generator does not have yet.
const (
	HookIndex = "index"
	HookPage  = "page"
)

// Run answers one request, read from standard input and written to standard
// output.
func Run(input io.Reader, output io.Writer) error {
	payload, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("cannot read the request: %w", err)
	}
	var request Request
	if err := json.Unmarshal(payload, &request); err != nil {
		return fmt.Errorf("cannot read the request: %w", err)
	}
	site := Site{}
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &site); err != nil {
			return fmt.Errorf("cannot read the site: %w", err)
		}
	}
	if request.Version != ProtocolVersion {
		// A different version is answered with a diagnostic rather than with a
		// guess, so the build reports a plugin that does not fit.
		return json.NewEncoder(output).Encode(Response{
			Version: ProtocolVersion,
			Diagnostics: []Diagnostic{{
				Level:   "warning",
				Message: fmt.Sprintf("rho-doc-toc speaks protocol %d, the build speaks %d", ProtocolVersion, request.Version),
			}},
		})
	}

	response := Response{Version: ProtocolVersion}
	switch request.Hook {
	case HookIndex:
		response.Pages = append(response.Pages, contents(site))
	case HookPage:
		if site.Page != nil {
			response.Replacements = map[string]string{
				site.Page.Target: Notice(*site.Page, site.Site),
			}
		}
	}
	return json.NewEncoder(output).Encode(response)
}

// contents is the table of contents page, built from the pages the build has.
//
// Pages are grouped by section, in the order they arrive, which is the order
// the build says it reads them in. A section with no pages does not appear: a
// heading over nothing is noise.
func contents(request Site) Page {
	var body strings.Builder
	body.WriteString("<nav class=\"generated-toc\">\n")
	body.WriteString("<p class=\"toc-title\">All pages</p>\n<ul>\n")
	section := ""
	for _, page := range request.Pages {
		if page.Section != section {
			if section != "" {
				body.WriteString("</ul>\n")
			}
			section = page.Section
			if section != "" {
				body.WriteString("<li class=\"toc-section\">" + html.EscapeString(section) + "\n<ul>\n")
			}
		}
		body.WriteString("<li><a href=\"" + html.EscapeString(page.Href) + "\">" + html.EscapeString(page.Title) + "</a></li>\n")
	}
	if section != "" {
		body.WriteString("</ul>\n")
	}
	body.WriteString("</ul>\n</nav>\n")

	return Page{
		Target:  "contents",
		Title:   "Contents",
		Href:    "contents.html",
		Section: sectionOf(request),
		HTML:    body.String(),
		Plain:   "contents " + request.Site,
	}
}

// sectionOf is the section the contents page is listed under: the section most
// pages belong to, so it sits next to them.
func sectionOf(request Site) string {
	counts := map[string]int{}
	for _, page := range request.Pages {
		counts[page.Section]++
	}
	best, bestCount := "", 0
	for name, count := range counts {
		if count > bestCount {
			best, bestCount = name, count
		}
	}
	return best
}

// Notice is the banner a generated page carries, so a reader can tell what was
// written by a person from what a build derived from the code.
func Notice(page Page, site string) string {
	var body strings.Builder
	body.WriteString("<div class=\"generated-notice\">\n")
	body.WriteString("<p>This page was generated from the source")
	if site != "" {
		body.WriteString(" of " + html.EscapeString(site))
	}
	body.WriteString(". Edit the source, not this page.</p>\n</div>\n")
	return body.String() + page.HTML
}

// Main runs the plugin, which is what the module's command is.
func Main() int {
	if err := Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "rho-doc-toc:", err)
		return 1
	}
	return 0
}
