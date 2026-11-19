//
// @file pages_test.go
// @brief The pages written for a fixture family, against a golden copy.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package docreq

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden file")

// fixture copies testdata/workspace to a temporary directory, with the keeper's
// results where rho test writes them (kept outside build/ in the tree, which git
// ignores), and returns the site's modules: every one but robohub-mesh, which is
// a member of the family that is not on the site.
func fixture(t *testing.T, withResults bool) (string, []Module) {
	t.Helper()
	t.Setenv("ROBOHUB_WORKSPACE", "")
	t.Setenv("RHO_WORKSPACE", "")
	dst := t.TempDir()
	src := filepath.Join("testdata", "workspace")
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if strings.Contains(rel, "test-reports.fixture") {
			if !withResults {
				return nil
			}
			target = filepath.Join(dst, strings.Replace(rel, "test-reports.fixture", filepath.Join("build", "test-reports"), 1))
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		os.MkdirAll(filepath.Dir(target), 0o755)
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	modules := []Module{
		{Name: "robohub", Root: filepath.Join(dst, "robohub"), Section: "process"},
		{Name: "keeper", Root: filepath.Join(dst, "robohub-keeper"), Section: "applications"},
		{Name: "notes", Root: filepath.Join(dst, "notes"), Section: "build"},
	}
	return dst, modules
}

func byAddress(pages []Page) map[string]Page {
	out := map[string]Page{}
	for _, page := range pages {
		out[page.Module+"/"+page.Path] = page
	}
	return out
}

// The pages a family gets: a requirements page per module that declares any, the
// trace on the root, a decision index and a page per decision; nothing for a
// module that keeps neither.
func TestThePagesOfAFamily(t *testing.T) {
	dir, modules := fixture(t, true)
	pages, notes := Pages(modules)
	if len(notes) != 0 {
		t.Errorf("unexpected notes %v", notes)
	}
	got := []string{}
	for _, page := range pages {
		label := page.Module + "/" + page.Path
		if page.Unlisted {
			label += " (unlisted)"
		}
		got = append(got, label)
	}
	want := "robohub/requirements/index robohub/requirements/trace robohub/adr/index robohub/adr/0001-mesh-bus (unlisted) " +
		"keeper/requirements/index keeper/adr/index keeper/adr/0001-sessions-on-disk (unlisted) keeper/adr/0002-sessions-in-the-store (unlisted)"
	if strings.Join(got, " ") != want {
		t.Fatalf("pages:\n%s\nwant\n%s", strings.Join(got, " "), want)
	}
	all := byAddress(pages)
	if source := all["keeper/adr/0002-sessions-in-the-store"].Source; source != filepath.Join(dir, "robohub-keeper", "docs", "architecture", "adr", "0002-sessions-in-the-store.md") {
		t.Errorf("a decision names its file, got %q", source)
	}

	var b strings.Builder
	for _, page := range pages {
		b.WriteString("======== " + page.Module + "/" + page.Path + "\n")
		b.WriteString(page.Markdown)
	}
	golden := filepath.Join("testdata", "golden", "pages.md")
	if *update {
		os.MkdirAll(filepath.Dir(golden), 0o755)
		if err := os.WriteFile(golden, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if b.String() != string(data) {
		t.Fatalf("pages differ from %s (go test ./... -run TestThePagesOfAFamily -update rewrites it):\n%s", golden, b.String())
	}
}

// What is on the site is a reference; what is not is its name, so no page carries
// a reference that resolves to nothing. A decision is its repository's label.
func TestLinksReachOnlyWhatIsOnTheSite(t *testing.T) {
	_, modules := fixture(t, true)
	pages, _ := Pages(modules)
	all := byAddress(pages)
	root := all["robohub/requirements/index"].Markdown
	for _, want := range []string{"### SYS-0001: Survive a power cut {#SYS-0001}", "{ref}`KEEP-0001`", "`MESH-0001`", "{ref}`ADR-0001 <robohub:ADR-0001>`"} {
		if !strings.Contains(root, want) {
			t.Errorf("robohub requirements lack %q:\n%s", want, root)
		}
	}
	if strings.Contains(root, "{ref}`MESH-0001`") {
		t.Error("a requirement not on the site is not a reference")
	}
	keeper := all["keeper/requirements/index"].Markdown
	for _, want := range []string{"{ref}`ADR-0001 <robohub-keeper:ADR-0001>`", "{ref}`robohub:ADR-0001 <robohub:ADR-0001>`", "{ref}`SYS-0001`", "**Evidence:** passing",
		"`session_survives_restart` (`tests/session_test.rho:3`): pass", "Evidence is from the test run of 2026-11-19T18:30:00Z"} {
		if !strings.Contains(keeper, want) {
			t.Errorf("keeper requirements lack %q:\n%s", want, keeper)
		}
	}
	adr := all["keeper/adr/0001-sessions-on-disk"].Markdown
	if !strings.HasPrefix(adr, "# ADR-0001: Sessions on disk {#robohub-keeper:ADR-0001}\n") || !strings.Contains(adr, "**Superseded by:** {ref}`ADR-0002 <robohub-keeper:ADR-0002>`") {
		t.Errorf("unexpected decision page:\n%s", adr)
	}
	if mesh := all["robohub/adr/0001-mesh-bus"].Markdown; strings.Count(mesh, "Nodes talk over a mesh bus") != 1 {
		t.Errorf("the body's own title is not repeated:\n%s", mesh)
	}
	trace := all["robohub/requirements/trace"].Markdown
	for _, want := range []string{"{ref}`robohub-keeper:ADR-0001 <robohub-keeper:ADR-0001>`", "| SYS | System | 2 |", "`MESH-0001`"} {
		if !strings.Contains(trace, want) {
			t.Errorf("the trace lacks %q:\n%s", want, trace)
		}
	}
}

// Without test results a page says so and shows no evidence, rather than calling
// everything untested.
func TestWithoutResultsThereIsNoEvidence(t *testing.T) {
	_, modules := fixture(t, false)
	pages, _ := Pages(modules)
	keeper := byAddress(pages)["keeper/requirements/index"].Markdown
	if !strings.Contains(keeper, "No test results were available") || strings.Contains(keeper, "Evidence") {
		t.Errorf("unexpected page:\n%s", keeper)
	}
}

// A module that is not a repository of its own (its requirements would be its
// enclosing repository's) and a module with nothing get no pages.
func TestOnlyARepositoryGetsPages(t *testing.T) {
	dir, _ := fixture(t, false)
	nested := filepath.Join(dir, "robohub", "modules", "inner")
	os.MkdirAll(filepath.Join(nested, "docs", "doc"), 0o755)
	os.WriteFile(filepath.Join(nested, "module.yml"), []byte("module:\n  name: inner\n  type: rho\n  version: 0.1.0\n"), 0o644)
	pages, _ := Pages([]Module{{Name: "inner", Root: nested}, {Name: "notes", Root: filepath.Join(dir, "notes")}})
	if len(pages) != 0 {
		t.Fatalf("expected no pages, got %+v", pages)
	}
}

// A family with errors still gets its pages, with a note to run the checker.
func TestErrorsAreANote(t *testing.T) {
	dir, modules := fixture(t, false)
	os.WriteFile(filepath.Join(dir, "robohub-keeper", "docs", "requirements", "broken.yml"), []byte("level: NODE\nprefix: KEEP\nrequirements:\n  - id: KEEP-0003\n"), 0o644)
	pages, notes := Pages(modules)
	if len(notes) == 0 || !strings.Contains(notes[0], "rho req check") {
		t.Errorf("expected a note, got %v", notes)
	}
	if _, ok := byAddress(pages)["keeper/requirements/index"]; !ok {
		t.Error("the pages are still written")
	}
}

// A module nested in a repository with a requirements block of its own is a
// member of the family: it gets its own pages, its decisions are labelled with
// its module name, and the root's trace counts its requirements.
func TestANestedModuleGetsItsOwnPages(t *testing.T) {
	dir, modules := fixture(t, false)
	nested := filepath.Join(dir, "robohub", "modules", "forms")
	files := map[string]string{
		"module.yml": "module:\n  name: robohub-forms\n  type: rho\n  version: 0.1.0\n\nrequirements: { prefix: FORMS, parent: robohub }\n",
		"docs/requirements/node.yml": "level: NODE\nprefix: FORMS\nrequirements:\n  - id: FORMS-0001\n    title: A form validates\n    statement: S\n" +
			"    satisfies: [SYS-0001]\n    status: proposed\n    verification: test\n    decided_by: [ADR-0001]\n    changed: 2026-11-19\n",
		"docs/architecture/adr/0001-one-schema.md": "---\nid: ADR-0001\ntitle: One schema\nstatus: proposed\ndate: 2026-11-19\nrequirements: [FORMS-0001]\n---\n\nbody\n",
	}
	for rel, text := range files {
		path := filepath.Join(nested, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(text), 0o644)
	}
	modules = append(modules, Module{Name: "forms", Root: nested, Section: "modules"})
	pages, notes := Pages(modules)
	if len(notes) != 0 {
		t.Errorf("unexpected notes %v", notes)
	}
	all := byAddress(pages)
	if page := all["forms/requirements/index"].Markdown; !strings.Contains(page, "### FORMS-0001: A form validates {#FORMS-0001}") ||
		!strings.Contains(page, "{ref}`ADR-0001 <robohub-forms:ADR-0001>`") {
		t.Errorf("the nested module's requirements:\n%s", page)
	}
	if adr := all["forms/adr/0001-one-schema"].Markdown; !strings.HasPrefix(adr, "# ADR-0001: One schema {#robohub-forms:ADR-0001}\n") {
		t.Errorf("the nested module's decision:\n%s", adr)
	}
	if strings.Contains(all["robohub/requirements/index"].Markdown, "### FORMS-0001") {
		t.Error("the enclosing repository's page does not hold the nested module's requirements")
	}
	if trace := all["robohub/requirements/trace"].Markdown; !strings.Contains(trace, "{ref}`FORMS-0001`") {
		t.Errorf("the trace counts the nested module:\n%s", trace)
	}
}
