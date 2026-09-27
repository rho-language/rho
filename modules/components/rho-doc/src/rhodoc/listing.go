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
	"fmt"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Sites                                                                     */
/* -------------------------------------------------------------------------- */

// SiteListing renders a site as one line per page: the target, the title, the
// section and the module, separated by tabs.
func SiteListing(site *Site) string {
	lines := []string{}
	header := strings.Join([]string{site.Title, site.Project, site.Version, itoa(len(site.Pages))}, "\t")
	lines = append(lines, field(header)...)
	for _, page := range site.Pages {
		lines = append(lines, field(strings.Join([]string{page.Target, page.Title, page.Section, page.Module}, "\t"))...)
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
	lines := []string{
		strings.Join([]string{"name", "target", "functions", "structs", "externs", "source"}, "\t"),
	}
	for _, module := range set.Modules {
		lines = append(lines, field(strings.Join([]string{
			module.Name,
			ApiTarget(module.Name),
			itoa(len(module.Functions)),
			itoa(len(module.Structs)),
			itoa(len(module.Externs)),
			module.Path,
		}, "\t"))...)
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
		lines = append(lines, field(strings.Join([]string{path, "0", "no readable module manifest"}, "\t"))...)
	}
	for _, diagnostic := range site.Diagnostics {
		lines = append(lines, field(strings.Join([]string{diagnostic.Path, itoa(diagnostic.Line), diagnostic.Message}, "\t"))...)
	}
	for _, request := range site.Requests {
		lines = append(lines, field(strings.Join([]string{request.Page, itoa(request.Line), "reference names no target: " + request.Label}, "\t"))...)
	}
	return strings.Join(lines, "\n")
}

/* -------------------------------------------------------------------------- */
/*  Fields                                                                    */
/* -------------------------------------------------------------------------- */

// field folds a value so it cannot break the line format: a tab becomes a
// space, a newline becomes a space, and two become one.
func field(value string) []string {
	replaced := strings.ReplaceAll(value, "\t", " ")
	replaced = strings.ReplaceAll(replaced, "\r", " ")
	replaced = strings.ReplaceAll(replaced, "\n", " ")
	return []string{strings.TrimSpace(replaced)}
}

// count renders a number for a header line, which is how a listing reports what
// it holds without the caller counting lines.
func count(value int) string { return fmt.Sprint(value) }
