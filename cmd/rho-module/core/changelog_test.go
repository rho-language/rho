//
// @file changelog_test.go
// @brief What a publish reads out of a module's release history.
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
	"strings"
	"testing"
)

const sampleChangelog = `# Changelog

## [0.2.0] - 2026-02-01

Added a thing.

### Fixed

A crash that a ` + "`" + `quote` + "`" + ` and a "jack" caused.

## [0.1.0] - 2026-01-01

Initial release.
`

// The note for a version is its section, subheadings included, and nothing from
// the release after it.
func TestReleaseForReadsOneSection(t *testing.T) {
	note, date, ok := ReleaseFor(sampleChangelog, "0.2.0")
	if !ok {
		t.Fatal("the section for 0.2.0 was not found")
	}
	if date != "2026-02-01" {
		t.Errorf("date = %q, want 2026-02-01", date)
	}
	for _, want := range []string{"Added a thing.", "A crash that", "jack"} {
		if !strings.Contains(note, want) {
			t.Errorf("the note is missing %q: %q", want, note)
		}
	}
	if strings.Contains(note, "Initial release.") {
		t.Errorf("the note ran into the next release: %q", note)
	}
	// The quote in the note survives: it is text, not markup.
	if !strings.Contains(note, "\"jack\"") {
		t.Errorf("a quote in the note was lost: %q", note)
	}
}

// A version the changelog does not mention has no note, rather than the first or
// nearest one.
func TestReleaseForWithoutTheVersion(t *testing.T) {
	if _, _, ok := ReleaseFor(sampleChangelog, "9.9.9"); ok {
		t.Error("a version that is not in the changelog reported a note")
	}
	if _, _, ok := ReleaseFor("", "0.1.0"); ok {
		t.Error("an empty changelog reported a note")
	}
}

// A heading with the bare version, without the brackets the convention uses, is
// still that version's section.
func TestReleaseForAcceptsABareVersionHeading(t *testing.T) {
	changelog := "## 0.1.0\n\nInitial release.\n"
	note, _, ok := ReleaseFor(changelog, "0.1.0")
	if !ok || !strings.Contains(note, "Initial release.") {
		t.Errorf("a bare version heading was not read: %q ok=%v", note, ok)
	}
}

// ReadChangelog reads the file beside the manifest, and answers "" when there is
// none rather than failing.
func TestReadChangelog(t *testing.T) {
	dir := t.TempDir()
	if got := ReadChangelog(dir); got != "" {
		t.Errorf("a directory with no changelog read %q", got)
	}
	body := "## [0.1.0] - 2026-01-01\n\nInitial release.\n"
	if err := os.WriteFile(filepath.Join(dir, ChangelogName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ReadChangelog(dir); got != body {
		t.Errorf("read %q, want %q", got, body)
	}
}
