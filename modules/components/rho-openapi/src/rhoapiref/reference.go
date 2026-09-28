//
// @file reference.go
// @brief Rendering an API description as documentation a person reads.
//
// A document is written for a tool: `openapi.json` is what a client generator
// reads, and it is not what a person reads when they want to know what an
// endpoint takes. So the same description is also rendered as a page — the
// operations grouped by where they live, the parameters in a table, the
// statuses beside them — and that page joins the site the rest of the
// documentation is in.
//
// One site rather than two is the point. An API reference that lived somewhere
// else would be a second place to look, a second search box, and a second set
// of links to keep working. Here it is a page, so the sidebar lists it beside
// the module reference and a cross-reference from prose reaches it the same way
// it reaches anything else.
//
// The rendering is deliberately plain: a heading, a table and a code block.
// What a reader needs is which endpoints exist, what they take, what they
// answer, and whether they need a token. Anything more is a page that looks
// better and says the same.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhoapiref

import (
	"sort"
	"strconv"
	"strings"

	"github.com/rho/rho-router/src/rhorouter"
)

// ReferenceOptions say what a rendered reference is called and where it sits.
type ReferenceOptions struct {
	// Target is the page's address, which is what a cross-reference names and
	// what the sidebar links to.
	Target string
	// Title is the page's heading and its sidebar entry.
	Title string
	// Section is the sidebar group the page is listed under, so an API
	// reference sits with the module reference rather than beside it.
	Section string
	// Summary is the sentence the page opens with.
	Summary string
	// Server is the address the service answers at, shown as the base a reader
	// calls against.
	Server string
	// Prefix is what every path is served under, for a service mounted below
	// its own root.
	Prefix string
}

// DefaultTarget is the address an API reference page is written to when a
// caller names none.
const DefaultTarget = "api"

// Render writes a reference page's HTML from a spec.
//
// The rendering reads the spec rather than a document, so a page and a document
// show the same thing by construction: both are written from one reading, and
// neither is derived from the other. It is a package of its own because its
// audience is a person rather than a parser, which is a different job with
// different rules: a document has to be exact, and a page has to be readable.
func Render(spec rhorouter.Spec, options ReferenceOptions) string {
	if options.Target == "" {
		options.Target = DefaultTarget
	}
	if options.Title == "" {
		options.Title = firstNonEmpty(spec.Title, spec.Module) + " HTTP API"
	}

	var b strings.Builder
	writeHeader(&b, spec, options)
	writeOverview(&b, spec, options)
	for _, tag := range tagsOf(spec) {
		writeTag(&b, spec, tag)
	}
	writeSchemes(&b, spec)
	return b.String()
}

// writeHeader opens the page with what the service is and how to reach it.
func writeHeader(b *strings.Builder, spec rhorouter.Spec, options ReferenceOptions) {
	b.WriteString("<h1>" + esc(options.Title) + "</h1>\n")
	if options.Summary != "" {
		b.WriteString("<p>" + esc(options.Summary) + "</p>\n")
	}
	if spec.Description != "" {
		b.WriteString("<p>" + esc(spec.Description) + "</p>\n")
	}
	if options.Server != "" {
		b.WriteString("<p>The service answers at <code>" + esc(options.Server) + "</code>.</p>\n")
	}
	if security := spec.Security.Describe(); security.Scheme != rhorouter.SecurityNone {
		b.WriteString("<p>" + esc(security.Description) + "</p>\n")
	}
}

// writeOverview lists every operation, so a reader can find the one they want
// before reading about it.
func writeOverview(b *strings.Builder, spec rhorouter.Spec, options ReferenceOptions) {
	if len(spec.Operations) == 0 {
		b.WriteString("<p>This service answers no routes.</p>\n")
		return
	}
	b.WriteString("<h2>Endpoints</h2>\n")
	b.WriteString("<table class=\"api-endpoints\">\n")
	b.WriteString("<thead><tr><th>Method</th><th>Path</th><th>Operation</th></tr></thead>\n<tbody>\n")
	for _, operation := range spec.Operations {
		b.WriteString("<tr>")
		b.WriteString("<td><span class=\"api-method api-method-" +
			strings.ToLower(operation.Method) + "\">" + esc(operation.Method) + "</span></td>")
		b.WriteString("<td><code>" + esc(options.Base(operation.Path)) + "</code></td>")
		b.WriteString("<td>" + esc(firstNonEmpty(operation.Summary, operation.ID)) + "</td>")
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody></table>\n")
}

// writeTag writes one group of operations.
func writeTag(b *strings.Builder, spec rhorouter.Spec, tag string) {
	b.WriteString("<h2>" + esc(firstNonEmpty(tag, "Operations")) + "</h2>\n")
	for _, operation := range spec.Operations {
		if operation.Tag != tag {
			continue
		}
		writeOperation(b, spec, operation)
	}
}

// writeOperation writes one operation in full: what it is, what it takes, what
// it answers.
func writeOperation(b *strings.Builder, spec rhorouter.Spec, operation rhorouter.Operation) {
	b.WriteString("<section class=\"api-operation\">\n")
	b.WriteString("<h3 id=\"" + esc(anchor(operation)) + "\">" +
		"<span class=\"api-method api-method-" + strings.ToLower(operation.Method) + "\">" +
		esc(operation.Method) + "</span> <code>" + esc(operation.Path) + "</code></h3>\n")

	if operation.Summary != "" {
		b.WriteString("<p>" + esc(operation.Summary) + "</p>\n")
	}
	if operation.Description != "" {
		b.WriteString(paragraphs(operation.Description))
	}
	if operation.Deprecated {
		b.WriteString("<p><strong>Deprecated.</strong> This operation is on its way out.</p>\n")
	}
	writeAuthNote(b, spec, operation)
	writeParameters(b, operation)
	writeBody(b, operation)
	writeResponses(b, operation)
	b.WriteString("</section>\n")
}

// writeAuthNote says what a caller has to prove, when they have to prove
// anything.
func writeAuthNote(b *strings.Builder, spec rhorouter.Spec, operation rhorouter.Operation) {
	security := spec.Security.Describe()
	if security.Scheme == rhorouter.SecurityNone {
		return
	}
	switch {
	case security.Required && !security.Public(operation.Path):
		b.WriteString("<p class=\"api-auth\">Requires " + esc(security.Name) + ".</p>\n")
	case operation.NeedsAuth:
		b.WriteString("<p class=\"api-auth\">Accepts " + esc(security.Name) +
			", which some deployments require.</p>\n")
	}
}

// writeParameters writes a table of what the operation reads, one row per
// value, with where it comes from and whether it is required.
func writeParameters(b *strings.Builder, operation rhorouter.Operation) {
	if len(operation.Parameters) == 0 {
		return
	}
	b.WriteString("<h4>Parameters</h4>\n")
	b.WriteString("<table class=\"api-parameters\">\n")
	b.WriteString("<thead><tr><th>Name</th><th>In</th><th>Type</th><th>Required</th></tr></thead>\n<tbody>\n")
	parameters := append([]rhorouter.Parameter{}, operation.Parameters...)
	sort.SliceStable(parameters, func(i, j int) bool {
		return sourceOrder(parameters[i].In) < sourceOrder(parameters[j].In)
	})
	for _, parameter := range parameters {
		b.WriteString("<tr>")
		b.WriteString("<td><code>" + esc(parameter.Name) + "</code></td>")
		b.WriteString("<td>" + esc(parameter.In) + "</td>")
		b.WriteString("<td>" + esc(parameter.Type) + "</td>")
		b.WriteString("<td>" + yesNo(parameter.Required || parameter.In == rhorouter.InPath) + "</td>")
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody></table>\n")
}

// writeBody writes what the operation accepts, when it accepts anything.
func writeBody(b *strings.Builder, operation rhorouter.Operation) {
	body := operation.RequestBody
	if body == nil {
		return
	}
	b.WriteString("<h4>Request body</h4>\n")
	if body.Raw {
		b.WriteString("<p>The request body is sent whole as <code>" +
			esc(body.ContentType) + "</code>.</p>\n")
		return
	}
	b.WriteString("<p>Sent as <code>" + esc(body.ContentType) + "</code>:</p>\n")
	b.WriteString("<table class=\"api-body\">\n")
	b.WriteString("<thead><tr><th>Field</th><th>Type</th><th>Required</th></tr></thead>\n<tbody>\n")
	for _, field := range body.Fields {
		b.WriteString("<tr>")
		b.WriteString("<td><code>" + esc(field.Name) + "</code></td>")
		b.WriteString("<td>" + esc(field.Type) + "</td>")
		b.WriteString("<td>" + yesNo(field.Required) + "</td>")
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody></table>\n")
}

// writeResponses writes the statuses an operation answers with.
func writeResponses(b *strings.Builder, operation rhorouter.Operation) {
	b.WriteString("<h4>Responses</h4>\n")
	if len(operation.Responses) == 0 {
		b.WriteString("<p>No status is named in the sources.</p>\n")
		return
	}
	b.WriteString("<table class=\"api-responses\">\n")
	b.WriteString("<thead><tr><th>Status</th><th>Body</th></tr></thead>\n<tbody>\n")
	responses := append([]rhorouter.Response{}, operation.Responses...)
	sort.SliceStable(responses, func(i, j int) bool {
		return responses[i].Status < responses[j].Status
	})
	for _, response := range responses {
		b.WriteString("<tr>")
		b.WriteString("<td><code>" + strconv.Itoa(response.Status) + "</code> " +
			esc(response.Description) + "</td>")
		if response.ContentType != "" {
			b.WriteString("<td><code>" + esc(response.ContentType) + "</code></td>")
		} else {
			b.WriteString("<td></td>")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody></table>\n")
}

// writeSchemes closes the page with a note on how a caller proves who it is,
// which is the one thing a reader cannot get from the operations alone.
func writeSchemes(b *strings.Builder, spec rhorouter.Spec) {
	security := spec.Security.Describe()
	if security.Scheme == rhorouter.SecurityNone {
		return
	}
	b.WriteString("<h2>Authentication</h2>\n")
	b.WriteString("<p>" + esc(security.Description) + "</p>\n")
	if len(security.PublicPaths) > 0 {
		b.WriteString("<p>Served without a credential:</p>\n<ul>\n")
		for _, path := range security.PublicPaths {
			b.WriteString("<li><code>" + esc(path) + "</code></li>\n")
		}
		b.WriteString("</ul>\n")
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                     */
/* -------------------------------------------------------------------------- */

// Base returns a path as a reader calls it, with a prefix applied when the
// service is mounted under one.
func (o ReferenceOptions) Base(path string) string { return o.Prefix + path }

// tagsOf returns the distinct tags, keeping the order they first appear so the
// page reads in the order the service declares.
func tagsOf(spec rhorouter.Spec) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, operation := range spec.Operations {
		tag := operation.Tag
		if seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

// sourceOrder orders the parameter table: what identifies the thing first, then
// how it is filtered, then what the caller declares about itself.
func sourceOrder(in string) int {
	switch in {
	case rhorouter.InPath:
		return 0
	case rhorouter.InQuery:
		return 1
	case rhorouter.InHeader:
		return 2
	default:
		return 3
	}
}

// anchor is the fragment an operation is linked by, which is its identifier: a
// handler is unique within a module, and a name a person wrote reads better in
// a link than one derived from a path.
func anchor(operation rhorouter.Operation) string {
	return firstNonEmpty(operation.ID, strings.ToLower(operation.Method)+"-"+operation.Path)
}

// yesNo writes a flag the way a reader reads one.
func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// paragraphs writes prose as paragraphs, splitting on the blank lines the
// source wrote.
func paragraphs(text string) string {
	var b strings.Builder
	for _, block := range strings.Split(strings.TrimSpace(text), "\n\n") {
		block = strings.TrimSpace(strings.ReplaceAll(block, "\n", " "))
		if block == "" {
			continue
		}
		b.WriteString("<p>" + esc(block) + "</p>\n")
	}
	return b.String()
}

// esc escapes text for HTML, so a description that mentions a tag is shown
// rather than obeyed.
func esc(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// firstNonEmpty returns the first value that says something, so a page falls
// back through the names a service may have been given.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
