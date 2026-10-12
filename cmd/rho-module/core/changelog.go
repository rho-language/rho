//
// @file changelog.go
// @brief A module's release history, read from the CHANGELOG.md it ships.
//
// A module's description says what the module is, and does not change between
// releases; two versions of one module therefore describe themselves identically,
// and a reader looking at them cannot tell which to take. The changelog is the
// other half: what changed, per version, in the module's own words.
//
// It lives in the module's folder as CHANGELOG.md, travels inside the package,
// and one section of it is sent with each publish, so the registry shows a
// version's release note without unpacking anything and without a second copy
// somewhere that would drift from the file the module's authors edit.
//
// The format is the one people already write — a `## <version>` heading per
// release, the date beside it, and the note under it:
//
//	# Changelog
//
//	## [0.1.1] - 2026-10-12
//
//	Added a remote registry section.
//
//	## [0.1.0] - 2026-10-01
//
//	Initial release.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ChangelogName is the file a module's release history is read from. It sits
// beside module.yml, so where a module is there its history is too.
const ChangelogName = "CHANGELOG.md"

// releaseDate matches the date on a release heading, in the form the changelog
// convention uses.
var releaseDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

// ReadChangelog returns the module's changelog, or "" when it has none.
func ReadChangelog(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, ChangelogName))
	if err != nil {
		return ""
	}
	return string(data)
}

// ReleaseFor returns the note and date for one version, and whether the
// changelog has a section for it.
//
// The note is everything under the version's heading up to the next release
// heading, trimmed. Subheadings under it stay: a release that groups its changes
// keeps the grouping.
func ReleaseFor(changelog, version string) (note, date string, ok bool) {
	if strings.TrimSpace(changelog) == "" {
		return "", "", false
	}
	lines := strings.Split(changelog, "\n")
	start := -1
	for i, line := range lines {
		if !isReleaseHeading(line) {
			continue
		}
		if headingVersion(line) == version {
			start = i
			date = releaseDate.FindString(line)
			break
		}
	}
	if start < 0 {
		return "", "", false
	}
	body := []string{}
	for i := start + 1; i < len(lines); i++ {
		if isReleaseHeading(lines[i]) || isTitleHeading(lines[i]) {
			break
		}
		body = append(body, lines[i])
	}
	return strings.TrimSpace(strings.Join(body, "\n")), date, true
}

// isReleaseHeading reports whether a line opens a release section: `## <version>`
// and not a deeper `### <subsection>`, which belongs to the release above it.
func isReleaseHeading(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ")
}

// isTitleHeading reports whether a line is the document's own title, which ends
// the last release section as surely as another release heading does.
func isTitleHeading(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ")
}

// headingVersion is the version a release heading names: `[1.2.3]` or `1.2.3`.
func headingVersion(line string) string {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "## "))
	if strings.HasPrefix(text, "[") {
		if end := strings.Index(text, "]"); end > 0 {
			return text[1:end]
		}
	}
	if fields := strings.Fields(text); len(fields) > 0 {
		return fields[0]
	}
	return ""
}
