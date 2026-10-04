//
// @file site.go
// @brief Building a documentation site.
//
// A site is a directory of static files: one HTML page per source, an index
// that lists the sections, a stylesheet, and a search index. Nothing in it
// needs a server, so it can be published to a file share, an object store or a
// plain web server, and it keeps working when none of them change.
//
// The pages are linked to each other by target, not by path: a cross-reference
// names a target, and the site builder resolves it to the page that declares
// it. A reference to a target nothing declares is reported rather than rendered
// as a dead link, because a broken cross-reference is a documentation bug the
// reader cannot fix and the author cannot see.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodoc

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Site model                                                                */
/* -------------------------------------------------------------------------- */

// SiteOptions is what a build is asked for.
type SiteOptions struct {
	// Title is the site title, shown in the header and the index.
	Title string
	// Out is the directory the site is written to.
	Out string
	// Project is the project name a page reports, which the build reads from
	// the root manifest.
	Project string
	// Root is the project directory the sources were read from, so a plugin
	// that generates pages from those sources can read them too. A build that
	// documents a tree it was not given a root for leaves it empty.
	Root string
	// Version is the project version a page reports.
	Version string
	// Home is the address of the site this documentation was published into,
	// and HomeTitle what it is called there. A standalone site has neither, and
	// a published one has both: a reader who arrived from the site that
	// publishes it needs a way back, and a generator cannot invent that address.
	Home      string
	HomeTitle string
}

// Site is a whole documentation set: the pages, grouped the way a sidebar
// lists them.
type Site struct {
	Title   string
	Project string
	// Root is the project directory this site documents, which a plugin that
	// generates pages from the sources is given along with the pages.
	Root    string
	Version string
	// Home is the address of the site this documentation was published into,
	// and HomeTitle what it is called there. A site built on its own has
	// neither; a published one has both, because a reader who followed a link
	// out of the site that publishes it needs a way back, and a generator
	// cannot invent that address.
	Home      string
	HomeTitle string
	// Sections are the sidebar groups, in order.
	Sections []SectionGroup
	// Pages are every page, in site order.
	Pages []*SitePage
	// Diagnostics are the problems found while building, in the order found.
	Diagnostics []Diagnostic
	// Requests are the cross-references that resolved to nothing.
	Requests []UnresolvedRef
	// plugins are the plugins this build runs, remembered so a hook pass does
	// not have to be handed them again.
	plugins []Plugin
}

// PluginResult is what the plugins answered one hook with, in the terms a site
// works in: pages they added, bodies they rewrote, and what they reported.
type PluginResult struct {
	// Pages are the pages the plugins added.
	Pages []*SitePage
	// Replacements are rewritten bodies, keyed by the target of the page they
	// belong to.
	Replacements map[string]string
	// Diagnostics is what the plugins reported.
	Diagnostics []Diagnostic
}

// SectionGroup is one sidebar group: a section of pages under a heading.
type SectionGroup struct {
	Name  string
	Pages []*SitePage
}

// SitePage is one page of the site.
type SitePage struct {
	// Target is the identifier a cross-reference names.
	Target string
	Title  string
	// Href is where the page lives, relative to the site root.
	Href string
	// Section is the sidebar group the page is listed under.
	Section string
	// Module is the module the page documents, when it documents one.
	Module string
	// Body is the rendered body of the page.
	Body string
	// Sections are the headings in the page, which is its table of contents.
	Sections []Section
	// Plain is the text of the page, for the search index.
	Plain string
	// Previous and Next walk the site in order, which is how a page offers the
	// reader a way onward.
	Previous *SitePage
	Next     *SitePage
	// Generated marks a page the generator wrote rather than one an author
	// wrote, such as an API reference page.
	Generated bool
	// ambiguous records, per target, the pages that answer to it, so a
	// reference that needs to tell them apart can be reported.
	ambiguous map[string][]string
}

// UnresolvedRef is a cross-reference that names a target no page declares.
type UnresolvedRef struct {
	Page  string
	Role  string
	Label string
	Line  int
}

func (u UnresolvedRef) String() string {
	where := u.Page
	if u.Line > 0 {
		where = fmt.Sprintf("%s:%d", u.Page, u.Line)
	}
	return fmt.Sprintf("%s: reference %q names no target", where, u.Label)
}

/* -------------------------------------------------------------------------- */
/*  Building                                                                  */
/* -------------------------------------------------------------------------- */

// BuildSite turns a set of modules and a generated reference into a site.
//
// Pages are laid out as <target>.html, so two modules that document a page of
// the same name do not overwrite one another, and every link inside the site is
// written as the target's own file.
func BuildSite(modules []ModuleDoc, api ApiSet, options SiteOptions) *Site {
	return buildSite(modules, api, options, nil)
}

// BuildSiteWithPlugins builds a site that hands itself to the named plugins on
// the way out. A caller that discovered plugins passes them here rather than
// after the build, so a hook pass sees the same set the caller chose.
func BuildSiteWithPlugins(modules []ModuleDoc, api ApiSet, options SiteOptions, found []Plugin) *Site {
	return buildSite(modules, api, options, found)
}

func buildSite(modules []ModuleDoc, api ApiSet, options SiteOptions, found []Plugin) *Site {
	site := &Site{
		plugins:   found,
		Title:     options.Title,
		Project:   options.Project,
		Root:      options.Root,
		Version:   options.Version,
		Home:      options.Home,
		HomeTitle: options.HomeTitle,
	}
	if site.Title == "" {
		site.Title = site.Project
	}
	if site.Title == "" {
		site.Title = "Documentation"
	}

	documents := []*parsedPage{}
	// The reference comes first, because it is what a reader arrives looking
	// for: what the language offers before what a project says about it.
	// What the reference could not read is reported whether or not it produced
	// a page: a scan that found nothing is exactly when a reader needs to be
	// told why.
	site.Diagnostics = append(site.Diagnostics, api.Diagnostics...)
	apiPages := api.documents()
	for index := range apiPages {
		document := apiPages[index]
		documents = append(documents, &parsedPage{
			document:   &document,
			section:    api.Section,
			module:     "",
			generated:  true,
			landing:    document.Target == "api",
			searchText: api.SearchText(document.Target),
		})
	}
	for _, module := range modules {
		for _, source := range module.Sources {
			document := readSource(source)
			// A page is addressed by where it lives in the tree that documents
			// it, not by its file name alone: two modules may each have an
			// index, and a reference to one of them has to be able to say which.
			document.Target = pageTarget(module.Name, module.Section, source, module.Root)
			landing := module.Index != "" && strings.HasSuffix(document.Path, filepath.FromSlash(module.Index))
			// A module describes itself in its manifest, and the page it leads
			// a section with is where that description belongs: a reader who
			// arrives at a component's page is owed a line saying what it is.
			if landing {
				document.applyModuleSummary(module)
			}
			site.Diagnostics = append(site.Diagnostics, document.Diagnostics...)
			page := &parsedPage{
				document: document,
				section:  module.Section,
				module:   module.Name,
				landing:  landing,
			}
			documents = append(documents, page)
		}
	}
	sortParsedPages(documents, modules)

	// Targets are resolved before anything is rendered, because a page is
	// written to the path its own target asks for.
	targets := map[string]*SitePage{}
	// Two pages answering to one target is ordinary: every component has an
	// index. It becomes a defect only when a cross-reference needs to tell them
	// apart, so the collision is recorded here and reported there.
	ambiguous := map[string][]string{}
	for _, page := range documents {
		href := pageHref(page)
		ambiguous[page.document.Target] = append(ambiguous[page.document.Target], page.document.Title)
		targets[page.document.Target] = &SitePage{
			Target:  page.document.Target,
			Title:   page.document.Title,
			Href:    href,
			Section: page.section,
			Module:  page.module,
		}
	}

	for _, page := range documents {
		site.Pages = append(site.Pages, &SitePage{
			Target:    page.document.Target,
			Title:     page.document.Title,
			Href:      pageHref(page),
			Section:   page.section,
			Module:    page.module,
			Body:      RenderPage(page.document).Body,
			Sections:  page.document.Sections,
			Plain:     indexText(page),
			Generated: page.generated,
		})
	}

	resolveReferences(site, documents, targets, ambiguous)
	linkNeighbours(site)
	site.groupSections(modules)
	return site
}

// parsedPage is a page before the site knows what it is called.
type parsedPage struct {
	document  *Document
	section   string
	module    string
	generated bool
	// searchText is text to index that the page body does not contain: a
	// reference page's declarations are drawn as markup, so its names are
	// carried here to stay searchable.
	searchText string
	// landing marks the page a reader arrives on within its module.
	landing bool
}

// readSource reads one doc-as-code file. A file that cannot be read is still
// reported as a diagnostic rather than skipped, so a build says what is wrong
// instead of quietly producing less documentation.
func readSource(path string) *Document {
	data, err := os.ReadFile(path)
	if err != nil {
		return Parse(path, "The source could not be read: "+err.Error()+"\n")
	}
	return Parse(path, string(data))
}

// stripDocsRoot is the directory a module documents itself in, which is left
// out of a target: a module's pages are addressed as the author wrote them
// (`index`, `deployment`) rather than with a directory every page repeats.
const stripDocsRoot = "docs"

// pageTarget is the name a cross-reference uses to reach a page.
//
// A page is named for the module it documents and its path inside that module,
// so `{ref}`rho-log/index“ reaches one component's page from another's. A
// repository's own pages are named for the directory they were written in,
// which is their section. Both are unique across a site by construction, which
// is what a site-wide name has to be.
func pageTarget(module, section, source, root string) string {
	relative, err := filepath.Rel(root, source)
	if err != nil {
		relative = filepath.Base(source)
	}
	relative = filepath.ToSlash(relative)
	relative = strings.TrimSuffix(relative, filepath.Ext(relative))
	if strip := stripDocsRoot + "/"; strings.HasPrefix(relative, strip) {
		relative = relative[len(strip):]
	}
	// A README is the page a module leads with, so it is named index: a module
	// documented from its package and a module documented from its own sources
	// then address their front page the same way.
	if strings.EqualFold(relative, "README") {
		relative = "index"
	}
	if module != "" {
		return module + "/" + relative
	}
	// A repository page carries the section it belongs to, so the front page of
	// a repository is not the same name as the index of the site.
	if section != "" && !strings.HasPrefix(relative, section+"/") {
		return section + "/" + relative
	}
	return relative
}

// pageHref is where a page is written, relative to the site root.
//
// The target is already unique across the site, so the address is the target:
// one rule, and no page can be written over another. The generated index takes
// `index.html`, which is why no page's target is a bare `index`.
func pageHref(page *parsedPage) string {
	return page.document.Target + ".html"
}

// sortParsedPages orders the pages the way a reader walks them: by section, by
// the order the module declared, then by the title, so a build is repeatable.
func sortParsedPages(pages []*parsedPage, modules []ModuleDoc) {
	order := map[string]int{}
	for index, module := range modules {
		if _, seen := order[module.Name]; !seen {
			order[module.Name] = index
		}
	}
	sort.SliceStable(pages, func(i, j int) bool {
		left, right := pages[i], pages[j]
		if left.section != right.section {
			return left.section < right.section
		}
		if left.module != right.module {
			return order[left.module] < order[right.module]
		}
		// A module lands on its index, so the index is read first and the
		// rest follow in the order a reader would look for them.
		if leftLanding, rightLanding := isLandingPage(left), isLandingPage(right); leftLanding != rightLanding {
			return leftLanding
		}
		return left.document.Title < right.document.Title
	})
}

// applyModuleSummary puts what a manifest says about a module on the page the
// module leads its section with: the title it declares becomes the summary line
// and the module's own description is shown. Both are declarations the manifest
// already carries, so a module that describes itself is described once, where it
// was written.
func (d *Document) applyModuleSummary(module ModuleDoc) {
	summary := module.Description
	if module.DocsTitle != "" {
		summary = module.DocsTitle
	}
	if summary == "" {
		return
	}
	// A page that already says it does not need the line repeated. A README
	// opens with what the module is, so the summary is for the page that does
	// not: a docs page written for a reader who arrived from elsewhere.
	if strings.EqualFold(summary, d.Title) || strings.Contains(strings.ToLower(plainText(d)), strings.ToLower(summary)) {
		return
	}
	d.Blocks = append([]*Node{{
		Kind:     BlockParagraph,
		Line:     1,
		Children: parseInline(summary),
	}}, d.Blocks...)
}

// isLandingPage reports whether a page is the one a reader arrives on: the
// index of a module, or the module's own declared index.
func isLandingPage(page *parsedPage) bool {
	if page.landing {
		return true
	}
	name := page.document.Path
	name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	return strings.EqualFold(name, "index") || strings.EqualFold(name, "readme")
}

/* -------------------------------------------------------------------------- */
/*  Cross-references                                                          */
/* -------------------------------------------------------------------------- */

// resolveReferences rewrites every cross-reference to the page that declares
// its target, and records the ones that resolve to nothing.
func resolveReferences(site *Site, documents []*parsedPage, targets map[string]*SitePage, ambiguous map[string][]string) {
	// A heading with an explicit target is addressable on its own, so a
	// reference can land on a section rather than on the top of a page.
	fragments := map[string]string{}
	labels := map[string]string{}
	for _, page := range documents {
		href := pageHref(page)
		for _, section := range page.document.Sections {
			key := page.document.Target + "#" + section.ID
			if _, seen := fragments[key]; !seen {
				fragments[key] = href + "#" + section.ID
			}
			if section.ID == "" || !section.Explicit {
				continue
			}
			if previous, taken := labels[section.ID]; taken {
				// Two pages can claim one label. A reference to it has to be
				// told which one it means, so the ambiguity is reported rather
				// than resolved arbitrarily.
				labels[section.ID] = ""
				site.Diagnostics = append(site.Diagnostics, Diagnostic{
					Path:    page.document.Path,
					Line:    section.Line,
					Message: fmt.Sprintf("label %q is also used by %s; a reference to it has to name the page", section.ID, previous),
				})
				continue
			}
			labels[section.ID] = href + "#" + section.ID
		}
	}

	byTarget := map[string]*SitePage{}
	for _, page := range site.Pages {
		byTarget[page.Target] = page
	}

	for index, page := range site.Pages {
		document := documents[index]
		resolved := resolveInBody(page.Body, byTarget, fragments, labels)
		page.Body = resolved
		page.ambiguous = ambiguous
		// A reference inside a page names a target; a reference that names
		// nothing is a defect in the page, not in the site.
		// A reference inside a page names a page target, a heading label, or a
		// heading of this very page. A reference that names none of them is a
		// defect in the page, not in the site.
		for _, node := range collectRoles(document.document) {
			if owners, ok := page.ambiguous[node.Href]; ok {
				// A target more than one page answers to resolves to the first,
				// which is arbitrary, so a reference that needs it is reported
				// rather than quietly pointed at whichever page was read first.
				if len(owners) > 1 {
					site.Requests = append(site.Requests, UnresolvedRef{
						Page:  document.document.Path,
						Role:  node.Role,
						Label: node.Href,
						Line:  node.Line,
					})
					site.Diagnostics = append(site.Diagnostics, Diagnostic{
						Path: document.document.Path,
						Line: node.Line,
						Message: fmt.Sprintf("target %q is declared by %d pages (%s); name the page a reference to it should reach",
							node.Href, len(owners), strings.Join(owners, ", ")),
					})
				}
				continue
			}
			if _, ok := labels[node.Href]; ok {
				continue
			}
			if _, ok := fragments[node.Href]; ok {
				continue
			}
			if _, ok := fragments[document.document.Target+"#"+node.Href]; ok {
				continue
			}
			site.Requests = append(site.Requests, UnresolvedRef{
				Page:  document.document.Path,
				Role:  node.Role,
				Label: node.Text,
				Line:  node.Line,
			})
		}
	}
}

// resolveInBody rewrites the placeholder links a renderer emits for roles into
// the real address of the target they name. A page is written at its own
// address, so a link to it is the target itself; a heading is an address within
// a page, so a link to one is the page plus a fragment.
//
// Only the markup a role produces is rewritten. A role is written as
// `<a class="xref" href="#name">`, and matching that whole form leaves an
// author's own `[text](#name)` link alone, which a page with a heading of that
// name would otherwise have silently redirected.
func resolveInBody(body string, targets map[string]*SitePage, fragments, labels map[string]string) string {
	out := body
	rewrite := func(href, resolved string) {
		out = strings.ReplaceAll(out,
			"class=\"xref\" href=\"#"+html.EscapeString(href)+"\"",
			"class=\"xref\" href=\""+html.EscapeString(resolved)+"\"")
	}
	for target, page := range targets {
		if page == nil || page.Href == "" {
			continue
		}
		rewrite(target, page.Href)
	}
	for key, href := range fragments {
		rewrite(key, href)
	}
	for label, href := range labels {
		// An ambiguous label resolves to nothing: it is reported, and a link
		// that goes nowhere would be worse than one that is visibly plain.
		if href == "" {
			continue
		}
		rewrite(label, href)
	}
	return out
}

// collectRoles gathers every cross-reference in a document, so the site can
// report the ones that resolve to nothing.
func collectRoles(document *Document) []*Node {
	found := []*Node{}
	var walk func(*Node)
	walk = func(current *Node) {
		if current == nil {
			return
		}
		if current.Kind == InlineRole {
			found = append(found, current)
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	for _, block := range document.Blocks {
		walk(block)
	}
	return found
}

/* -------------------------------------------------------------------------- */
/*  Layout                                                                    */
/* -------------------------------------------------------------------------- */

// linkNeighbours wires each page to the next and the previous one in site
// order, so a reader always has a way onward.
func linkNeighbours(site *Site) {
	for index, page := range site.Pages {
		if index > 0 {
			page.Previous = site.Pages[index-1]
		}
		if index+1 < len(site.Pages) {
			page.Next = site.Pages[index+1]
		}
	}
}

// groupSections fills the sidebar groups, in the order sections first appear.
func (site *Site) groupSections(modules []ModuleDoc) {
	order := []string{}
	groups := map[string]*SectionGroup{}
	for _, page := range site.Pages {
		group, seen := groups[page.Section]
		if !seen {
			group = &SectionGroup{Name: page.Section}
			groups[page.Section] = group
			order = append(order, page.Section)
		}
		group.Pages = append(group.Pages, page)
	}
	// A section a module declared but contributed no page to still belongs in
	// the sidebar: a reader looking for it should find it named, and empty,
	// rather than absent.
	for _, module := range modules {
		if len(module.Sources) == 0 {
			continue
		}
		if _, seen := groups[module.Section]; seen {
			continue
		}
		groups[module.Section] = &SectionGroup{Name: module.Section}
		order = append(order, module.Section)
	}
	for _, name := range order {
		site.Sections = append(site.Sections, *groups[name])
	}
}

/* -------------------------------------------------------------------------- */
/*  Plain text                                                                */
/* -------------------------------------------------------------------------- */

// indexText is what a page contributes to the search index: its own text, plus
// whatever the page carries that is drawn as markup rather than written. A
// reference page names its declarations in a table, so those names are added
// explicitly, which is what makes searching for a function find its module.
func indexText(page *parsedPage) string {
	text := plainText(page.document)
	if page.searchText == "" {
		return text
	}
	return text + " " + page.searchText
}

// plainText is the text of a page without markup, which is what a search index
// holds: it stores words, not the page that draws them.
func plainText(document *Document) string {
	parts := []string{document.Title}
	for _, section := range document.Sections {
		parts = append(parts, section.Title)
	}
	parts = append(parts, textOf(document.Blocks)...)
	return strings.Join(parts, " ")
}

func textOf(nodes []*Node) []string {
	parts := []string{}
	var walk func(*Node)
	walk = func(node *Node) {
		if node == nil {
			return
		}
		if node.Text != "" {
			parts = append(parts, node.Text)
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, node := range nodes {
		walk(node)
	}
	return parts
}

/* -------------------------------------------------------------------------- */
/*  Writing                                                                   */
/* -------------------------------------------------------------------------- */

// Write writes the site as a directory of static files.
func (site *Site) Write(out string) error {
	if site.Title != "" {
		if err := os.MkdirAll(out, 0o755); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(out, "static"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "static", "rho-doc.css"), []byte(Stylesheet), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "static", "rho-doc.js"), []byte(SearchScript), 0o644); err != nil {
		return err
	}
	for _, page := range site.Pages {
		path, err := pagePath(out, page.Href)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		document := site.renderPageHTML(page)
		if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(out, "index.html"), []byte(site.renderIndexHTML()), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "search-index.json"), []byte(site.searchIndex()), 0o644)
}

// pagePath is where a page is written, checked to be inside the output
// directory.
//
// A page's address comes from what a module declares: its section, its name,
// and the paths of its sources. A repository that names a section of
// `../../.ssh` would otherwise write wherever it liked, so an address that
// leaves the output directory is refused rather than followed. A documentation
// build must not be a way to write outside the directory it was given.
func pagePath(out, href string) (string, error) {
	base, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	path := filepath.Join(base, filepath.FromSlash(href))
	relative, err := filepath.Rel(base, path)
	if err != nil {
		return "", fmt.Errorf("page address %q is not inside %s", href, out)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("page address %q is outside %s", href, out)
	}
	return path, nil
}

// searchIndex is the JSON a page's search box reads. It is a document per
// page, so the index stays readable and a reader can see what is indexed.
func (site *Site) searchIndex() string {
	entries := []string{}
	for _, page := range site.Pages {
		entries = append(entries, fmt.Sprintf("  {\"title\": %s, \"href\": %s, \"section\": %s, \"text\": %s}",
			jsString(page.Title), jsString(page.Href), jsString(page.Section), jsString(page.Plain)))
	}
	return "{\n  \"site\": " + jsString(site.Title) + ",\n  \"pages\": [\n" +
		strings.Join(entries, ",\n") + "\n  ]\n}\n"
}

// jsString quotes a value as a JSON string. The output is written by hand
// rather than through a package because a documentation tool must not drag a
// dependency into every project it documents.
func jsString(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, "\\u%04x", r)
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
