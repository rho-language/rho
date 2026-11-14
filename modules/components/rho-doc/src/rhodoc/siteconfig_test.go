package rhodoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A repository's docs/site.yml puts its sections and pages in the order it
// lists them, names the sections, and opens the site on the page it chooses.
func TestASiteFileOrdersTheSiteAndChoosesItsFrontPage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/site.yml", "title: Rho\nfront: documentation/index\napi: none\nsections:\n  - name: documentation\n    title: Start\n    pages: [index, getting-started]\n  - name: language\n    title: Language\n")
	writeFile(t, root, "docs/index.md", "# Overview\n")
	writeFile(t, root, "docs/cli.md", "# Command line\n")
	writeFile(t, root, "docs/getting-started.md", "# Getting started\n")
	writeFile(t, root, "docs/language/syntax.md", "# Syntax\n")
	writeFile(t, root, "docs/aaa/notes.md", "# Notes\n")

	config, err := ReadSiteConfig(root)
	if err != nil || config.Title != "Rho" || config.API != "none" || len(config.Sections) != 2 || len(config.Sections[0].Pages) != 2 {
		t.Fatalf("site.yml read as %+v, %v", config, err)
	}
	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{Title: config.Title, Sections: config.Sections, Front: config.Front})
	titles := []string{}
	for _, page := range site.Pages {
		titles = append(titles, page.Title)
	}
	if got := strings.Join(titles, ","); got != "Overview,Getting started,Command line,Syntax,Notes" {
		t.Fatalf("pages in the wrong order: %s", got)
	}
	if label := site.sectionLabel("documentation"); label != "Start" {
		t.Fatalf("the section is called %q", label)
	}
	out := t.TempDir()
	if err := site.Write(out); err != nil {
		t.Fatal(err)
	}
	index, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !strings.Contains(string(index), "url=documentation/index.html") {
		t.Fatalf("index.html does not open the front page:\n%s", index)
	}
}
