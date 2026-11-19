//
// @file edit_test.go
// @brief Edits change the lines of one value and nothing else.
//
// Each case is a whole file before and after: comments, blank lines, quoting
// and key order that the edit has no business with must come through byte for
// byte, which is what comparing whole files shows.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rhoreq

import (
	"strings"
	"testing"
)

const editSource = `# Gateway requirements.   (header comment)
level: NODE
prefix: GW
requirements:
  # The first one.
  - id: GW-0001
    title: "Sessions survive"   # keep this comment
    statement: >
      After a restart the gateway shall
      re-establish every session.
    satisfies: [SYS-0001]   # upward
    status: approved
    verification: test
    changed: 2026-01-01

  - id: GW-0002
    title: Second
    statement: Two.
    satisfies:
      - SYS-0001   # first parent
      - SYS-0002
    # a comment between keys
    status: draft
    verification: test
    tags: [a]
    changed: 2026-01-01
# trailing comment
`

func edited(t *testing.T, change func(d *Doc) error) string {
	t.Helper()
	doc := NewDoc(editSource, false)
	if err := change(doc); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Root(); err != nil {
		t.Fatalf("the edit left the file unreadable: %v\n%s", err, doc.String())
	}
	return doc.String()
}

func expectDiff(t *testing.T, got string, replacements ...string) {
	t.Helper()
	want := editSource
	for index := 0; index+1 < len(replacements); index += 2 {
		if !strings.Contains(want, replacements[index]) {
			t.Fatalf("test bug: %q is not in the source", replacements[index])
		}
		want = strings.Replace(want, replacements[index], replacements[index+1], 1)
	}
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetAScalarKeepsItsComment(t *testing.T) {
	got := edited(t, func(d *Doc) error {
		return d.Set(RequirementItem("GW-0001"), "title", "Sessions: survive", RequirementKeys)
	})
	expectDiff(t, got, `title: "Sessions survive"   # keep this comment`, `title: "Sessions: survive"   # keep this comment`)
}

func TestSetABlockScalarKeepsItsStyle(t *testing.T) {
	got := edited(t, func(d *Doc) error {
		return d.Set(RequirementItem("GW-0001"), "statement", "A new statement.", RequirementKeys)
	})
	expectDiff(t, got, "      After a restart the gateway shall\n      re-establish every session.\n", "      A new statement.\n")
}

func TestInsertAMissingKeyInOrder(t *testing.T) {
	got := edited(t, func(d *Doc) error { return d.Set(RequirementItem("GW-0001"), "owner", "ada", RequirementKeys) })
	expectDiff(t, got, "    verification: test\n    changed: 2026-01-01\n\n", "    verification: test\n    owner: ada\n    changed: 2026-01-01\n\n")
	got = edited(t, func(d *Doc) error { return d.Set(RequirementItem("GW-0002"), "rationale", "Because.", RequirementKeys) })
	expectDiff(t, got, "    statement: Two.\n", "    statement: Two.\n    rationale: Because.\n")
}

func TestFlowListAddAndRemove(t *testing.T) {
	got := edited(t, func(d *Doc) error { return d.Add(RequirementItem("GW-0001"), "satisfies", "SYS-0003", RequirementKeys) })
	expectDiff(t, got, "satisfies: [SYS-0001]   # upward", "satisfies: [SYS-0001, SYS-0003]   # upward")
	got = edited(t, func(d *Doc) error { return d.Remove(RequirementItem("GW-0001"), "satisfies", "SYS-0001") })
	expectDiff(t, got, "satisfies: [SYS-0001]   # upward", "satisfies: []   # upward")
}

func TestBlockListAddAndRemoveOneLine(t *testing.T) {
	got := edited(t, func(d *Doc) error { return d.Add(RequirementItem("GW-0002"), "satisfies", "SYS-0009", RequirementKeys) })
	expectDiff(t, got, "      - SYS-0002\n", "      - SYS-0002\n      - SYS-0009\n")
	got = edited(t, func(d *Doc) error { return d.Remove(RequirementItem("GW-0002"), "satisfies", "SYS-0002") })
	expectDiff(t, got, "      - SYS-0002\n", "")
}

func TestDeleteAKey(t *testing.T) {
	got := edited(t, func(d *Doc) error { return d.Delete(RequirementItem("GW-0002"), "tags") })
	expectDiff(t, got, "    tags: [a]\n", "")
	doc := NewDoc(editSource, false)
	if err := doc.Delete(RequirementItem("GW-0002"), "id"); err == nil {
		t.Fatal("the key on the dash line was removed")
	}
}

func TestAppendFollowsTheFilesStyle(t *testing.T) {
	got := edited(t, func(d *Doc) error {
		return d.AppendItem("requirements", []KV{{"id", "GW-0003"}, {"title", "Third"}})
	})
	expectDiff(t, got, "    changed: 2026-01-01\n# trailing comment\n", "    changed: 2026-01-01\n\n  - id: GW-0003\n    title: Third\n# trailing comment\n")

	empty := NewDoc("level: NODE\nprefix: GW\nrequirements: []   # none yet\n", false)
	if err := empty.AppendItem("requirements", []KV{{"id", "GW-0001"}}); err != nil {
		t.Fatal(err)
	}
	if want := "level: NODE\nprefix: GW\nrequirements:   # none yet\n  - id: GW-0001\n"; empty.String() != want {
		t.Fatalf("got %q", empty.String())
	}
}

func TestFrontMatterEditLeavesTheBody(t *testing.T) {
	src := "---\nid: ADR-0002\nstatus: accepted   # decided\nsupersedes: []\n---\n\n## Context\n\nstatus: not yaml\n"
	doc := NewDoc(src, true)
	if err := doc.Set(RootMap, "status", "superseded", ADRKeys); err != nil {
		t.Fatal(err)
	}
	if err := doc.Add(RootMap, "superseded_by", "ADR-0003", ADRKeys); err != nil {
		t.Fatal(err)
	}
	want := "---\nid: ADR-0002\nstatus: superseded   # decided\nsupersedes: []\nsuperseded_by: [ADR-0003]\n---\n\n## Context\n\nstatus: not yaml\n"
	if doc.String() != want {
		t.Fatalf("got:\n%s", doc.String())
	}
}

func TestCRLFSurvives(t *testing.T) {
	doc := NewDoc("level: NODE\r\nprefix: GW\r\nrequirements:\r\n  - id: GW-0001\r\n    title: x\r\n", false)
	if err := doc.Set(RequirementItem("GW-0001"), "title", "z", RequirementKeys); err != nil {
		t.Fatal(err)
	}
	if want := "level: NODE\r\nprefix: GW\r\nrequirements:\r\n  - id: GW-0001\r\n    title: z\r\n"; doc.String() != want {
		t.Fatalf("got %q", doc.String())
	}
}

func TestFormatScalarQuotesOnlyWhenNeeded(t *testing.T) {
	cases := map[string]string{
		"plain words":   "plain words",
		"a: b":          `"a: b"`,
		"yes":           `"yes"`,
		"12":            `"12"`,
		"2026-11-19":    "2026-11-19",
		"- dash":        `"- dash"`,
		`say "hi"`:      `say "hi"`,
		"x #y":          `"x #y"`,
		"line\nbreak":   `"line\nbreak"`,
		"30 s, at most": "30 s, at most",
	}
	for in, want := range cases {
		if got := formatScalar(in); got != want {
			t.Errorf("formatScalar(%q) = %s, want %s", in, got, want)
		}
	}
	if got := formatFlow([]string{"a", "b,c"}); got != `[a, "b,c"]` {
		t.Errorf("formatFlow = %s", got)
	}
}
