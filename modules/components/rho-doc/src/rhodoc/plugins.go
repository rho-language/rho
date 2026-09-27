//
// @file plugins.go
// @brief Running the plugins that extend a site build.
//
// A build is composed in two passes. The first builds the pages from the
// sources; the second hands the result to the plugins, which may add pages,
// rewrite a body, or report a problem with the documentation they were given.
// Only then is the site written.
//
// The two passes are why a plugin is a plugin: a hook sees the whole site, not
// just one page, so an extension can do something a single page cannot, such as
// building a table of contents over the finished set.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import "sort"

/* -------------------------------------------------------------------------- */
/*  Describing the site                                                       */
/* -------------------------------------------------------------------------- */

// Describe renders the site as a plugin request: what it is called, where it is
// written, and what pages it has.
func (site *Site) Describe(out string) PluginRequest {
	request := PluginRequest{
		Version: ProtocolVersion,
		Site:    site.Title,
		Project: site.Project,
		Out:     out,
	}
	for _, page := range site.Pages {
		request.Pages = append(request.Pages, PluginPage{
			Target:  page.Target,
			Title:   page.Title,
			Href:    page.Href,
			Section: page.Section,
		})
	}
	return request
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                   */
/* -------------------------------------------------------------------------- */

// RunPlugins runs the plugins over a built site and folds what they returned in.
//
// A plugin failure is a diagnostic on the build, not a failure of the build:
// the documentation of everything else is still worth writing.
func (site *Site) RunPlugins(plugins []Plugin, out string) {
	if len(plugins) == 0 {
		return
	}
	request := site.Describe(out)

	// The whole-site pass comes first, so a per-page pass sees the pages a
	// whole-site plugin added.
	whole := RunHooks(plugins, HookIndex, request)
	site.merge(whole)

	// The per-page pass runs once for every page, with that page in the
	// request. A plugin that renders a page itself answers with a replacement
	// body rather than adding a second page.
	//
	// The pages are copied before the loop: merging a result reorders the
	// slice, and a loop over a slice that is reordered underneath it visits a
	// page twice and skips another.
	snapshot := append([]*SitePage{}, site.Pages...)
	for _, page := range snapshot {
		one := site.Describe(out)
		one.Page = &PluginPage{
			Target:  page.Target,
			Title:   page.Title,
			Href:    page.Href,
			Section: page.Section,
			HTML:    page.Body,
			Plain:   page.Plain,
		}
		// The whole set travels with every request, so a page hook can also see
		// the site the page belongs to.
		result := RunHooks(plugins, HookPage, one)
		site.merge(result)
	}
}

// HookIndex is the whole-site hook: a plugin that builds something over the
// finished set of pages.
const HookIndex = "index"

// HookPage is the per-page hook: a plugin that renders or rewrites one page.
const HookPage = "page"

// ComponentName is this component's own name, which is what a plugin has to say
// in order to be loaded by it. A component knows its own name; that is the one
// thing it does not have to be told.
const ComponentName = "rho-doc"

// OfferedHooks are the hooks a run of this generator goes through, and so the
// hooks a plugin may answer. They are declared in the component's manifest as
// well, because that is what a plugin is validated against; the test beside
// this file checks the two agree, so the list is written once in the manifest
// and read here.
var OfferedHooks = []string{HookIndex, HookPage}

// offers reports whether this generator runs a hook at all. A build asks for
// nothing it does not run, whatever a plugin claims.
func offers(hook string) bool {
	for _, offered := range OfferedHooks {
		if offered == hook {
			return true
		}
	}
	return false
}

// merge folds a plugin result into the site.
func (site *Site) merge(result PluginResult) {
	if len(result.Pages) > 0 {
		site.Pages = append(site.Pages, result.Pages...)
	}
	for target, body := range result.Replacements {
		if replaced := site.find(target); replaced != nil {
			replaced.Body = body
			replaced.Plain = plainFromHTML(body)
		}
	}
	site.Diagnostics = append(site.Diagnostics, result.Diagnostics...)
	relink(site)
}

// find returns the page a target names.
func (site *Site) find(target string) *SitePage {
	for _, page := range site.Pages {
		if page.Target == target {
			return page
		}
	}
	return nil
}

// relink sorts the pages, regroups the sidebar and rewires the neighbours,
// because a plugin can add pages at any point.
func relink(site *Site) {
	// Within a section the reading order is the order the pages were already
	// in, so each page is ranked by its position and the sort only moves pages
	// between sections. Returning true for every pair in a section is not an
	// ordering at all, and would shuffle a section's pages arbitrarily.
	rank := map[*SitePage]int{}
	for _, group := range site.Sections {
		for position, page := range group.Pages {
			rank[page] = position
		}
	}
	sort.SliceStable(site.Pages, func(i, j int) bool {
		left, right := site.Pages[i], site.Pages[j]
		if left.Section != right.Section {
			return left.Section < right.Section
		}
		return rank[left] < rank[right]
	})
	site.Sections = nil
	site.groupSections(nil)
	linkNeighbours(site)
}

// plainFromHTML is the text of a rendered body, which a plugin's page carries
// when it does not know the source text. Markup is dropped rather than escaped,
// because the search index stores words and not the page that draws them.
func plainFromHTML(body string) string {
	out := []rune{}
	depth := 0
	for _, r := range body {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			out = append(out, r)
		}
	}
	return string(out)
}
