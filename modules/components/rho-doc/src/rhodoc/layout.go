//
// @file layout.go
// @brief The page layout and the stylesheet.
//
// A page is a header, a sidebar of the sections, the page itself, and links to
// the neighbouring pages. The layout is a template rather than string
// concatenation so that a theme can replace it: a project that wants its own
// look supplies a theme directory, and nothing in the generator changes.
//
// The stylesheet travels with the site. A page that borrows a stylesheet from a
// server stops being readable the moment the server is gone, and documentation
// that outlives its deployment is the point of writing it down.
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
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\" />\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\" />\n")
	b.WriteString("<title>" + html.EscapeString(page.Title) + " — " + html.EscapeString(site.Title) + "</title>\n")
	b.WriteString("<link rel=\"stylesheet\" href=\"static/rho-doc.css\" />\n")
	b.WriteString("</head>\n<body>\n")
	b.WriteString("<header class=\"site-header\">\n")
	b.WriteString("<a class=\"site-title\" href=\"index.html\">" + html.EscapeString(site.Title) + "</a>\n")
	if site.Project != "" {
		label := html.EscapeString(site.Project)
		if site.Version != "" {
			label += " <span class=\"version\">" + html.EscapeString(site.Version) + "</span>"
		}
		b.WriteString("<span class=\"site-project\">" + label + "</span>\n")
	}
	b.WriteString("<form class=\"search\" onsubmit=\"return rhoDocSearch(event)\">\n")
	b.WriteString("<input type=\"search\" id=\"rho-doc-search\" placeholder=\"Search\" autocomplete=\"off\" />\n")
	b.WriteString("</form>\n")
	b.WriteString("</header>\n")
	b.WriteString("<div class=\"site-body\">\n")
	b.WriteString(site.renderSidebar(page))
	b.WriteString("<main class=\"page\">\n")
	b.WriteString("<p class=\"breadcrumb\">" + site.renderBreadcrumb(page) + "</p>\n")
	b.WriteString(page.Body)
	if len(page.Sections) > 1 {
		b.WriteString(renderTableOfContents(page.Sections))
	}
	b.WriteString(site.renderNeighbours(page))
	b.WriteString("</main>\n")
	b.WriteString("</div>\n")
	b.WriteString("<script src=\"static/rho-doc.js\"></script>\n")
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// renderSidebar writes the section list, marking the page being read.
func (site *Site) renderSidebar(current *SitePage) string {
	var b strings.Builder
	b.WriteString("<nav class=\"sidebar\">\n")
	for _, group := range site.Sections {
		title := strings.ReplaceAll(group.Name, "-", " ")
		b.WriteString("<div class=\"sidebar-group\">\n")
		b.WriteString("<h2 class=\"sidebar-title\">" + html.EscapeString(title) + "</h2>\n<ul>\n")
		for _, page := range group.Pages {
			class := "sidebar-link"
			if current != nil && page.Target == current.Target {
				class += " current"
			}
			b.WriteString("<li><a class=\"" + class + "\" href=\"" + html.EscapeString(page.Href) + "\">" +
				html.EscapeString(page.Title) + "</a></li>\n")
		}
		b.WriteString("</ul>\n</div>\n")
	}
	b.WriteString("</nav>\n")
	return b.String()
}

// renderBreadcrumb writes the trail leading to a page.
func (site *Site) renderBreadcrumb(page *SitePage) string {
	if page.Section == "" {
		return ""
	}
	parts := []string{html.EscapeString(strings.ReplaceAll(page.Section, "-", " "))}
	if page.Module != "" {
		parts = append(parts, html.EscapeString(page.Module))
	}
	parts = append(parts, html.EscapeString(page.Title))
	return strings.Join(parts, " › ")
}

// renderTableOfContents writes the headings of a page as a list of links.
func renderTableOfContents(sections []Section) string {
	var b strings.Builder
	b.WriteString("<nav class=\"toc\">\n<p class=\"toc-title\">On this page</p>\n<ul>\n")
	for _, section := range sections {
		if section.Level < 2 {
			continue
		}
		class := ""
		if section.Level > 2 {
			class = " class=\"toc-nested\""
		}
		b.WriteString("<li" + class + "><a href=\"#" + html.EscapeString(section.ID) + "\">" +
			html.EscapeString(section.Title) + "</a></li>\n")
	}
	b.WriteString("</ul>\n</nav>\n")
	return b.String()
}

// renderNeighbours writes the links to the previous and the next page, so a
// reader working through a set is never at a dead end.
func (site *Site) renderNeighbours(page *SitePage) string {
	var b strings.Builder
	b.WriteString("<nav class=\"neighbours\">\n")
	if page.Previous != nil {
		b.WriteString("<a class=\"previous\" href=\"" + html.EscapeString(page.Previous.Href) + "\">← " +
			html.EscapeString(page.Previous.Title) + "</a>\n")
	}
	if page.Next != nil {
		b.WriteString("<a class=\"next\" href=\"" + html.EscapeString(page.Next.Href) + "\">" +
			html.EscapeString(page.Next.Title) + " →</a>\n")
	}
	b.WriteString("</nav>\n")
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Index                                                                     */
/* -------------------------------------------------------------------------- */

// renderIndexHTML writes the front page: what the site covers, and every
// section with its pages.
func (site *Site) renderIndexHTML() string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\" />\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\" />\n")
	b.WriteString("<title>" + html.EscapeString(site.Title) + "</title>\n")
	b.WriteString("<link rel=\"stylesheet\" href=\"static/rho-doc.css\" />\n")
	b.WriteString("</head>\n<body>\n")
	b.WriteString("<header class=\"site-header\">\n")
	b.WriteString("<span class=\"site-title\">" + html.EscapeString(site.Title) + "</span>\n")
	b.WriteString("</header>\n")
	b.WriteString("<div class=\"site-body\">\n")
	b.WriteString(site.renderSidebar(nil))
	b.WriteString("<main class=\"page\">\n")
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
		b.WriteString("<h2>" + html.EscapeString(strings.ReplaceAll(group.Name, "-", " ")) + "</h2>\n<ul>\n")
		for _, page := range group.Pages {
			b.WriteString("<li><a href=\"" + html.EscapeString(page.Href) + "\">" +
				html.EscapeString(page.Title) + "</a>")
			if page.Generated {
				b.WriteString(" <span class=\"badge\">generated</span>")
			}
			b.WriteString("</li>\n")
		}
		b.WriteString("</ul>\n</section>\n")
	}
	b.WriteString("</main>\n</div>\n")
	b.WriteString("<script src=\"static/rho-doc.js\"></script>\n")
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Assets                                                                    */
/* -------------------------------------------------------------------------- */

// Stylesheet is the stylesheet every page uses. It is written with the site
// rather than fetched, so a page renders the same way wherever it is served
// from, and it works from a file:// URL.
const Stylesheet = `:root {
  --ink: #1c1f24;
  --muted: #5b6472;
  --line: #dfe3e8;
  --accent: #1f5f9c;
  --surface: #f7f8fa;
  --note: #eef5fb;
  --warning: #fdf6e6;
  --danger: #fbeceb;
  --mono: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
}

* { box-sizing: border-box; }

body {
  margin: 0;
  color: var(--ink);
  background: #fff;
  font: 16px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
}

.site-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 1.5rem;
  border-bottom: 1px solid var(--line);
  background: var(--surface);
  position: sticky;
  top: 0;
  z-index: 10;
}

.site-title { font-weight: 600; text-decoration: none; color: var(--ink); }
.site-project { color: var(--muted); font-size: 0.9rem; }
.site-project .version { color: var(--muted); }
.search { margin-left: auto; }
.search input {
  padding: 0.35rem 0.6rem;
  border: 1px solid var(--line);
  border-radius: 4px;
  font: inherit;
  font-size: 0.9rem;
  min-width: 14rem;
}

.site-body { display: flex; align-items: flex-start; gap: 2rem; }

.sidebar {
  flex: 0 0 17rem;
  padding: 1.5rem 1rem 3rem 1.5rem;
  border-right: 1px solid var(--line);
  max-height: calc(100vh - 3.2rem);
  overflow-y: auto;
  position: sticky;
  top: 3.2rem;
}

.sidebar-group { margin-bottom: 1.25rem; }
.sidebar-title {
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--muted);
  margin: 0 0 0.4rem;
}
.sidebar ul { list-style: none; margin: 0; padding: 0; }
.sidebar-link { display: block; padding: 0.15rem 0.5rem; color: var(--ink); text-decoration: none; border-radius: 4px; }
.sidebar-link:hover { background: var(--surface); }
.sidebar-link.current { background: var(--note); color: var(--accent); font-weight: 600; }

.page { flex: 1 1 auto; padding: 1.5rem 2rem 4rem; max-width: 52rem; min-width: 0; }
.breadcrumb { color: var(--muted); font-size: 0.85rem; margin-top: 0; }
.page h1 { font-size: 1.9rem; margin: 0.2rem 0 1rem; }
.page h2 { font-size: 1.4rem; margin: 2rem 0 0.6rem; padding-bottom: 0.2rem; border-bottom: 1px solid var(--line); }
.page h3 { font-size: 1.15rem; margin: 1.5rem 0 0.5rem; }
.page h4, .page h5, .page h6 { font-size: 1rem; margin: 1.2rem 0 0.4rem; }
.page a { color: var(--accent); }
.page p { margin: 0.7rem 0; }
.page li { margin: 0.2rem 0; }

code {
  font-family: var(--mono);
  font-size: 0.9em;
  background: var(--surface);
  padding: 0.1em 0.3em;
  border-radius: 3px;
}
pre {
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 0.8rem 1rem;
  overflow-x: auto;
}
pre code { background: none; padding: 0; }

blockquote {
  margin: 0.8rem 0;
  padding: 0.1rem 1rem;
  border-left: 3px solid var(--line);
  color: var(--muted);
}

.admonition {
  margin: 1rem 0;
  padding: 0.6rem 1rem;
  border-left: 4px solid var(--accent);
  background: var(--note);
  border-radius: 0 6px 6px 0;
}
.admonition-title { margin: 0 0 0.3rem; font-weight: 600; text-transform: capitalize; }
.admonition.warning, .admonition.caution { background: var(--warning); border-left-color: #c98a00; }
.admonition.danger, .admonition.error { background: var(--danger); border-left-color: #b3261e; }

.directive {
  margin: 1rem 0;
  padding: 0.6rem 1rem;
  border: 1px dashed var(--line);
  border-radius: 6px;
}
.directive-arguments { margin: 0 0 0.4rem; }

table { border-collapse: collapse; width: 100%; margin: 1rem 0; }
th, td { border: 1px solid var(--line); padding: 0.4rem 0.6rem; text-align: left; vertical-align: top; }
th { background: var(--surface); }

.toc {
  margin: 2.5rem 0 1rem;
  padding: 0.8rem 1rem;
  background: var(--surface);
  border-radius: 6px;
}
.toc-title { margin: 0 0 0.4rem; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); }
.toc ul { list-style: none; margin: 0; padding: 0; }
.toc-nested { padding-left: 1.2rem; }

.neighbours { display: flex; justify-content: space-between; margin-top: 2.5rem; padding-top: 1rem; border-top: 1px solid var(--line); }
.neighbours a { text-decoration: none; }

.index-section { margin: 1.5rem 0; }
.badge {
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--muted);
  border: 1px solid var(--line);
  border-radius: 3px;
  padding: 0 0.3em;
}
.site-identity { color: var(--muted); }
.page-count { color: var(--muted); font-size: 0.9rem; }

#rho-doc-results {
  position: absolute;
  right: 1.5rem;
  top: 3rem;
  width: 22rem;
  max-height: 60vh;
  overflow-y: auto;
  background: #fff;
  border: 1px solid var(--line);
  border-radius: 6px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08);
  display: none;
}
#rho-doc-results.visible { display: block; }
#rho-doc-results a { display: block; padding: 0.4rem 0.7rem; text-decoration: none; color: var(--ink); border-bottom: 1px solid var(--line); }
#rho-doc-results a:hover { background: var(--surface); }
#rho-doc-results .result-section { color: var(--muted); font-size: 0.75rem; }
#rho-doc-results .empty { padding: 0.6rem 0.7rem; color: var(--muted); }

@media (max-width: 900px) {
  .site-body { display: block; }
  .sidebar { position: static; max-height: none; border-right: none; border-bottom: 1px solid var(--line); }
  .page { max-width: none; }
}
`

// SearchScript is the search the header offers. It reads the index the build
// wrote, so search works on a static site with no server behind it.
const SearchScript = `// Search over the index the documentation build wrote. The index is a file
// next to the pages, so this works from a web server and from a file:// URL.
(function () {
  var input = document.getElementById("rho-doc-search");
  if (!input) { return; }
  var panel = document.createElement("div");
  panel.id = "rho-doc-results";
  document.body.appendChild(panel);
  var index = null;

  function load() {
    if (index) { return Promise.resolve(index); }
    return fetch("search-index.json").then(function (response) {
      return response.json();
    }).then(function (data) {
      index = data.pages || [];
      return index;
    }).catch(function () {
      index = [];
      return index;
    });
  }

  function search(term) {
    var needle = term.toLowerCase();
    return index.filter(function (page) {
      return page.title.toLowerCase().indexOf(needle) >= 0 ||
        (page.text || "").toLowerCase().indexOf(needle) >= 0;
    }).slice(0, 20);
  }

  function show(term) {
    load().then(function () {
      panel.innerHTML = "";
      if (!term) { panel.className = ""; return; }
      var hits = search(term);
      if (!hits.length) {
        panel.innerHTML = "<p class=\\"empty\\">No page mentions that.</p>";
      }
      hits.forEach(function (page) {
        var link = document.createElement("a");
        link.href = page.href;
        link.innerHTML = "";
        link.appendChild(document.createTextNode(page.title));
        var section = document.createElement("span");
        section.className = "result-section";
        section.textContent = "  " + (page.section || "");
        link.appendChild(section);
        panel.appendChild(link);
      });
      panel.className = hits.length ? "visible" : "visible";
    });
  }

  input.addEventListener("input", function () { show(input.value.trim()); });
  input.addEventListener("focus", function () { if (input.value.trim()) { show(input.value.trim()); } });
  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape") { panel.className = ""; input.blur(); }
  });
})();
`
