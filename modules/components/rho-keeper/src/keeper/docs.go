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
		Body:       rewriteReferences(page.Body, Routes()),
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
			Section: filepath.ToSlash(filepath.Dir(relative))})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// DocRoutes maps the generator's page targets to this dashboard's routes.
//
// The two are not the same, and assuming they are is what makes every link in the
// documentation wrong here:
//
//   - A page the *generator* writes has a **target**. For the project's own
//     documentation the target carries the section a page lives in, so
//     `docs/getting-started.md` is the target `documentation/getting-started`, and
//     a module's front page is `<module>/index`.
//   - A page *this dashboard* serves has a **route**, which is the path a reader
//     sends: `getting-started` for the project's own page, and the module's own
//     name — `rho-flash` — for a module, because that is the only page of a
//     module's documentation this dashboard reads.
//
// So `{ref}`getting-started <documentation/getting-started>“ resolves to
// `#/docs/documentation/getting-started` if the target is used as the route, and
// the page behind it does not exist: the file is `docs/getting-started.md`.
// Every such link lands on a 404 while looking entirely plausible.
//
// Built from what is actually here rather than from a rule, because the two
// schemes differ in more than one place and a rule would be a guess about the next
// difference. A target this dashboard has no page for is simply absent from the
// map, and is then left alone rather than turned into a link to nothing.
type DocRoutes map[string]string

// Routes is every target this dashboard can reach, and where it lives.
//
// Read once per render. It walks the documentation tree and the installed
// modules, which is a directory listing rather than work: a dashboard renders one
// page at a time and this is cheap enough that caching it would be a thing to
// invalidate.
func Routes() DocRoutes {
	routes := DocRoutes{}

	// The project's own pages: the route is the path, and the target is that path
	// under the section the file is in — with the pages written directly in the
	// documentation root belonging to the section the generator calls them.
	for _, file := range ProjectDocs() {
		target := file.Path
		if !strings.Contains(file.Path, "/") {
			target = repositorySection + "/" + file.Path
		}
		routes[target] = file.Path
	}

	// A module's documentation is served at its own name, whichever of the two it
	// keeps. Both spellings of the target are registered, because a cross-reference
	// may name either and both mean the same page.
	for _, module := range Read().Modules {
		if !HasDocs(module.Name) {
			continue
		}
		routes[module.Name] = module.Name
		routes[module.Name+"/index"] = module.Name
	}
	return routes
}

// repositorySection is the section the generator files the pages written directly
// in a repository's documentation root under.
//
// It is the generator's own name for them, read from its source rather than
// repeated here, so that the two cannot drift: a reference is written with this
// name because that is the name the generator gives the page.
const repositorySection = rhodoc.RepositorySection

// rewriteReferences turns the generator's cross-references into this page's
// routes.
//
// A cross-reference is written as `#handbook/ports` — the *target* of another
// page, which is what the generator resolves a reference against — and this
// dashboard addresses pages by route. So the target is looked up in the map of
// what is here and replaced by the route for it.
//
// Two things are left alone, both deliberately:
//
//   - A link whose fragment is an identifier on this page is an anchor. That is
//     what it is, and rewriting it would send a reader to another page.
//   - A target with no page here is left as the fragment it already is. A link to
//     a page this dashboard does not serve is a link that goes nowhere either way;
//     turning it into a route that does not exist makes it a dead one instead of
//     an inert one, and an inert link is visibly inert in the source.
//
// A cross-reference left as `#target` renders as an anchor on this page, so it does
// not look like navigation — which is why the map is built from what is installed
// rather than left to chance.
func rewriteReferences(body string, routes DocRoutes) string {
	onPage := map[string]bool{}
	for _, match := range anchorID.FindAllStringSubmatch(body, -1) {
		onPage[match[1]] = true
	}
	return crossReference.ReplaceAllStringFunc(body, func(fragment string) string {
		target := strings.TrimSuffix(strings.TrimPrefix(fragment, "#"), ".md")
		if onPage[target] {
			return fragment
		}
		route, reachable := routes[target]
		if !reachable {
			return fragment
		}
		return "#/docs/" + route
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
