//
// @file listing.go
// @brief Listings a Rho handler can read.
//
// Rho cannot walk a nested document: a handler has no way to iterate a JSON
// array or reach into an object. The store port solved this by rendering its
// results as tab-separated lines, and this is the same answer for
// documentation: a listing is a line-based one, with the fields in a fixed
// order, so a Rho handler can render a site index, an API index and a set of
// diagnostics without the generator growing a data structure language for its
// own output.
//
// A field never breaks the line format. Tabs and newlines inside a value are
// folded to spaces, so a title written over two lines stays one record.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import (
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Sites                                                                     */
/* -------------------------------------------------------------------------- */

// SiteListing renders a site as one line per page: the target, the title, the
// section and the module, separated by tabs.
//
// A caller reads a field by position, so the delimiters are the structure and
// are never folded: each value is made safe on its own, and the values are then
// joined by a tab.
func SiteListing(site *Site) string {
	lines := []string{record(site.Title, site.Project, site.Version, itoa(len(site.Pages)))}
	for _, page := range site.Pages {
		lines = append(lines, record(page.Target, page.Title, page.Section, page.Module))
	}
	return strings.Join(lines, "\n")
}

/* -------------------------------------------------------------------------- */
/*  The reference                                                             */
/* -------------------------------------------------------------------------- */

// ApiListing renders a reference as one line per module: the name, its target,
// and how much it declares, separated by tabs. The header says the same, so a
// handler can show the shape of the library without counting.
func ApiListing(set ApiSet) string {
	lines := []string{record("name", "target", "functions", "structs", "externs", "source")}
	for _, module := range set.Modules {
		lines = append(lines, record(
			module.Name,
			ApiTarget(module.Name),
			itoa(len(module.Functions)),
			itoa(len(module.Structs)),
			itoa(len(module.Externs)),
			module.Path,
		))
	}
	return strings.Join(lines, "\n")
}

/* -------------------------------------------------------------------------- */
/*  Diagnostics                                                               */
/* -------------------------------------------------------------------------- */

// DiagnosticListing renders the problems a build found, one per line: the path,
// the line number and the message, separated by tabs. An empty answer means the
// build was clean, which is the case a caller checks for first.
func DiagnosticListing(site *Site, unreadable []string) string {
	lines := []string{}
	for _, path := range unreadable {
		lines = append(lines, record(path, "0", "no readable module manifest"))
	}
	for _, diagnostic := range site.Diagnostics {
		lines = append(lines, record(diagnostic.Path, itoa(diagnostic.Line), diagnostic.Message))
	}
	for _, request := range site.Requests {
		lines = append(lines, record(request.Page, itoa(request.Line), "reference names no target: "+request.Label))
	}
	return strings.Join(lines, "\n")
}

/* -------------------------------------------------------------------------- */
/*  Fields                                                                    */
/* -------------------------------------------------------------------------- */

// record is one line of a listing: values made safe on their own, then joined
// by the delimiter. Folding the finished line would fold the delimiters with it,
// which is what makes a listing unreadable by position.
func record(values ...string) string {
	safe := make([]string, 0, len(values))
	for _, value := range values {
		safe = append(safe, field(value))
	}
	return strings.Join(safe, "\t")
}

// field makes one value safe to carry: a tab, a carriage return and a newline
// would each break the line format, so each becomes a space.
func field(value string) string {
	replaced := strings.ReplaceAll(value, "\t", " ")
	replaced = strings.ReplaceAll(replaced, "\r", " ")
	replaced = strings.ReplaceAll(replaced, "\n", " ")
	return strings.TrimSpace(replaced)
}
