//
// @file layout.go
// @brief The page layout and the theme.
//
// A page is what a documentation reader lives in, so it is built the way such a
// page is built: a header with the site and a search box, a menu of every page
// on the left with the one being read marked, the page itself in the middle, and
// the headings of the page in a rail on the right. The menu is what makes a
// generated site navigable once it is more than a few pages, which it always is:
// the language reference is a tree, not a document.
//
// The stylesheet travels with the site rather than coming from a server, so a
// page renders the same way wherever it is opened, and a generated site can be
// published as files. The theme is one stylesheet with custom properties, so a
// site that wants its own colours overrides a handful of them rather than
// rewriting the page.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import (
	"html"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Page                                                                      */
/* -------------------------------------------------------------------------- */

// renderPageHTML writes one page of the site.
func (site *Site) renderPageHTML(page *SitePage) string {
	// A page that sits in a directory of the site has to name the site's assets
	// by its own depth, or it looks for the stylesheet where the stylesheet is
	// not. A page is addressed by its module and its path, so a nested page is
	// the normal case rather than the exception.
	root := rootPrefix(page.Href)
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\" />\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\" />\n")
	b.WriteString("<title>" + html.EscapeString(page.Title) + " — " + html.EscapeString(site.Title) + "</title>\n")
	b.WriteString("<link rel=\"stylesheet\" href=\"" + root + "static/rho-doc.css\" />\n")
	b.WriteString("</head>\n")
	b.WriteString("<body data-root=\"" + root + "\">\n")
	b.WriteString(site.renderHeader(root))
	b.WriteString("<div class=\"layout\">\n")
	b.WriteString(site.renderSidebar(page, root))
	b.WriteString("<main class=\"page\" id=\"content\">\n")
	b.WriteString(page.Body)
	b.WriteString(site.renderNeighbours(page, root))
	b.WriteString("</main>\n")
	b.WriteString(site.renderRail(page))
	b.WriteString("</div>\n")
	b.WriteString("<script src=\"" + root + "static/rho-doc.js\"></script>\n")
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// rootPrefix is the path from a page back to the root of its site.
//
// A page's links to other pages are relative to itself, which is what keeps a
// site movable, but its links to the site's own assets have to climb out of
// whatever directory the page sits in.
func rootPrefix(href string) string {
	depth := strings.Count(href, "/")
	if depth == 0 {
		return ""
	}
	return strings.Repeat("../", depth)
}

// renderHeader writes the site header: what the site is, what it is of, and the
// search box.
func (site *Site) renderHeader(root string) string {
	var b strings.Builder
	b.WriteString("<header class=\"topbar\">\n")
	// A published site is shown inside another one, so its header says where it
	// came from: a reader who followed a link out of a registry needs a way back
	// to the registry, and the generator is told that address rather than
	// guessing it.
	if site.Home != "" {
		label := site.HomeTitle
		if label == "" {
			label = "back"
		}
		b.WriteString("<a class=\"up\" href=\"" + html.EscapeString(site.Home) + "\">" +
			html.EscapeString(label) + "</a>\n")
	}
	b.WriteString("<a class=\"site-title\" href=\"" + root + "index.html\">" + html.EscapeString(site.Title) + "</a>\n")
	if site.Project != "" {
		b.WriteString("<span class=\"site-project\">" + html.EscapeString(site.Project))
		if site.Version != "" {
			b.WriteString(" <span class=\"version\">" + html.EscapeString(site.Version) + "</span>")
		}
		b.WriteString("</span>\n")
	}
	b.WriteString("<div class=\"search\">\n")
	b.WriteString("<input type=\"search\" id=\"rho-doc-search\" placeholder=\"Search\" autocomplete=\"off\" aria-label=\"Search the documentation\" />\n")
	b.WriteString("<div id=\"rho-doc-results\" role=\"listbox\"></div>\n")
	b.WriteString("</div>\n")
	b.WriteString("<button class=\"menu-toggle\" type=\"button\" onclick=\"rhoDocToggleMenu()\" aria-label=\"Menu\">☰</button>\n")
	b.WriteString("</header>\n")
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  The menu                                                                  */
/* -------------------------------------------------------------------------- */

// renderSidebar writes the menu: every page of the site, grouped by section,
// with the page being read marked as the one in view.
//
// The mark matters: a reader who has followed a reference into a page needs to
// see where they are in the tree, which is the thing a menu of links without a
// current item cannot tell them.
func (site *Site) renderSidebar(current *SitePage, root string) string {
	var b strings.Builder
	b.WriteString("<nav class=\"sidebar\" id=\"menu\" aria-label=\"Documentation pages\">\n")
	for _, group := range site.Sections {
		title := humanise(group.Name)
		b.WriteString("<div class=\"nav-group\">\n")
		b.WriteString("<h2 class=\"nav-title\">" + html.EscapeString(title) + "</h2>\n<ul>\n")
		for _, page := range group.Pages {
			class := "nav-link"
			if current != nil && page.Target == current.Target {
				class += " current"
			}
			badge := ""
			if page.Generated {
				badge = "<span class=\"nav-badge\">generated</span>"
			}
			b.WriteString("<li><a class=\"" + class + "\" href=\"" + root + html.EscapeString(page.Href) + "\">" +
				html.EscapeString(page.Title) + badge + "</a></li>\n")
		}
		b.WriteString("</ul>\n</div>\n")
	}
	b.WriteString("</nav>\n")
	return b.String()
}

// humanise turns a section or directory name into words: `api-reference` reads
// as `api reference`.
func humanise(name string) string {
	return strings.ReplaceAll(name, "-", " ")
}

/* -------------------------------------------------------------------------- */
/*  The rail                                                                  */
/* -------------------------------------------------------------------------- */

// renderRail writes the headings of the page being read, so a long page can be
// navigated rather than scrolled.
func (site *Site) renderRail(current *SitePage) string {
	if current == nil || len(current.Sections) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<nav class=\"rail\" aria-label=\"On this page\">\n")
	b.WriteString("<p class=\"rail-title\">On this page</p>\n<ul>\n")
	for _, section := range current.Sections {
		if section.Level < 2 {
			continue
		}
		class := "rail-link"
		if section.Level > 2 {
			class += " nested"
		}
		b.WriteString("<li><a class=\"" + class + "\" href=\"#" + html.EscapeString(section.ID) + "\">" +
			html.EscapeString(section.Title) + "</a></li>\n")
	}
	b.WriteString("</ul>\n</nav>\n")
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Neighbours                                                                */
/* -------------------------------------------------------------------------- */

// renderNeighbours links to the previous and the next page, so a reader working
// through a set is never at a dead end.
func (site *Site) renderNeighbours(page *SitePage, root string) string {
	if page.Previous == nil && page.Next == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("<nav class=\"neighbours\">\n")
	if page.Previous != nil {
		b.WriteString("<a class=\"previous\" href=\"" + root + html.EscapeString(page.Previous.Href) + "\">← " +
			html.EscapeString(page.Previous.Title) + "</a>\n")
	}
	if page.Next != nil {
		b.WriteString("<a class=\"next\" href=\"" + root + html.EscapeString(page.Next.Href) + "\">" +
			html.EscapeString(page.Next.Title) + " →</a>\n")
	}
	b.WriteString("</nav>\n")
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Index                                                                     */
/* -------------------------------------------------------------------------- */

// renderIndexHTML writes the front page of the site: what it covers, and every
// page under its section.
//
// A site of one module is a module's own documentation, so the front page leads
// with that module: what it is, and the pages it has.
func (site *Site) renderIndexHTML() string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\" />\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\" />\n")
	b.WriteString("<title>" + html.EscapeString(site.Title) + "</title>\n")
	b.WriteString("<link rel=\"stylesheet\" href=\"static/rho-doc.css\" />\n")
	b.WriteString("</head>\n<body>\n")
	b.WriteString(site.renderHeader(""))
	b.WriteString("<div class=\"layout\">\n")
	b.WriteString(site.renderSidebar(nil, ""))
	b.WriteString("<main class=\"page\" id=\"content\">\n")
	b.WriteString("<h1>" + html.EscapeString(site.Title) + "</h1>\n")
	if site.Project != "" {
		label := html.EscapeString(site.Project)
		if site.Version != "" {
			label += " " + html.EscapeString(site.Version)
		}
		b.WriteString("<p class=\"site-identity\">" + label + "</p>\n")
	}
	b.WriteString("<p class=\"page-count\">" + itoa(len(site.Pages)) + " pages.</p>\n")
	for _, group := range site.Sections {
		b.WriteString("<section class=\"index-section\">\n")
		b.WriteString("<h2>" + html.EscapeString(humanise(group.Name)) + "</h2>\n<ul class=\"index-list\">\n")
		for _, page := range group.Pages {
			b.WriteString("<li><a href=\"" + html.EscapeString(page.Href) + "\">" +
				html.EscapeString(page.Title) + "</a>")
			if page.Generated {
				b.WriteString(" <span class=\"nav-badge\">generated</span>")
			}
			if page.Module != "" {
				b.WriteString(" <span class=\"index-module\">" + html.EscapeString(page.Module) + "</span>")
			}
			b.WriteString("</li>\n")
		}
		b.WriteString("</ul>\n</section>\n")
	}
	b.WriteString("</main>\n")
	b.WriteString("</div>\n")
	b.WriteString("<script src=\"static/rho-doc.js\"></script>\n")
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  The theme                                                                 */
/* -------------------------------------------------------------------------- */

// Stylesheet is the theme. It is one file with custom properties at the top, so
// a site that wants its own colours sets a handful of them and changes nothing
// else.
const Stylesheet = `/* The documentation theme.
 *
 * Colours are declared once as custom properties, and everything else is
 * written against them, so a site re-themes itself by changing the first block
 * rather than by rewriting the rules below it.
 */

:root {
  color-scheme: light dark;

  --ink: #1a1d21;
  --ink-soft: #4a5158;
  --ink-faint: #6b7278;
  --surface: #ffffff;
  --surface-sunken: #f6f7f9;
  --surface-raised: #ffffff;
  --line: #e3e6ea;
  --line-strong: #cfd4da;
  --accent: #1f5f9c;
  --accent-soft: #1f5f9c14;
  --accent-line: #1f5f9c40;

  --note: #1f5f9c;
  --note-wash: #eef5fb;
  --tip: #0f766e;
  --tip-wash: #ecfdf7;
  --warning: #a45b00;
  --warning-wash: #fdf6e6;
  --danger: #b3261e;
  --danger-wash: #fbeceb;

  --code-bg: #f6f7f9;
  --code-line: #e3e6ea;
  --code-ink: #24292f;

  --measure: 46rem;
  --sidebar-width: 17rem;
  --rail-width: 15rem;

  --font: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  --mono: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace;
}

@media (prefers-color-scheme: dark) {
  :root {
    --ink: #e6e8ea;
    --ink-soft: #b6bcc2;
    --ink-faint: #8b9299;
    --surface: #14171a;
    --surface-sunken: #1a1e22;
    --surface-raised: #1f2429;
    --line: #2a3036;
    --line-strong: #3a4249;
    --accent: #79b8f3;
    --accent-soft: #79b8f31f;
    --accent-line: #79b8f34d;

    --note: #79b8f3;
    --note-wash: #16232e;
    --tip: #5eead4;
    --tip-wash: #10272444;
    --warning: #fbbf24;
    --warning-wash: #2b2411;
    --danger: #f87171;
    --danger-wash: #2c1616;

    --code-bg: #1a1e22;
    --code-line: #2a3036;
    --code-ink: #d6dae0;
  }
}

* { box-sizing: border-box; }

html { scroll-behavior: smooth; }

body {
  margin: 0;
  background: var(--surface);
  color: var(--ink);
  font-family: var(--font);
  font-size: 16.5px;
  line-height: 1.65;
  -webkit-font-smoothing: antialiased;
}

/* ---------------------------------------------------------------- header - */

.topbar {
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.7rem 1.5rem;
  background: var(--surface-raised);
  border-bottom: 1px solid var(--line);
}

.site-title {
  font-weight: 650;
  font-size: 1.05rem;
  color: var(--ink);
  text-decoration: none;
  white-space: nowrap;
}

.site-title:hover { color: var(--accent); }

.site-project {
  color: var(--ink-faint);
  font-size: 0.9rem;
  white-space: nowrap;
}

.version { color: var(--ink-faint); }

.search { margin-left: auto; position: relative; }

.search input {
  width: 15rem;
  max-width: 40vw;
  padding: 0.4rem 0.7rem;
  font: inherit;
  font-size: 0.9rem;
  color: var(--ink);
  background: var(--surface-sunken);
  border: 1px solid var(--line);
  border-radius: 0.5rem;
}

.search input:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

#rho-doc-results {
  display: none;
  position: absolute;
  right: 0;
  top: calc(100% + 0.4rem);
  width: 26rem;
  max-width: 80vw;
  max-height: 60vh;
  overflow-y: auto;
  background: var(--surface-raised);
  border: 1px solid var(--line);
  border-radius: 0.6rem;
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.14);
}

#rho-doc-results.visible { display: block; }

#rho-doc-results a {
  display: block;
  padding: 0.5rem 0.75rem;
  color: var(--ink);
  text-decoration: none;
  border-bottom: 1px solid var(--line);
}

#rho-doc-results a:hover, #rho-doc-results a:focus { background: var(--surface-sunken); }

#rho-doc-results .result-section { display: block; color: var(--ink-faint); font-size: 0.75rem; }
#rho-doc-results .empty { padding: 0.7rem 0.75rem; color: var(--ink-faint); }

.menu-toggle {
  display: none;
  font-size: 1.1rem;
  padding: 0.25rem 0.5rem;
  color: var(--ink);
  background: none;
  border: 1px solid var(--line);
  border-radius: 0.4rem;
  cursor: pointer;
}

/* ----------------------------------------------------------------- layout - */

.layout {
  display: grid;
  grid-template-columns: var(--sidebar-width) minmax(0, 1fr) var(--rail-width);
  align-items: start;
  gap: 2.5rem;
  max-width: 90rem;
  margin: 0 auto;
  padding: 0 1.5rem;
}

/* ----------------------------------------------------------------- menu - */

.sidebar {
  position: sticky;
  top: 4rem;
  max-height: calc(100vh - 5rem);
  overflow-y: auto;
  padding: 1.75rem 0 3rem;
  border-right: 1px solid var(--line);
  padding-right: 1rem;
}

.nav-group { margin-bottom: 1.5rem; }

.nav-title {
  margin: 0 0 0.5rem;
  font-size: 0.72rem;
  font-weight: 650;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-faint);
}

.sidebar ul { list-style: none; margin: 0; padding: 0; }

.nav-link {
  display: block;
  padding: 0.2rem 0.55rem;
  margin-left: -0.55rem;
  color: var(--ink-soft);
  text-decoration: none;
  border-left: 2px solid transparent;
  border-radius: 0 0.3rem 0.3rem 0;
}

.nav-link:hover { color: var(--accent); background: var(--accent-soft); }

/* The page being read, marked in the menu. A reader who has followed a
   reference needs to see where they are in the tree. */
.nav-link.current {
  color: var(--accent);
  font-weight: 600;
  background: var(--accent-soft);
  border-left-color: var(--accent);
}

.nav-badge {
  display: inline-block;
  margin-left: 0.4rem;
  padding: 0 0.35em;
  font-size: 0.62rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--ink-faint);
  border: 1px solid var(--line-strong);
  border-radius: 0.5rem;
  vertical-align: 0.08em;
}

/* ------------------------------------------------------------------ page - */

.page {
  min-width: 0;
  max-width: var(--measure);
  padding: 2rem 0 4rem;
}

.page h1 {
  margin: 0 0 1.25rem;
  font-size: 2.1rem;
  line-height: 1.2;
  letter-spacing: -0.015em;
}

.page h2 {
  margin: 2.75rem 0 0.9rem;
  padding-bottom: 0.3rem;
  font-size: 1.5rem;
  line-height: 1.3;
  letter-spacing: -0.01em;
  border-bottom: 1px solid var(--line);
}

.page h3 { margin: 2rem 0 0.7rem; font-size: 1.2rem; line-height: 1.35; }
.page h4 { margin: 1.6rem 0 0.6rem; font-size: 1.03rem; }
.page h5, .page h6 { margin: 1.4rem 0 0.5rem; font-size: 0.95rem; color: var(--ink-soft); }

/* A heading is the anchor a reference names, so it is marked on hover. */
.page h2[id]:hover::before,
.page h3[id]:hover::before,
.page h4[id]:hover::before {
  content: "#";
  margin-left: -1.1em;
  padding-right: 0.3em;
  color: var(--line-strong);
  font-weight: 400;
}

.page p { margin: 0.85rem 0; }

.page a { color: var(--accent); text-underline-offset: 0.15em; }
.page a:hover { text-decoration: none; border-bottom: 1px solid var(--accent-line); }

.page li { margin: 0.25rem 0; }
.page li > ul, .page li > ol { margin: 0.25rem 0; }

.page hr { margin: 2.5rem 0; border: none; border-top: 1px solid var(--line); }

.site-identity { margin: 0 0 0.3rem; color: var(--ink-faint); }
.page-count { margin: 0 0 2rem; color: var(--ink-faint); font-size: 0.9rem; }

/* ------------------------------------------------------------------ code - */

code {
  font-family: var(--mono);
  font-size: 0.86em;
  padding: 0.12em 0.35em;
  color: var(--code-ink);
  background: var(--code-bg);
  border: 1px solid var(--code-line);
  border-radius: 0.3rem;
  word-break: break-word;
}

pre {
  margin: 1rem 0;
  padding: 0.9rem 1.1rem;
  overflow-x: auto;
  background: var(--code-bg);
  border: 1px solid var(--code-line);
  border-radius: 0.6rem;
  line-height: 1.55;
}

pre code {
  padding: 0;
  font-size: 0.855rem;
  background: none;
  border: none;
  white-space: pre;
}

/* ----------------------------------------------------------------- notes - */

.admonition {
  margin: 1.25rem 0;
  padding: 0.85rem 1.1rem 0.85rem 1.1rem;
  background: var(--note-wash);
  border: 1px solid var(--accent-line);
  border-left: 4px solid var(--note);
  border-radius: 0.5rem;
}

.admonition-title {
  margin: 0 0 0.35rem;
  font-weight: 650;
  font-size: 0.8rem;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: var(--note);
}

.admonition > :last-child { margin-bottom: 0; }

.admonition.tip, .admonition.hint { background: var(--tip-wash); border-color: #0f766e40; border-left-color: var(--tip); }
.admonition.tip .admonition-title, .admonition.hint .admonition-title { color: var(--tip); }
.admonition.warning, .admonition.caution, .admonition.important, .admonition.attention {
  background: var(--warning-wash);
  border-color: #a45b0040;
  border-left-color: var(--warning);
}
.admonition.warning .admonition-title, .admonition.caution .admonition-title,
.admonition.important .admonition-title, .admonition.attention .admonition-title { color: var(--warning); }
.admonition.danger, .admonition.error { background: var(--danger-wash); border-color: #b3261e40; border-left-color: var(--danger); }
.admonition.danger .admonition-title, .admonition.error .admonition-title { color: var(--danger); }

blockquote {
  margin: 1.1rem 0;
  padding: 0.2rem 0 0.2rem 1.1rem;
  color: var(--ink-soft);
  border-left: 3px solid var(--line-strong);
}

blockquote > :last-child { margin-bottom: 0; }

/* ----------------------------------------------------------------- table - */

table {
  width: 100%;
  margin: 1.25rem 0;
  border-collapse: collapse;
  font-size: 0.94rem;
}

th, td {
  padding: 0.5rem 0.7rem;
  text-align: left;
  vertical-align: top;
  border-bottom: 1px solid var(--line);
}

th {
  font-weight: 650;
  font-size: 0.8rem;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--ink-faint);
  border-bottom: 1px solid var(--line-strong);
}

tbody tr:hover { background: var(--surface-sunken); }

table code { font-size: 0.85em; }

/* ------------------------------------------------------------------ rail - */

.rail {
  position: sticky;
  top: 4rem;
  max-height: calc(100vh - 5rem);
  overflow-y: auto;
  padding: 2.25rem 0 3rem;
  font-size: 0.85rem;
}

.rail-title {
  margin: 0 0 0.5rem;
  font-size: 0.72rem;
  font-weight: 650;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--ink-faint);
}

.rail ul { list-style: none; margin: 0; padding: 0; border-left: 1px solid var(--line); }

.rail-link {
  display: block;
  padding: 0.15rem 0 0.15rem 0.75rem;
  color: var(--ink-soft);
  text-decoration: none;
  border-left: 2px solid transparent;
  margin-left: -1px;
}

.rail-link:hover { color: var(--accent); border-left-color: var(--accent); }
.rail-link.nested { padding-left: 1.5rem; font-size: 0.95em; }

/* ----------------------------------------------------------- neighbours - */

.neighbours {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  margin-top: 3rem;
  padding-top: 1.25rem;
  border-top: 1px solid var(--line);
  font-size: 0.94rem;
}

.neighbours a { color: var(--accent); text-decoration: none; }
.neighbours a:hover { text-decoration: underline; }
.neighbours .next { margin-left: auto; text-align: right; }

/* ----------------------------------------------------------------- index - */

.index-section { margin: 2rem 0; }
.index-section h2 { font-size: 1.2rem; border-bottom: none; margin-bottom: 0.5rem; }

.index-list { list-style: none; margin: 0; padding: 0; }
.index-list li { padding: 0.25rem 0; border-bottom: 1px solid var(--line); }
.index-list a { text-decoration: none; }
.index-list a:hover { text-decoration: underline; }
.index-module { color: var(--ink-faint); font-size: 0.82rem; margin-left: 0.4rem; }

/* ------------------------------------------------------------ responsive - */

@media (max-width: 1180px) {
  .layout { grid-template-columns: var(--sidebar-width) minmax(0, 1fr); }
  .rail { display: none; }
}

@media (max-width: 820px) {
  .layout { grid-template-columns: minmax(0, 1fr); gap: 0; }
  .menu-toggle { display: inline-block; order: 3; }
  .search { margin-left: 0; flex: 1; }
  .search input { width: 100%; max-width: none; }
  .sidebar {
    position: static;
    max-height: none;
    padding: 1rem 0;
    border-right: none;
    border-bottom: 1px solid var(--line);
  }
  .sidebar.hidden { display: none; }
  .page { padding-top: 1.25rem; }
  .page h1 { font-size: 1.7rem; }
}

@media print {
  .topbar, .sidebar, .rail, .neighbours, .search, .menu-toggle { display: none; }
  .layout { display: block; }
  .page { max-width: none; padding: 0; }
  pre, .admonition, table { break-inside: avoid; }
  a { color: inherit; text-decoration: none; }
}
`

// SearchScript is the search the header offers. It reads the index the build
// wrote, so search works on a static site with no server behind it.
const SearchScript = `// Search over the index the documentation build wrote.
//
// The index is a file beside the pages, so this works from a web server and from
// a file:// URL: nothing is asked of anything but the filesystem the site is
// already on.
(function () {
  var input = document.getElementById("rho-doc-search");
  if (!input) { return; }
  var panel = document.getElementById("rho-doc-results");
  if (!panel) { return; }
  var index = null;
  var loading = null;
  // The index lives at the root of the site, and a page may sit in a directory
  // of it, so the path is read from the page rather than assumed.
  var root = (document.body && document.body.dataset.root) || "";

  function load() {
    if (index) { return Promise.resolve(index); }
    if (loading) { return loading; }
    loading = fetch(root + "search-index.json")
      .then(function (response) { return response.json(); })
      .then(function (data) { index = data.pages || []; return index; })
      .catch(function () { index = []; return index; });
    return loading;
  }

  function score(page, needle) {
    var title = page.title.toLowerCase();
    if (title === needle) { return 3; }
    if (title.indexOf(needle) === 0) { return 2; }
    if (title.indexOf(needle) >= 0) { return 1; }
    return 0;
  }

  function search(term) {
    var needle = term.toLowerCase();
    return index
      .map(function (page) { return { page: page, rank: score(page, needle) }; })
      .filter(function (hit) {
        if (hit.rank > 0) { return true; }
        return (hit.page.text || "").toLowerCase().indexOf(needle) >= 0;
      })
      .sort(function (left, right) { return right.rank - left.rank; })
      .slice(0, 20)
      .map(function (hit) { return hit.page; });
  }

  function show(term) {
    if (!term) { panel.className = ""; panel.innerHTML = ""; return; }
    load().then(function () {
      panel.innerHTML = "";
      var hits = search(term);
      if (!hits.length) {
        panel.innerHTML = "<p class=\\"empty\\">No page mentions that.</p>";
      }
      hits.forEach(function (page) {
        var link = document.createElement("a");
        link.href = page.href;
        link.appendChild(document.createTextNode(page.title));
        var where = document.createElement("span");
        where.className = "result-section";
        where.textContent = page.section || "";
        link.appendChild(where);
        panel.appendChild(link);
      });
      panel.className = "visible";
    });
  }

  input.addEventListener("input", function () { show(input.value.trim()); });
  input.addEventListener("focus", function () { if (input.value.trim()) { show(input.value.trim()); } });
  document.addEventListener("click", function (event) {
    if (!panel.contains(event.target) && event.target !== input) { panel.className = ""; }
  });
  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape") { panel.className = ""; input.blur(); }
  });

  // The menu is a column on a wide screen and a disclosure on a narrow one, so
  // the toggle only does something where the menu is hidden.
  window.rhoDocToggleMenu = function () {
    var menu = document.getElementById("menu");
    if (menu) { menu.classList.toggle("hidden"); }
  };
})();
`
