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
// The finding and the running of plugins are the toolchain's; this file holds
// what a documentation build asks them about and what it does with the answer.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import (
	"sort"

	"rho/src/plugins"
)

/* -------------------------------------------------------------------------- */
/*  Describing the site                                                       */
/* -------------------------------------------------------------------------- */

// The payload shapes are the published ones: a plugin decodes the envelope and
// then the payload, and the two sides of that have to be one definition or a
// field renamed on one side is a field a plugin silently stops seeing.
type (
	sitePayload = SitePayload
	pagePayload = PagePayload
)

// describe renders the site as a plugin would see it.
func (site *Site) describe(out string) sitePayload {
	payload := sitePayload{Site: site.Title, Out: out, Root: site.Root}
	for _, page := range site.Pages {
		payload.Pages = append(payload.Pages, PluginPage{
			Target:  page.Target,
			Title:   page.Title,
			Href:    page.Href,
			Section: page.Section,
		})
	}
	return payload
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                   */
/* -------------------------------------------------------------------------- */

// RunPlugins runs the plugins this site was built with, and folds what they
// answered in.
//
// A plugin failure is a diagnostic on the build, not a failure of the build: the
// documentation of everything else is still worth writing.
func (site *Site) RunPlugins(out string) {
	if len(site.plugins) == 0 {
		return
	}
	// The whole-site pass comes first, so a per-page pass sees the pages a
	// whole-site plugin added.
	site.merge(site.run(HookIndex, site.describe(out)))

	// The per-page pass runs once for every page, with that page in the request.
	// A plugin that renders a page itself answers with a replacement body rather
	// than adding a second page with the same target.
	//
	// The pages are copied before the loop: merging an answer reorders the
	// slice, and a loop over a slice that is reordered underneath it visits a
	// page twice and skips another.
	snapshot := append([]*SitePage{}, site.Pages...)
	for _, page := range snapshot {
		payload := pagePayload{Site: site.Title, Page: PluginPage{
			Target:  page.Target,
			Title:   page.Title,
			Href:    page.Href,
			Section: page.Section,
			HTML:    page.Body,
			Plain:   page.Plain,
		}}
		// The whole set travels with every request, so a page hook can also see
		// the site the page belongs to.
		site.merge(site.run(HookPage, payload))
	}
}

// run answers one hook and returns what the plugins said about it.
func (site *Site) run(hook string, payload any) PluginResult {
	if !offers(hook) {
		return PluginResult{}
	}
	// A run asks for nothing it does not run, whatever a plugin claims.
	answer := plugins.RunHooks(site.plugins, ComponentName, hook, payload)
	result := PluginResult{Replacements: map[string]string{}}
	for _, page := range answer.Pages {
		// A plugin names a page and may leave the rest to the build: its
		// address is the page's own name, and a page with no section belongs
		// with the other pages a plugin added.
		href := page.Href
		if href == "" {
			href = page.Target + ".html"
		}
		section := page.Section
		if section == "" {
			section = DefaultExtensionSection
		}
		result.Pages = append(result.Pages, &SitePage{
			Target:  page.Target,
			Title:   page.Title,
			Href:    href,
			Section: section,
			Body:    page.HTML,
			Plain:   page.Plain,
		})
	}
	if page := pageOf(payload); page != nil {
		result.Replacements[page.Target] = answer.Replacements[page.Target]
	}
	for _, diagnostic := range answer.Diagnostics {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Message: diagnostic.String()})
	}
	return result
}

// pageOf is the page a per-page request carries, or nil for a whole-site one.
func pageOf(payload any) *PluginPage {
	if one, ok := payload.(pagePayload); ok {
		return &one.Page
	}
	return nil
}

// merge folds what the plugins answered into the site.
func (site *Site) merge(result PluginResult) {
	if len(result.Pages) > 0 {
		site.Pages = append(site.Pages, result.Pages...)
	}
	for target, body := range result.Replacements {
		if body == "" {
			continue
		}
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
