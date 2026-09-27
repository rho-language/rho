//
// @file site_test.go
// @brief Tests for building a documentation site.
//
// A site is the part of the generator a reader sees, so these tests cover what
// a page links to, what the sidebar lists, and what a build says when a
// cross-reference or a source is wrong. A build that quietly drops either is a
// build whose documentation is quietly wrong.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                  */
/* -------------------------------------------------------------------------- */

// writeFile writes a file below a root, creating the directories it needs.
func writeFile(t *testing.T, root, relative, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeComponentModule writes a module that carries a documentation section.
func writeComponentModule(t *testing.T, root, name, section, sources string) {
	t.Helper()
	writeFile(t, root, name+"/module.yml", "module:\n"+
		"  name: "+name+"\n"+
		"  type: component\n"+
		"  version: 0.1.0\n"+
		"  description: The "+name+" component\n"+
		"\n"+
		"main: "+name+"\n"+
		"\n"+
		"docs:\n"+
		"  section: "+section+"\n"+
		"  order: \"10\"\n"+
		"  index: docs/index.md\n"+
		"  sources:\n"+
		sources)
}

/* -------------------------------------------------------------------------- */
/*  Discovery                                                                 */
/* -------------------------------------------------------------------------- */

func TestDiscoverModulesReadsTheDeclaredSources(t *testing.T) {
	root := t.TempDir()
	writeComponentModule(t, root, "rho-log", "components", "    - docs/index.md\n    - docs/records.md\n")
	writeFile(t, root, "rho-log/docs/index.md", "# Logger\n")
	writeFile(t, root, "rho-log/docs/records.md", "# Records\n")

	modules, missing := DiscoverModules(root)
	if len(missing) != 0 {
		t.Fatalf("expected every module to be readable, missing %+v", missing)
	}
	if len(modules) != 1 {
		t.Fatalf("expected one module, got %d", len(modules))
	}
	module := modules[0]
	if module.Name != "rho-log" || module.Section != "components" || module.Order != 10 {
		t.Fatalf("unexpected module: %+v", module)
	}
	if module.Description != "The rho-log component" {
		t.Fatalf("expected the manifest description, got %q", module.Description)
	}
	if len(module.Sources) != 2 || filepath.Base(module.Sources[0]) != "index.md" {
		t.Fatalf("unexpected sources: %+v", module.Sources)
	}
}

func TestDiscoverModulesReadsTheDocsConvention(t *testing.T) {
	// A module that drops a file into docs/ gets it documented without having
	// to declare it, which is the point of the convention.
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	writeFile(t, root, "app/docs/guide.md", "# Guide\n")

	modules, _ := DiscoverModules(root)
	if len(modules) != 1 || len(modules[0].Sources) != 1 {
		t.Fatalf("expected the convention to find the page, got %+v", modules)
	}
}

func TestDiscoverModulesExpandsAGlob(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndocs:\n  sources:\n    - docs/*.md\n")
	writeFile(t, root, "app/docs/one.md", "# One\n")
	writeFile(t, root, "app/docs/two.md", "# Two\n")
	writeFile(t, root, "app/docs/notes.txt", "not documentation\n")

	modules, _ := DiscoverModules(root)
	if len(modules) != 1 || len(modules[0].Sources) != 2 {
		t.Fatalf("expected the glob to find two pages, got %+v", modules[0].Sources)
	}
}

func TestDiscoverModulesSkipsBuildOutput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	writeFile(t, root, "app/docs/page.md", "# Page\n")
	writeFile(t, root, "app/build/generated/docs.md", "# Generated copy\n")

	modules, _ := DiscoverModules(root)
	for _, source := range modules[0].Sources {
		if strings.Contains(source, "build") {
			t.Fatalf("expected build output to be skipped, got %s", source)
		}
	}
}

func TestDiscoverModulesSkipsATreeWithoutAManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	// A manifest that does not parse is reported rather than half-read.
	writeFile(t, root, "broken/module.yml", "module:\n  name: broken\n")

	modules, missing := DiscoverModules(root)
	if len(modules) != 1 {
		t.Fatalf("expected the readable module, got %d", len(modules))
	}
	if len(missing) != 1 || !strings.Contains(missing[0], "broken") {
		t.Fatalf("expected the unreadable manifest to be reported, got %+v", missing)
	}
}

/* -------------------------------------------------------------------------- */
/*  Sites                                                                     */
/* -------------------------------------------------------------------------- */

func TestBuildSiteWritesPagesASidebarAndAnIndex(t *testing.T) {
	root := t.TempDir()
	writeComponentModule(t, root, "rho-log", "components", "    - docs/index.md\n")
	writeFile(t, root, "rho-log/docs/index.md", "# Logger\n\nThe logger is a component.\n")

	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{Title: "Rho", Out: filepath.Join(root, "build", "docs")})
	if len(site.Pages) != 1 || site.Pages[0].Title != "Logger" {
		t.Fatalf("expected the logger page, got %+v", site.Pages)
	}

	out := filepath.Join(root, "out")
	if err := site.Write(out); err != nil {
		t.Fatalf("Write: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatalf("the index was not written: %v", err)
	}
	pageDocument, err := os.ReadFile(filepath.Join(out, "components", "rho-log", "index.html"))
	if err != nil {
		t.Fatalf("the page was not written: %v", err)
	}
	if !strings.Contains(string(page), "Logger") {
		t.Fatalf("expected the index to list the page, got:\n%s", page)
	}
	if !strings.Contains(string(pageDocument), "The logger is a component.") {
		t.Fatalf("expected the body on the page, got:\n%s", pageDocument)
	}
	if !strings.Contains(string(pageDocument), "rho-doc.css") {
		t.Fatal("expected the page to carry the stylesheet")
	}
	if _, err := os.Stat(filepath.Join(out, "static", "rho-doc.css")); err != nil {
		t.Fatalf("expected the stylesheet to travel with the site: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "search-index.json")); err != nil {
		t.Fatalf("expected a search index: %v", err)
	}
}

func TestBuildSiteResolvesACrossReference(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndocs:\n  section: guides\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n\nStart with {ref}`the guide <guide>`.\n")
	writeFile(t, root, "app/docs/guide.md", "# Guide\n\nHow to do the thing.\n")

	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	page := findPage(site, "index")
	if page == nil {
		t.Fatal("expected the index page")
	}
	if !strings.Contains(page.Body, "href=\"guides/app/guide.html\"") {
		t.Fatalf("expected the reference resolved to the page, got:\n%s", page.Body)
	}
	if len(site.Requests) != 0 {
		t.Fatalf("expected every reference to resolve, got %+v", site.Requests)
	}
}

func TestBuildSiteReportsAReferenceToNothing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n\nSee {ref}`nowhere`.\n")

	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	if len(site.Requests) != 1 {
		t.Fatalf("expected the unresolved reference to be reported, got %+v", site.Requests)
	}
	if !strings.Contains(site.Requests[0].String(), "nowhere") {
		t.Fatalf("expected the label in the report, got %s", site.Requests[0])
	}
}

func TestBuildSiteGroupsPagesIntoSections(t *testing.T) {
	root := t.TempDir()
	writeComponentModule(t, root, "rho-log", "components", "    - docs/index.md\n")
	writeComponentModule(t, root, "rho-fmt", "components", "    - docs/index.md\n")
	writeFile(t, root, "app/module.yml", "module:\n  name: registry\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndocs:\n  section: applications\n")
	writeFile(t, root, "app/docs/index.md", "# Registry\n")
	writeFile(t, root, "rho-log/docs/index.md", "# Logger\n")
	writeFile(t, root, "rho-fmt/docs/index.md", "# Formatter\n")

	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	names := []string{}
	for _, group := range site.Sections {
		names = append(names, group.Name)
	}
	if len(names) != 2 || names[0] != "applications" || names[1] != "components" {
		t.Fatalf("expected the sections in order, got %+v", names)
	}
	// Two components documented a page of the same name; neither may overwrite
	// the other, because a page is addressed by its target alone.
	hrefs := map[string]bool{}
	for _, page := range site.Pages {
		if hrefs[page.Href] {
			t.Fatalf("two pages share the address %s", page.Href)
		}
		hrefs[page.Href] = true
	}
	// Two components both have an index, so the target is claimed twice. That
	// is ordinary, and a build that references neither is not a build with a
	// defect, so nothing is reported.
	for _, diagnostic := range site.Diagnostics {
		if strings.Contains(diagnostic.Message, "declared by") {
			t.Fatalf("expected a collision nothing references to go unreported, got %q", diagnostic.Message)
		}
	}
}

func TestBuildSiteReportsAReferenceToAnAmbiguousTarget(t *testing.T) {
	// A reference to a target two pages answer to cannot be resolved: pointing
	// it at whichever page was read first would be arbitrary, so it is
	// reported and the author says which page they meant.
	root := t.TempDir()
	writeComponentModule(t, root, "rho-log", "components", "    - docs/index.md\n")
	writeComponentModule(t, root, "rho-fmt", "components", "    - docs/index.md\n")
	writeFile(t, root, "rho-log/docs/index.md", "# Logger\n\nSee {ref}`index`.\n")
	writeFile(t, root, "rho-fmt/docs/index.md", "# Formatter\n")

	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	reported := false
	for _, diagnostic := range site.Diagnostics {
		if strings.Contains(diagnostic.Message, "name the page a reference") {
			reported = true
		}
	}
	if !reported {
		t.Fatalf("expected the ambiguous reference to be reported, got %+v", site.Diagnostics)
	}
}

func TestBuildSiteLinksNeighbours(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndocs:\n  section: guides\n")
	// Pages are read in the order a reader sees them, which is by title.
	writeFile(t, root, "app/docs/a-first.md", "# Alpha\n")
	writeFile(t, root, "app/docs/b-second.md", "# Bravo\n")
	writeFile(t, root, "app/docs/c-third.md", "# Charlie\n")

	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	one, two := findPage(site, "a-first"), findPage(site, "b-second")
	if one == nil || two == nil {
		t.Fatal("expected the pages")
	}
	if one.Next == nil || one.Next.Target != two.Target {
		t.Fatalf("expected one to lead to two, got %+v", one.Next)
	}
	if two.Previous == nil || two.Previous.Target != one.Target {
		t.Fatalf("expected two to follow one, got %+v", two.Previous)
	}
	if one.Previous != nil {
		t.Fatal("expected the first page to have no previous")
	}
	// An index is what a reader arrives on, so it is read before the rest of
	// the module's pages even though its title sorts last.
	writeFile(t, root, "app/docs/index.md", "# Index\n")
	modules, _ = DiscoverModules(root)
	site = BuildSite(modules, ApiSet{}, SiteOptions{})
	if site.Pages[0].Target != "index" {
		t.Fatalf("expected the index first, got %q", site.Pages[0].Target)
	}
}

func TestSiteSearchIndexCarriesThePageText(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n\nA page about configuration files.\n")

	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	index := site.searchIndex()
	if !strings.Contains(index, "configuration files") {
		t.Fatalf("expected the page text in the index, got:\n%s", index)
	}
	if !strings.Contains(index, "\"href\": \"modules/app/index.html\"") {
		t.Fatalf("expected the address in the index, got:\n%s", index)
	}
}

/* -------------------------------------------------------------------------- */
/*  The API reference                                                         */
/* -------------------------------------------------------------------------- */

func TestReadApiSetDescribesWhatTheSourceDeclares(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "std/core/option.rho", "module std.core.option\n\nstruct Option[T] {\n    is_some: bool\n    value: T\n}\n\nfn is_some(o: Option[T]) bool {\n}\n\nextern fn emit(level: u32, subject: string, message: string) void\n")

	set := ReadApiSet(root, "")
	if set.Empty() {
		t.Fatal("expected the source to be read")
	}
	if len(set.Modules) != 1 || set.Modules[0].Name != "std.core.option" {
		t.Fatalf("unexpected modules: %+v", set.Modules)
	}
	module := set.Modules[0]
	if len(module.Functions) != 1 || module.Functions[0].Name != "is_some" {
		t.Fatalf("expected the declared function, got %+v", module.Functions)
	}
	if module.Functions[0].Signature() != "fn is_some(o: Option[T]) bool" {
		t.Fatalf("unexpected signature %q", module.Functions[0].Signature())
	}
	if len(module.Structs) != 1 || len(module.Structs[0].Fields) != 2 {
		t.Fatalf("expected the struct and its fields, got %+v", module.Structs)
	}
	// An extern is what a port supplies, so it is listed apart from the
	// functions Rho declares.
	if len(module.Externs) != 1 || module.Externs[0].Name != "emit" {
		t.Fatalf("expected the extern, got %+v", module.Externs)
	}
	if module.Path != "std/core/option.rho" {
		t.Fatalf("expected the source path relative to the tree, got %q", module.Path)
	}
}

func TestReadApiSetSkipsTestSources(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "std/core/option.rho", "module std.core.option\n")
	writeFile(t, root, "tests/option_test.rho", "test \"x\" {\n}\n")
	writeFile(t, root, "tests/helper.rho", "module helper\n")

	set := ReadApiSet(root, "")
	names := []string{}
	for _, module := range set.Modules {
		names = append(names, module.Name)
	}
	if len(names) != 1 || names[0] != "std.core.option" {
		t.Fatalf("expected only the library source, got %+v", names)
	}
}

func TestReadApiSetReportsASourceItCannotRead(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "std/core/option.rho", "module std.core.option\n\nfn is_some(o: u8) bool {\n}\n")
	writeFile(t, root, "std/core/broken.rho", "module std.core.broken\n\nfn (\n")

	set := ReadApiSet(root, "")
	if set.Empty() {
		t.Fatal("expected the readable module to still be found")
	}
	if len(set.Modules) != 1 || set.Modules[0].Name != "std.core.option" {
		t.Fatalf("expected only the readable module, got %+v", set.Modules)
	}
	if len(set.Diagnostics) == 0 {
		t.Fatal("expected the unreadable source to be reported")
	}
}

func TestApiPagesAppearInTheSite(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "std/core/option.rho", "module std.core.option\n\nfn is_some(o: u8) bool {\n}\n")

	set := ReadApiSet(root, "")
	site := BuildSite(nil, set, SiteOptions{Title: "Rho"})
	index := findPage(site, "api")
	page := findPage(site, "api-std-core-option")
	if index == nil || page == nil {
		t.Fatalf("expected an index and a reference page, got %d pages", len(site.Pages))
	}
	if !index.Generated || !page.Generated {
		t.Fatal("expected the reference pages to be marked generated")
	}
	if !strings.Contains(page.Body, "is_some()") {
		t.Fatalf("expected the function in the reference, got:\n%s", page.Body)
	}
	if !strings.Contains(index.Body, "api-std-core-option") {
		t.Fatalf("expected the index to link the module page, got:\n%s", index.Body)
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

func findPage(site *Site, target string) *SitePage {
	for _, page := range site.Pages {
		if page.Target == target {
			return page
		}
	}
	return nil
}

func TestBuildSiteResolvesAHeadingLabel(t *testing.T) {
	// A heading written with an explicit target is addressable from anywhere in
	// the site, which is what lets a page point at a section rather than at the
	// top of another page.
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndocs:\n  section: guides\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n\nSee {ref}`declaring-it`.\n")
	writeFile(t, root, "app/docs/guide.md", "# Guide\n\n## Declaring it {#declaring-it}\n\nHow to declare a port.\n")

	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	index := findPage(site, "index")
	if index == nil {
		t.Fatal("expected the index page")
	}
	if !strings.Contains(index.Body, "href=\"guides/app/guide.html#declaring-it\"") {
		t.Fatalf("expected the label to resolve to the section, got:\n%s", index.Body)
	}
	if len(site.Requests) != 0 {
		t.Fatalf("expected the label to resolve, got %+v", site.Requests)
	}
}

func TestBuildSiteReportsALabelUsedTwice(t *testing.T) {
	// A label two pages claim cannot be resolved, so it is reported rather
	// than pointing a reference at whichever page happened to be read first.
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	writeFile(t, root, "app/docs/one.md", "# One\n\n## Shared {#shared}\n")
	writeFile(t, root, "app/docs/two.md", "# Two\n\n## Shared {#shared}\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n\nSee {ref}`shared`.\n")

	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	reported := false
	for _, diagnostic := range site.Diagnostics {
		if strings.Contains(diagnostic.Message, "label \"shared\"") {
			reported = true
		}
	}
	if !reported {
		t.Fatalf("expected the duplicate label to be reported, got %+v", site.Diagnostics)
	}
}
