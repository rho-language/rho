//
// @file docs.go
// @brief The documentation this dashboard can show, read properly.
//
// Two things are documented here: this project's own, and each module's. A
// machine with a dozen modules is a machine whose modules have documentation
// that is somewhere, and a page that lists a module without a way to read it is
// a list that sends a person to a terminal to find out what a thing is.
//
// The markdown is read by the generator's own reader rather than a second one.
// That is the point of sharing it: a parser written for a dashboard would be a
// third opinion about what the project's documentation says, and the one a person
// reads on a generated site is the one they should be shown here too. It also
// brings the table of contents with it, which is a list of the headings a
// document has — the thing a person scrolls beside a long page rather than a
// thing this has to work out again.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/rho/rho-doc/src/rhodoc"
	"rho/src/deps"
)

// Docs is a document, read and rendered.
type Docs struct {
	// Title is the document's own, and Target the name it is addressed by.
	Title  string `json:"title"`
	Target string `json:"target"`
	// Body is the rendered document, as HTML.
	Body string `json:"body"`
	// Sections are its headings, which is the menu beside it. They come from the
	// generator's own reading, so a heading is identified here the way it is on a
	// generated site and a link to one is a link to the same place.
	Sections []Section `json:"sections"`
	// Breadcrumb is the trail of files leading here, so a reader knows where in
	// the documentation they are.
	Breadcrumb []string `json:"breadcrumb,omitempty"`
}

// The documentation root is where the project's own documentation is.
//
// The documentation is about the project rather than about any one component,
// so it is not part of a component's package. It is looked for in three places,
// in the order a person would:
//   - where it was named, when `RHO_DOCS_ROOT` says;
//   - beside the components, walking up from where this module was installed
//     from, which is a checkout;
//   - in the Rho home, which is where `rho install` puts it, and which is what
//     makes the documentation reachable on a machine that has the toolchain and
//     not the source.
func docsRoot() string {
	if root := strings.TrimSpace(os.Getenv("RHO_DOCS_ROOT")); root != "" {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root
		}
	}
	for current := moduleRoot(); current != ""; {
		candidate := filepath.Join(current, "docs")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		if home := deps.RhoHome(); home != "" {
			candidate = filepath.Join(home, "docs")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

// ProjectDocsAvailable reports whether the project's own documentation was found,
// so a page can say that rather than showing an empty menu and a page that
// quietly did not load.
func ProjectDocsAvailable() bool { return docsRoot() != "" }

// render is a markdown file read and rendered, or nil when it is not there.
func render(path, target string, breadcrumb []string) (*Docs, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	document := rhodoc.Parse(target, string(source))
	page := rhodoc.RenderPage(document)
	title := page.Title
	if title == "" {
		title = page.Target
	}
	out := &Docs{
		Title:  title,
		Target: page.Target,
		// The body is rewritten before it is sent, because a link that was right
		// on a generated site is not right here: there, a cross-reference is
		// resolved against the site the pages were written into; here, a page is
		// addressed by this dashboard's own route.
		Body:       rewriteReferences(page.Body),
		Breadcrumb: breadcrumb,
	}
	for _, section := range page.Sections {
		out.Sections = append(out.Sections, Section{ID: section.ID, Title: section.Title, Level: section.Level})
	}
	return out, nil
}

// ProjectDocs is the table of files in the project's own documentation.
//
// It is a listing rather than one page, because the documentation is a tree —
// a handbook and a language reference, each with pages — and a dashboard that
// showed one of them and hid the rest would be showing a choice somebody else
// made.
func ProjectDocs() []DocFile {
	// An empty list rather than nothing at all: a page that cannot tell "there
	// is no documentation" from "the request failed" shows the same thing twice.
	out := []DocFile{}
	root := docsRoot()
	if root == "" {
		return out
	}
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		if name != "index.md" && !strings.HasSuffix(name, ".md") {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		out = append(out, DocFile{
			Path:    strings.TrimSuffix(filepath.ToSlash(relative), ".md"),
			Title:   docTitle(path, relative),
			Section: filepath.ToSlash(filepath.Dir(relative)),
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// rewriteReferences turns the generator's cross-references into this page's
// routes.
//
// A cross-reference is written as `#handbook/ports` — the name of another page —
// because on a generated site a page is addressed by its target. Here a page is
// addressed by a route, and a link left as it is would be read by the browser as
// an anchor inside the page being read: the page would appear not to link
// anywhere at all, which is worse than a link that is plainly wrong.
//
// A link whose fragment *is* an identifier on this page is an anchor and is left
// alone, because that is what it is.
func rewriteReferences(body string) string {
	onPage := map[string]bool{}
	for _, match := range anchorID.FindAllStringSubmatch(body, -1) {
		onPage[match[1]] = true
	}
	return crossReference.ReplaceAllStringFunc(body, func(fragment string) string {
		target := strings.TrimSuffix(strings.TrimPrefix(fragment, "#"), "")
		if onPage[target] {
			return fragment
		}
		return "#/docs/" + strings.TrimSuffix(target, ".md")
	})
}

var (
	// anchorID is an identifier a page defines, which is what an in-page link
	// points at.
	anchorID = regexp.MustCompile(`id="([^"]+)"`)
	// crossReference is a link to another page, written as the name of that
	// page.
	crossReference = regexp.MustCompile(`#([A-Za-z0-9_./-]+)`)
)

// Section is one heading, as this page sends it.
//
// It is the generator's own heading, carried in this page's own shape. The
// generator does not promise a wire format for its headings, and a dashboard
// that read one would break the first time a field was renamed there — which is
// a thing that happens to a struct nobody serialises on purpose.
type Section struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Level int    `json:"level"`
}

// DocFile is one page in the documentation.
type DocFile struct {
	// Path is how it is addressed, relative to the documentation root.
	//
	// It is the page's name and not its file's: a reader moves between
	// `#/docs/handbook/ports` and a page, and an address that said `.md` in it
	// would be a filename in a place that is otherwise made of names. The
	// extension is added when the file is looked up, not carried in the link.
	Path string `json:"path"`
	// Title is what a menu shows for it.
	Title string `json:"title"`
	// Section is the part of the tree it is in, which is what the menu groups
	// by.
	Section string `json:"section"`
}

// docTitle is a page's own title, read from its first heading.
//
// The heading is read rather than the file name, because a file called
// `modules.md` is titled "Modules, components and plugins" and a menu that said
// the former would be a menu of file names pretending to be titles.
func docTitle(path, relative string) string {
	source, err := os.ReadFile(path)
	if err != nil {
		return relative
	}
	for _, line := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "<!--") {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
		break
	}
	return relative
}

// ReadProjectDoc is one page of the project's own documentation.
func ReadProjectDoc(name string) (*Docs, error) {
	root := docsRoot()
	if root == "" {
		return nil, os.ErrNotExist
	}
	// A name is a path inside the documentation and nothing else. A reader sends
	// one from a menu this process built, and a name that climbs out of the tree
	// is not one of those.
	// The front page is addressed by nothing, the way a book's first page is.
	requested := strings.TrimSpace(filepath.ToSlash(name))
	if requested == "" {
		requested = "index"
	}
	clean := filepath.Clean(filepath.FromSlash(requested))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, os.ErrNotExist
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	// A name is a page's name. The extension is this side's business: a link
	// that carried it would be a filename in a place made of names, and the file
	// it needs is a decision made here rather than by whoever wrote the link.
	//
	// The whole relative path is what is looked up, not only the last part of
	// it: `handbook/ports` is a page two directories in, and looking for
	// `ports.md` in the root is looking for a page that is not that one.
	target := strings.TrimSuffix(parts[len(parts)-1], ".md")
	file := filepath.Join(root, clean)
	// A name is a page, and a page's file carries an extension. A name that is a
	// directory is a section, and the page that opens it is its index — the two
	// are told apart by asking, not by the shape of the name, because a reader
	// writes both the same way.
	if info, err := os.Stat(file + ".md"); err == nil && !info.IsDir() {
		file += ".md"
	} else if info, err := os.Stat(file); err == nil && info.IsDir() {
		file = filepath.Join(file, "index.md")
	} else if _, err := os.Stat(filepath.Join(file, "index.md")); err == nil {
		file = filepath.Join(file, "index.md")
	} else {
		return nil, os.ErrNotExist
	}
	if _, err := os.Stat(file); err != nil {
		// A name that is a directory names the page that opens it, which is how a
		// person writes a path they can see in the menu.
		if _, index := os.Stat(filepath.Join(file, "index.md")); index == nil {
			file = filepath.Join(file, "index.md")
		} else {
			return nil, os.ErrNotExist
		}
	}
	return render(file, target, parts[:max(len(parts)-1, 0)])
}

// ReadModuleDoc is a module's own documentation, which is the README beside its
// manifest — the file a person is sent to when a page offers them a module's
// documentation at all.
func ReadModuleDoc(name string) (*Docs, error) {
	root := moduleRoot()
	_ = root
	found := ModuleDocFile(name)
	if found == "" {
		return nil, os.ErrNotExist
	}
	return render(found, name, []string{name})
}

// ModuleDocFile is where a module's documentation is, or "" when it has none.
//
// A module may keep more than a README, and may have a docs directory beside it
// the way this one does. A module with either has a badge; a module with
// neither does not, because a badge that opens an empty page is a badge that
// lies.
func ModuleDocFile(name string) string {
	dir := installedRoot(name)
	if dir == "" {
		return ""
	}
	// The README is the documentation a module is said to have by existing, and
	// it is read first because it is the one a person is sent to.
	readme := filepath.Join(dir, "README.md")
	if _, err := os.Stat(readme); err == nil {
		return readme
	}
	docs := filepath.Join(dir, "docs", "index.md")
	if _, err := os.Stat(docs); err == nil {
		return docs
	}
	return ""
}

// HasDocs reports whether a module carries documentation worth a badge.
func HasDocs(name string) bool { return ModuleDocFile(name) != "" }

// installedRoot is where a module of that name is installed, whatever kind it
// is.
func installedRoot(name string) string {
	for _, kind := range []string{deps.KindComponent, deps.KindPlugin} {
		versions := deps.InstalledVersions(kind, name)
		if len(versions) == 0 {
			continue
		}
		for _, version := range versions {
			candidate := filepath.Join(deps.RhoHome(), "modules", "installed", kind, name+"@"+version)
			if _, err := os.Stat(filepath.Join(candidate, "module.yml")); err == nil {
				return candidate
			}
		}
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
